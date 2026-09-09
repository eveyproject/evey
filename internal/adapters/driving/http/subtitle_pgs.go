package httpapi

// subtitle_pgs.go - serving a BITMAP subtitle track (PGS/VobSub) to the player
// instead of burning it into the video.
//
// Until now a bitmap track had exactly two fates: burnt into the frames by
// ffmpeg, or nothing. Burning denies the video copy rung, so one forced French
// subtitle - the track a retail rip flags DEFAULT - turned a 4K HEVC remux that
// direct-played fine into a full transcode (prod, 2026-09-08). This path is the
// third fate: decode the bitmaps once, cache them, and let the client draw them
// exactly as it already draws WebVTT cues.
//
// WHY THE DECODED IMAGES AND NOT THE .sup STREAM. Caching the encoded stream is
// what Jellyfin does, and it is the most expensive of the three options
// measured: PGS's RLE compresses worse than PNG's deflate, and a .sup repeats
// its palette and window segments at every display set. On two real tracks:
// 21.0/28.6 MB as .sup, 9.4/13.4 MB as true-colour PNG, 6.3/8.8 MB as PALETTED
// PNG. A PGS cue never exceeds 256 colours - the format guarantees it, one
// palette per cue - so the paletted form always applies. It is a third of the
// size AND serving becomes a file read instead of a decode per request.
//
// Sized on a real library: 733 bitmap tracks x ~988 cues x 5.5-7.9 kB is 4 to
// 6 GB for the whole catalogue, against a transcode cache already set to 50 GB.
//
// WHY ONE BLOB. A track is ~1000 cues; one HTTP request per cue is not an
// option, and a sprite sheet would need packing a thousand variable-size images.
// The blob is a JSON header naming every cue plus the PNG bytes concatenated
// behind it, so the client fetches once, slices, and builds object URLs.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/subtitle"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// pgsBlobMagic identifies the container and its version. A player that does not
// recognise it must refuse the body rather than guess at its shape.
const pgsBlobMagic = "EVEYPGS1"

// pgsHeaderMaxBytes bounds the JSON header a reader will accept before it has
// any reason to trust the body. A track of 3000 cues serialises well under it;
// anything larger is a corrupt or hostile blob, not a subtitle.
const pgsHeaderMaxBytes = 8 << 20

// pgsCue is one displayed bitmap: when it shows, where on the subtitle plane,
// and where its PNG sits in the blob.
//
// The field names are short because they repeat once per cue and the header is
// sent to the client verbatim - on a 2000-cue track the difference is tens of
// kilobytes of JSON.
type pgsCue struct {
	StartMs int64 `json:"s"`
	EndMs   int64 `json:"e"`
	X       int   `json:"x"`
	Y       int   `json:"y"`
	W       int   `json:"w"`
	H       int   `json:"h"`
	// Off/Len locate this cue's PNG in the bytes that follow the header.
	Off int `json:"o"`
	Len int `json:"l"`
	// Forced marks a cue the disc wants shown whatever the viewer chose - a
	// translated sign, a line in another language. Carried PER CUE because a
	// full track commonly mixes both: one measured disc has 13 forced cues
	// inside a 2032-cue track. The track-level flag cannot express that.
	Forced bool `json:"f,omitempty"`
}

// pgsHeader is the JSON that opens the blob.
type pgsHeader struct {
	// ScreenW/ScreenH is the SUBTITLE PLANE the coordinates refer to, which is
	// NOT necessarily the video resolution: every 2160p disc measured carries a
	// 1920x1080 subtitle plane. Scaling by the video size instead would offset
	// and mis-size every cue by a factor of two.
	ScreenW int      `json:"screenW"`
	ScreenH int      `json:"screenH"`
	Cues    []pgsCue `json:"cues"`
}

// encodePGSBlob renders the header and the images into the on-disk/on-wire form:
//
//	"EVEYPGS1" | uint32be header length | header JSON | PNG bytes, back to back
func encodePGSBlob(h pgsHeader, images [][]byte) ([]byte, error) {
	hj, err := json.Marshal(h)
	if err != nil {
		return nil, fmt.Errorf("pgs blob: marshal header: %w", err)
	}
	if len(hj) > pgsHeaderMaxBytes {
		return nil, fmt.Errorf("pgs blob: header of %d bytes exceeds the %d cap", len(hj), pgsHeaderMaxBytes)
	}
	var b bytes.Buffer
	b.Grow(len(pgsBlobMagic) + 4 + len(hj))
	b.WriteString(pgsBlobMagic)
	// Written by hand rather than through binary.Write: a bytes.Buffer never
	// fails, so the error return would only be there to be discarded.
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(hj))) //nolint:gosec // bounded by the cap above
	b.Write(n[:])
	b.Write(hj)
	for _, img := range images {
		b.Write(img)
	}
	return b.Bytes(), nil
}

