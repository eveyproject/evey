package httpapi

// subtitle_bitmap.go - the VobSub (DVD) and DVB bitmap tracks, served in the
// same decoded-image blob as PGS (subtitle_pgs.go) so every player draws them
// with the code it already has.
//
// mkvgo reads the blocks of an embedded track, from the subtitle index when
// there is one, and passes over a damaged region
// (matroska.ForEachSubtitleBlock[From]). The cues are assembled here, from
// mkvgo's block decoders, so that a block whose bytes do not decode costs its
// own picture and not the track: mkvgo's ready-made bitmap read
// (ForEachSubtitleBitmap) stops on the first such block. A picture sidecar
// (.sup, or an .idx/.sub pair) goes through the same assembly.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mkv/subtitle"
)

// mkvgo's names (mkv.Track.Codec) for the bitmap formats decoded here; the
// Matroska codec IDs are S_VOBSUB and S_DVBSUB.
const (
	codecVobSub = "vobsub"
	codecDVBSub = "dvbsub"
)

// bitmapDefaultCueMs is how long a cue stays on screen when nothing in the
// stream ends it: no stop command, no block duration, no later display set.
// The value mkvgo uses for the same case on PGS and text tracks.
const bitmapDefaultCueMs = 3000

// bitmapMaxPlane bounds the picture several objects of one display set are
// composited onto: two dots placed far apart by a forged stream would otherwise
// ask for a picture of gigabytes.
const bitmapMaxPlane = 4096

// bitmapDecoder is what VobSubDecoder and DVBSubDecoder have in common.
type bitmapDecoder interface {
	Decode(block []byte) ([]subtitle.BitmapDisplaySet, error)
}

// newBitmapDecoder returns the decoder for a VobSub or DVB track, or an error
// for any other codec.
func newBitmapDecoder(t *extmkv.Track) (bitmapDecoder, error) {
	switch t.Codec {
	case codecVobSub:
		d, err := subtitle.NewVobSubDecoder(t.CodecPrivate)
		if err != nil {
			return nil, fmt.Errorf("vobsub track %d: %w", t.ID, err)
		}
		return d, nil
	case codecDVBSub:
		return subtitle.NewDVBSubDecoder(t.CodecPrivate), nil
	}
	return nil, fmt.Errorf("track %d: codec %q is not a VobSub or DVB subtitle", t.ID, t.Codec)
}

// bitmapCueAssembler turns display sets into timed cues.
//
// A cue starts at its block's time plus the set's Delay (a VobSub start
// command). It ends, in order of authority: at the set's own Duration (a
// VobSub stop command), at the block's duration, or at the next display set,
// bounded by the set's Timeout (a DVB page time-out). A cue nothing ends lasts
// bitmapDefaultCueMs, or its Timeout when it has one.
type bitmapCueAssembler struct {
	dec       bitmapDecoder
	emit      func(matroska.PGSCue) error
	open      *matroska.PGSCue
	openLimit int64 // the open cue's Timeout in ms, 0 for none
	// The time and duration of the block that started the unit being
	// decoded: a VobSub unit a muxer spread over several blocks is timed by
	// its first one.
	unitMs, unitDurMs int64
	// Blocks that did not decode, and the last one's error: a track where
	// every block fails is reported as the failure it is.
	failed, emitted int
	lastErr         error
}

// decoderResetter is the decoder that carries state from block to block (a
// DVB epoch, a VobSub unit started in an earlier block).
type decoderResetter interface{ Reset() }

// restart drops what the decoder carried over, after a block that did not
// decode or a damaged region the read passed over.
func (a *bitmapCueAssembler) restart() {
	if r, ok := a.dec.(decoderResetter); ok {
		r.Reset()
	}
}

// skipDamage closes the cue on screen where a damaged region starts and
// restarts the decoder past it, so no picture spans the gap.
func (a *bitmapCueAssembler) skipDamage(fromMs int64) error {
	a.restart()
	if fromMs < 0 || a.open == nil {
		return nil
	}
	return a.closeAt(max(fromMs, a.open.StartMs))
}

// unitContinuer is the decoder that can tell a block finishing a unit started
// in earlier blocks from one starting a unit.
type unitContinuer interface {
	Continues(block []byte) bool
}

