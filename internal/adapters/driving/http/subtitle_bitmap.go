package httpapi

// subtitle_bitmap.go - the VobSub (DVD) and DVB bitmap tracks, served in the
// same decoded-image blob as PGS (subtitle_pgs.go) so every player draws them
// with the code it already has.
//
// mkvgo walks a PGS track itself (ForEachSubtitlePGS); for VobSub and DVB it
// offers the block decoders only, so the walk and the cue timing live here.
// There is no stored-index fast path for them: they are rare in a real library
// (a few dozen tracks against several hundred PGS), and the slow walk is
// answered 202 + Retry-After like any long extraction.

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io"
	"os"
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
}

func (a *bitmapCueAssembler) block(timeMs, durMs int64, data []byte) error {
	sets, err := a.dec.Decode(data)
	if err != nil {
		return fmt.Errorf("subtitle block at %d ms: %w", timeMs, err)
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
			cue.EndMs = timeMs + durMs
		default:
			a.open, a.openLimit = &cue, ds.Timeout.Milliseconds()
			continue
		}
		if err := a.emit(cue); err != nil {
			return err
		}
	}
	return nil
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
	return a.emit(cue)
}

// finish flushes a cue the track never closed.
func (a *bitmapCueAssembler) finish() error {
	if a.open == nil {
		return nil
	}
	cue := *a.open
	a.open = nil
	cue.EndMs = cue.StartMs + bitmapDefaultCueMs
	if a.openLimit > 0 {
		cue.EndMs = cue.StartMs + a.openLimit
	}
	return a.emit(cue)
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

// forEachMatroskaBitmapCue walks one VobSub or DVB track of a Matroska file and
// calls fn with each cue, in presentation order.
func forEachMatroskaBitmapCue(ctx context.Context, filePath string, trackID uint64, fn func(matroska.PGSCue) error) error {
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

	f, err := openPacedSubtitleSource(ctx, filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	br, err := matroska.NewBlockReader(f, c.Info.TimecodeScale)
	if err != nil {
		return err
	}
	br.KeepTracks(trackID)

	asm := &bitmapCueAssembler{dec: dec, emit: fn}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		blk, err := br.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		data, err := track.DecodePayload(blk.Data)
		if err != nil {
			return fmt.Errorf("subtitle block at %d ms: %w", blk.Timecode, err)
		}
		if err := asm.block(blk.Timecode, blk.Duration, data); err != nil {
			return err
		}
	}
	return asm.finish()
}

// openPacedSubtitleSource opens the source through the read pacer when one
// applies, so a walk never competes with a stream being served.
func openPacedSubtitleSource(ctx context.Context, filePath string) (extmkv.ReadSeekCloser, error) {
	if opts := subtitleReadOptions(ctx, filePath); len(opts) > 0 && opts[0].FS != nil && opts[0].FS.Open != nil {
		return opts[0].FS.Open(filePath)
	}
	return os.Open(filePath) //nolint:gosec // the path is the library file being served
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

// isPGSCodec reports whether mkvgo's codec name (mkv.Track.Codec) is PGS, the
// one bitmap format mkvgo walks itself and can serve from a stored index.
func isPGSCodec(codec string) bool {
	return codec == "pgs"
}