// decodePGSBlobHeader reads back the header. Used by the tests and by any
// caller that needs the cue list without the pixels.
func decodePGSBlobHeader(blob []byte) (pgsHeader, error) {
	var h pgsHeader
	if len(blob) < len(pgsBlobMagic)+4 || string(blob[:len(pgsBlobMagic)]) != pgsBlobMagic {
		return h, fmt.Errorf("pgs blob: bad magic")
	}
	n := int(binary.BigEndian.Uint32(blob[len(pgsBlobMagic):]))
	start := len(pgsBlobMagic) + 4
	if n > pgsHeaderMaxBytes || start+n > len(blob) {
		return h, fmt.Errorf("pgs blob: header length %d out of range", n)
	}
	if err := json.Unmarshal(blob[start:start+n], &h); err != nil {
		return h, fmt.Errorf("pgs blob: header: %w", err)
	}
	return h, nil
}

// extractPGSBlob decodes one bitmap subtitle track into the blob above.
//
// ForEach and not the slice form on purpose: a full track is ~2000 cues, and
// holding every decoded RGBA image at once is tens of megabytes for a job that
// runs at nice 19 next to a playing film. This way one image is live at a time
// and the PNG bytes accumulate instead.
//
// The index is optional. Building one is a full pass of the container, so it
// only pays when several tracks will be read from it; a caller extracting a
// single track hands nil and mkvgo walks it directly.
func extractPGSBlob(ctx context.Context, filePath string, trackID uint64, ix *matroska.SubtitleIndex) ([]byte, error) {
	var (
		h      pgsHeader
		images [][]byte
		off    int
		buf    bytes.Buffer
	)
	// Paletted PNG: the whole reason the decoded form is smaller than the .sup.
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}

	collect := func(c matroska.PGSCue) error {
		if c.Image == nil {
			return nil
		}
		// The plane is a property of the track, not of the cue; every cue of a
		// track reports the same one. Taking the last is taking the only.
		h.ScreenW, h.ScreenH = c.ScreenW, c.ScreenH

		var src image.Image = c.Image
		// A PGS cue is drawn from a single palette and never exceeds 256
		// colours, so this re-indexing always applies. It halves the encoded
		// size on its own - measured -51% against true-colour RGBA, where the
		// compression level is worth only -4%, which is why the level stays at
		// the default and the CPU stays with the film.
		if p := subtitle.Paletted(c.Image); p != nil {
			src = p
		}
		buf.Reset()
		if err := enc.Encode(&buf, src); err != nil {
			return fmt.Errorf("encode cue at %d ms: %w", c.StartMs, err)
		}
		b := bytes.Clone(buf.Bytes())
		r := c.Image.Bounds()
		h.Cues = append(h.Cues, pgsCue{
			StartMs: c.StartMs, EndMs: c.EndMs,
			X: c.X, Y: c.Y, W: r.Dx(), H: r.Dy(),
			Off: off, Len: len(b), Forced: c.Forced,
		})
		images = append(images, b)
		off += len(b)
		return nil
	}

	var err error
	if ix != nil {
		err = matroska.ForEachSubtitlePGSFrom(ctx, filePath, trackID, ix, collect)
	} else {
		err = matroska.ForEachSubtitlePGS(ctx, filePath, trackID, collect)
	}
	if err != nil {
		return nil, fmt.Errorf("pgs extract: %w", err)
	}
	// A track with no cue at all is a legitimate answer, not a failure: a forced
	// track carries a handful of cues in a whole film and can be empty over any
	// stretch of it. The blob is still well-formed and the player shows nothing.
	return encodePGSBlob(h, images)
}