func (a *bitmapCueAssembler) block(timeMs, durMs int64, data []byte) error {
	if c, ok := a.dec.(unitContinuer); !ok || !c.Continues(data) {
		a.unitMs, a.unitDurMs = timeMs, durMs
	}
	timeMs, durMs = a.unitMs, a.unitDurMs
	sets, err := a.dec.Decode(data)
	if err != nil {
		// A block that does not decode loses its own picture, not the track:
		// the sets completed ahead of the fault are kept, and the decoder
		// starts over on the next block.
		a.failed++
		a.lastErr = fmt.Errorf("subtitle block at %d ms: %w", timeMs, err)
		a.restart()
	}
	for i := range sets {
		ds := &sets[i]
		start := timeMs + ds.Delay.Milliseconds()
		if err := a.closeAt(start); err != nil {
			return err
		}
		if ds.Clear() {
			continue
		}
		cue, err := flattenBitmapDisplaySet(ds, start)
		if err != nil {
			return fmt.Errorf("subtitle block at %d ms: %w", timeMs, err)
		}
		switch {
		case ds.Duration > 0:
			cue.EndMs = start + ds.Duration.Milliseconds()
		case durMs > 0 && timeMs+durMs > start:
			// A DVB page's time-out bounds it whatever the block says.
			cue.EndMs = timeMs + durMs
			if lim := ds.Timeout.Milliseconds(); lim > 0 && start+lim < cue.EndMs {
				cue.EndMs = start + lim
			}
		default:
			a.open, a.openLimit = &cue, ds.Timeout.Milliseconds()
			continue
		}
		if err := a.out(cue); err != nil {
			return err
		}
	}
	return nil
}

func (a *bitmapCueAssembler) out(c matroska.PGSCue) error {
	a.emitted++
	return a.emit(c)
}

// closeAt ends the open cue at t, or earlier when its time-out says so.
func (a *bitmapCueAssembler) closeAt(t int64) error {
	if a.open == nil {
		return nil
	}
	cue := *a.open
	a.open = nil
	cue.EndMs = t
	if a.openLimit > 0 && cue.StartMs+a.openLimit < cue.EndMs {
		cue.EndMs = cue.StartMs + a.openLimit
	}
	if cue.EndMs <= cue.StartMs {
		cue.EndMs = cue.StartMs + bitmapDefaultCueMs
	}
	return a.out(cue)
}

// finish flushes a cue the track never closed. A track that gave no cue
// while some of its blocks failed to decode is an error, not an empty track.
func (a *bitmapCueAssembler) finish() error {
	if a.open == nil {
		if a.emitted == 0 && a.failed > 0 {
			return a.lastErr
		}
		return nil
	}
	cue := *a.open
	a.open = nil
	cue.EndMs = cue.StartMs + bitmapDefaultCueMs
	if a.openLimit > 0 {
		cue.EndMs = cue.StartMs + a.openLimit
	}
	return a.out(cue)
}

// flattenBitmapDisplaySet reduces a display set to the one picture a cue
// holds: a set of several objects (a top and a bottom line) is composited onto
// the smallest rectangle covering them.
func flattenBitmapDisplaySet(ds *subtitle.BitmapDisplaySet, startMs int64) (matroska.PGSCue, error) {
	cue := matroska.PGSCue{StartMs: startMs, ScreenW: ds.ScreenW, ScreenH: ds.ScreenH}
	if len(ds.Objects) == 1 {
		o := ds.Objects[0]
		cue.X, cue.Y, cue.Forced, cue.Image = o.X, o.Y, o.Forced, o.Image
		return cue, nil
	}
	var union image.Rectangle
	for _, o := range ds.Objects {
		union = union.Union(o.Image.Bounds().Add(image.Pt(o.X, o.Y)))
		cue.Forced = cue.Forced || o.Forced
	}
	if union.Dx() > bitmapMaxPlane || union.Dy() > bitmapMaxPlane {
		return cue, fmt.Errorf("a display set spreads its objects over %dx%d pixels, past the %d of a subtitle plane", union.Dx(), union.Dy(), bitmapMaxPlane)
	}
	out := image.NewNRGBA(image.Rect(0, 0, union.Dx(), union.Dy()))
	for _, o := range ds.Objects {
		at := image.Pt(o.X-union.Min.X, o.Y-union.Min.Y)
		draw.Draw(out, image.Rectangle{Min: at, Max: at.Add(o.Image.Bounds().Size())}, o.Image, image.Point{}, draw.Over)
	}
	cue.X, cue.Y, cue.Image = union.Min.X, union.Min.Y, out
	return cue, nil
}

// forEachMatroskaBitmapCue calls fn with each cue of one VobSub or DVB track
// of a Matroska file, in presentation order: the blocks from mkvgo, from ix
// when it is not nil, the cues assembled here.
func forEachMatroskaBitmapCue(ctx context.Context, filePath string, trackID uint64, ix *matroska.SubtitleIndex, fn func(matroska.PGSCue) error, opts ...matroska.Options) error {
	c, err := reader.OpenMeta(ctx, filePath)
	if err != nil {
		return fmt.Errorf("open container: %w", err)
	}
	var track *extmkv.Track
	for i := range c.Tracks {
		if c.Tracks[i].ID == trackID {
			track = &c.Tracks[i]
			break
		}
	}
	if track == nil {
		return fmt.Errorf("no track %d", trackID)
	}
	dec, err := newBitmapDecoder(track)
	if err != nil {
		return err
	}
	asm := &bitmapCueAssembler{dec: dec, emit: fn}
	feed := func(blk matroska.SubtitleBlock) error {
		if blk.Lost {
			return asm.skipDamage(blk.TimeMs)
		}
		return asm.block(blk.TimeMs, blk.DurationMs, blk.Data)
	}
	if ix != nil {
		err = matroska.ForEachSubtitleBlockFrom(ctx, filePath, trackID, ix, feed, opts...)
	} else {
		err = matroska.ForEachSubtitleBlock(ctx, filePath, trackID, feed, opts...)
	}
	if err != nil {
		return err
	}
	return asm.finish()
}

// bitmapServable reports whether a bitmap track can be served as decoded
// images rather than burnt in: PGS, VobSub and DVB. XSUB cannot.
//
// ⚠️ Must agree with isClientDrawableSubtitle in
// frontend/packages/api/src/imageSubtitles.ts.
func bitmapServable(codec string) bool {
	switch strings.TrimSpace(strings.ToLower(codec)) {
	case "hdmv_pgs_subtitle", "pgssub", "pgs",
		"dvd_subtitle", "dvdsub",
		"dvb_subtitle", "dvbsub":
		return true
	}
	return false
}

// isPGSCodec reports whether mkvgo's codec name (mkv.Track.Codec) is PGS, read
// by mkvgo's PGS walk (extractPGSBlob).
func isPGSCodec(codec string) bool {
	return codec == "pgs"
}

// isExternalBitmapSubtitle reports whether a sidecar file is a picture
// subtitle: a PGS .sup, or the .idx of a VobSub .idx/.sub pair.
func isExternalBitmapSubtitle(ext string) bool {
	return ext == ".sup" || ext == ".idx"
}

// extractExternalBitmapBlob decodes a sidecar picture subtitle into the same
// blob as an embedded track. subPath is the .sub of a VobSub pair, unused for a
// .sup. Of a .idx holding several languages, the first is read: the scanner
// names the file by one language, and a pair made for one film rarely holds
// more.
func extractExternalBitmapBlob(ctx context.Context, path, subPath string) ([]byte, error) {
	var b bitmapBlobBuilder
	f, err := os.Open(path) //nolint:gosec // resolved under the video's directory by safeJoin
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var asm *bitmapCueAssembler
	feed := func(timeMs int64, block []byte) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return asm.block(timeMs, 0, block)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sup":
		asm = &bitmapCueAssembler{dec: subtitle.NewPGSDecoder(), emit: b.add}
		err = subtitle.ForEachSUPDisplaySet(f, feed)
	case ".idx":
		ix, pErr := subtitle.ParseVobSubIndex(f)
		if pErr != nil {
			return nil, pErr
		}
		dec, dErr := subtitle.NewVobSubDecoder(ix.Header)
		if dErr != nil {
			return nil, dErr
		}
		sub, oErr := os.Open(subPath) //nolint:gosec // resolved under the video's directory by safeJoin
		if oErr != nil {
			return nil, fmt.Errorf("the .sub of the pair: %w", oErr)
		}
		defer sub.Close()
		asm = &bitmapCueAssembler{dec: dec, emit: b.add}
		err = ix.ForEachUnit(sub, 0, feed)
	default:
		return nil, fmt.Errorf("%s is not a picture subtitle file", filepath.Base(path))
	}
	if err != nil {
		return nil, err
	}
	if err := asm.finish(); err != nil {
		return nil, err
	}
	return b.blob()
}