// mkvgoImageSubtitleTrackIDs is mkvgoTextSubtitleTrackIDs' counterpart: the
// mkvgo track ID of each BITMAP subtitle track, keyed by the subtitle-relative
// ordinal the route and the cache key count in.
func mkvgoImageSubtitleTrackIDs(ctx context.Context, ext, filePath string) (map[int]uint64, error) {
	c, err := openMkvgoSubtitleContainer(ctx, ext, filePath)
	if err != nil {
		return nil, fmt.Errorf("open container: %w", err)
	}
	out := make(map[int]uint64)
	subIdx := 0
	for i := range c.Tracks {
		if c.Tracks[i].Type != extmkv.SubtitleTrack {
			continue
		}
		cur := subIdx
		subIdx++
		if !apptranscoding.IsImageSubtitle(extmkv.FFprobeCodecName(c.Tracks[i].Codec)) {
			continue
		}
		out[cur] = c.Tracks[i].ID
	}
	return out, nil
}

// pgsServable reports whether this bitmap track can be served as decoded images
// rather than burnt in. PGS only: VobSub is a bitmap too but mkvgo decodes only
// PGS, and claiming to serve a VobSub track would leave the viewer with a
// subtitle menu entry that draws nothing.
func pgsServable(codec string) bool {
	return strings.Contains(strings.ToLower(codec), "pgs")
}

// serveBitmapSubtitle answers the subtitle route for a PGS track: the cached
// blob when there is one, a fresh extraction otherwise.
//
// Same contract as the WebVTT path next door - disk cache keyed on the source's
// size and mtime, one extraction at a time per track, best-effort store - for
// the same reason: producing this costs a full read of the container, so a
// player retrying what it believes is a hung request must not start a second.
func serveBitmapSubtitle(w http.ResponseWriter, r *http.Request, span trace.Span, cache subtitleCache, filePath string, trackIndex int, mediaID string) {
	ctx := r.Context()
	cacheKey := cache.keyForKind(filePath, trackIndex, subtitleKindPGS)
	if data, ok := cache.get(cacheKey); ok {
		writePGSBlob(w, r, data)
		return
	}

	data, err := runSharedSubtitleExtraction(ctx, subtitleFlightKey(cacheKey, filePath), func(ctx context.Context) ([]byte, error) {
		if data, ok := cache.get(cacheKey); ok {
			return data, nil
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if !isMkvgoSubtitleFastPath(ext) || isMkvgoMP4Ext(ext) {
			// No PGS transport exists in MP4, and .avi never reaches mkvgo. There
			// is no fallback to fall back to: ffmpeg would only hand back the
			// undecoded stream.
			return nil, fmt.Errorf("bitmap subtitles are only served from Matroska (got %q)", ext)
		}
		ids, idErr := mkvgoImageSubtitleTrackIDs(ctx, ext, filePath)
		if idErr != nil {
			return nil, idErr
		}
		trackID, ok := ids[trackIndex]
		if !ok {
			return nil, fmt.Errorf("no bitmap subtitle at ordinal %d", trackIndex)
		}
		// nil index: this is one track on demand, and building an index is a
		// full pass that would save nothing here. The prewarmer, which extracts
		// several tracks of the same file, passes one.
		data, exErr := extractPGSBlob(ctx, filePath, trackID, nil)
		if exErr != nil {
			return nil, exErr
		}
		if cErr := cache.put(cacheKey, data); cErr != nil {
			slog.WarnContext(ctx, "subtitle: pgs cache store failed",
				slog.String("media_id", mediaID), slog.Any("err", cErr))
		}
		return data, nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "pgs extraction failed")
		slog.WarnContext(ctx, "subtitle: pgs extraction failed",
			slog.String("media_id", mediaID), slog.Int("track", trackIndex), slog.Any("err", err))
		http.Error(w, `{"message":"subtitle not available"}`, http.StatusUnprocessableEntity)
		return
	}
	writePGSBlob(w, r, data)
}

// writePGSBlob sends the blob. Same caching contract as the WebVTT response:
// the bytes are pinned to the source's size and mtime by the cache key, so a
// client may keep them - re-fetching costs a full read of the film.
func writePGSBlob(w http.ResponseWriter, r *http.Request, data []byte) {
	w.Header().Set("Content-Type", "application/vnd.evey.pgs")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	setCORSHeaders(w, r, "")
	if _, err := w.Write(data); err != nil {
		slog.WarnContext(r.Context(), "subtitle: pgs response write failed", slog.Any("err", err))
	}
}