// vobSubPairSub finds the .sub beside an .idx, whatever the case of its
// extension.
func vobSubPairSub(idxPath string) (string, bool) {
	dir, name := filepath.Split(idxPath)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	// The scanner pairs the two in any letter case (hasSibling); so does this.
	for _, e := range entries {
		n := e.Name()
		if !strings.EqualFold(filepath.Ext(n), ".sub") || strings.TrimSuffix(n, filepath.Ext(n)) != stem {
			continue
		}
		// Followed through a symbolic link, as the scanner lists it and as
		// the video itself is served.
		if st, err := os.Stat(filepath.Join(dir, n)); err == nil && st.Mode().IsRegular() {
			return filepath.Join(dir, n), true
		}
	}
	return "", false
}

// externalBitmapKey keys a sidecar's blob on every file it is decoded from: a
// VobSub's .sub holds the pictures, so replacing it alone must miss the cache.
func externalBitmapKey(cache subtitleCache, path, subPath string) string {
	k := cache.keyForKind(path, 0, subtitleKindPGS)
	if k == "" || subPath == "" {
		return k
	}
	ks := cache.keyFor(subPath, 0)
	if ks == "" {
		return ""
	}
	h := sha256.Sum256([]byte(k + "\x00" + ks))
	return string(subtitleKindPGS) + "-" + hex.EncodeToString(h[:])
}

// serveExternalBitmapSubtitle answers the subtitle route for a sidecar picture
// subtitle with the decoded-image blob, cached like an embedded track's: a
// full film's .sup takes seconds to decode, not worth paying per request.
func serveExternalBitmapSubtitle(w http.ResponseWriter, r *http.Request, cache subtitleCache, path, baseURL string) {
	var subPath string
	if strings.EqualFold(filepath.Ext(path), ".idx") {
		p, ok := vobSubPairSub(path)
		if !ok {
			writeSubtitleFailed(w, r, baseURL, "the .idx has no .sub beside it")
			return
		}
		subPath = p
	}
	key := externalBitmapKey(cache, path, subPath)
	if data, ok := cache.get(key); ok {
		writePGSBlob(w, r, data)
		return
	}
	if reason, failed := cache.getFailure(key); failed {
		writeSubtitleFailed(w, r, baseURL, reason)
		return
	}
	// One decode however many viewers or retries ask at once, remembered when
	// it fails, as an embedded track's extraction is.
	work := func(ctx context.Context) ([]byte, error) {
		if data, ok := cache.get(key); ok {
			return data, nil
		}
		data, err := extractExternalBitmapBlob(ctx, path, subPath)
		if err != nil {
			if fErr := cache.putFailure(key, err); fErr != nil {
				slog.WarnContext(ctx, "subtitle: external picture subtitle failure memo store failed",
					slog.String("path", path), slog.Any("err", fErr))
			}
			return nil, err
		}
		if cErr := cache.put(key, data); cErr != nil {
			slog.WarnContext(ctx, "subtitle: external picture subtitle cache store failed",
				slog.String("path", path), slog.Any("err", cErr))
		}
		return data, nil
	}
	data, err := runSharedSubtitleExtraction(r.Context(), subtitleFlightKey(key, path), path, work)
	if err != nil {
		slog.WarnContext(r.Context(), "subtitle: external picture subtitle decode failed",
			slog.String("path", path), slog.Any("err", err))
		writeSubtitleFailed(w, r, baseURL, err.Error())
		return
	}
	writePGSBlob(w, r, data)
}
