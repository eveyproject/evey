package prober

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mp4"

	mkvadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// metaReader is the slice of mkvgo the dispatcher depends on (so tests can
// inject a fake without touching the filesystem or the real library). It covers
// both container families mkvgo can read head-only: EBML (.mkv/.webm) and
// ISO-BMFF (.mp4/.m4v/.mov). ReadMeta returns the same mkv.Container shape for both
// (mp4.OpenMeta builds Matroska-shaped metadata), so a single mapper serves
// them.
type metaReader interface {
	IsEBML(path string) bool
	IsMP4(path string) bool
	ReadMeta(ctx context.Context, path string) (*extmkv.Container, error)
	// TrackEndMs measures where an EBML file's content ends, for a file whose
	// header declares no duration (see mkvadapter.TrackEndMs).
	TrackEndMs(ctx context.Context, path string) (int64, error)
}

// Dispatcher implements library.StreamProber. For a container mkvgo can read
// head-only (EBML .mkv/.webm, or ISO-BMFF .mp4/.m4v/.mov) it reads metadata
// in-process via mkvgo OpenMeta and maps it to a ProbeResult; for everything
// else - and for any file it cannot map field-equivalent - it delegates to
// fallback (the ffprobe prober, the authority). The fast-path is a fork-free,
// head-only optimization, never a correctness requirement.
type Dispatcher struct {
	fallback library.StreamProber
	meta     metaReader
	// mkvEnabled / mp4Enabled gate each fast-path independently (nil == always
	// on). Both container families are validated field-equivalent against ffprobe
	// on the real library - EBML, and MP4 via parity_realfile_test - so production
	// wires BOTH gates to nil (always on) with ffprobe as the safety net (see
	// wire.go). The two func hooks remain only as a per-family kill switch for
	// tests/debugging; there is no production env toggle. When a gate returns
	// false the dispatcher is a pure pass-through for that container family.
	mkvEnabled func() bool
	mp4Enabled func() bool
}

var _ library.StreamProber = (*Dispatcher)(nil)

// New builds a Dispatcher over the ffprobe fallback, using the real mkvgo
// readers. mkvEnabled/mp4Enabled gate each fast-path (nil == always on).
func New(fallback library.StreamProber, mkvEnabled, mp4Enabled func() bool) *Dispatcher {
	return &Dispatcher{fallback: fallback, meta: realMetaReader{}, mkvEnabled: mkvEnabled, mp4Enabled: mp4Enabled}
}

func gateOpen(f func() bool) bool { return f == nil || f() }

func (d *Dispatcher) ProbeStreams(ctx context.Context, filePath string) (res *library.ProbeResult, err error) {
	ctx, span := tracing.StartSpan(ctx, "prober.dispatch")
	defer span.End()

	if d.meta == nil {
		return d.fallback.ProbeStreams(ctx, filePath)
	}
	// Take the fast path only when the container family is recognized AND its own
	// gate is open; otherwise ffprobe (the authority) handles the file.
	switch {
	case d.meta.IsEBML(filePath) && gateOpen(d.mkvEnabled):
	case d.meta.IsMP4(filePath) && gateOpen(d.mp4Enabled):
	default:
		return d.fallback.ProbeStreams(ctx, filePath)
	}

	// Defense-in-depth at the untrusted-input boundary: a malformed/adversarial
	// container must never crash the scan worker. mkvgo is fuzz-tested and
	// fail-soft, but if an in-process parse ever panics we absorb it here and let
	// ffprobe (the authority) handle the file - the fast-path is never required.
	defer func() {
		if r := recover(); r != nil {
			span.AddEvent("meta-panic: ffprobe fallback")
			slog.Warn("prober: mkvgo fast-path panicked, falling back to ffprobe",
				slog.String("file", filePath), slog.Any("panic", r))
			res, err = d.fallback.ProbeStreams(ctx, filePath)
		}
	}()

	c, err := d.meta.ReadMeta(ctx, filePath)
	if err != nil {
		span.AddEvent("meta-readmeta-error: ffprobe fallback")
		return d.fallback.ProbeStreams(ctx, filePath)
	}
	ebml := d.meta.IsEBML(filePath)
	res, ok := mapContainer(c, ebml)
	if !ok {
		span.AddEvent("meta-unmappable: ffprobe fallback")
		return d.fallback.ProbeStreams(ctx, filePath)
	}
	if ebml && res.Duration.IsZero() {
		// A Matroska written as a live stream declares no Duration; ffprobe
		// reads none either, and a file without a duration is refused at
		// playback ("duration missing"). Measure it from the tracks' ends
		// instead - a bounded, header-only read of the tail.
		if endMs, err := d.meta.TrackEndMs(ctx, filePath); err != nil {
			slog.Warn("prober: duration not declared and the tracks' ends could not be read",
				slog.String("file", filePath), slog.Any("err", err))
		} else if dur, derr := shared.NewDurationMs(endMs); derr == nil && !dur.IsZero() {
			res.Duration = dur
			span.AddEvent("meta-duration-measured")
			slog.Info("prober: duration not declared, measured from the tracks' ends",
				slog.String("file", filePath), slog.Int64("duration_ms", endMs))
		}
	}
	span.AddEvent("meta-fastpath")
	return res, nil
}

// realMetaReader is the production metaReader backed by the mkvgo library: EBML
// via the Matroska reader, ISO-BMFF via the mp4 package - both head-only.
type realMetaReader struct{}

func (realMetaReader) IsEBML(path string) bool { return mkvadapter.IsEBMLContainer(path) }
func (realMetaReader) IsMP4(path string) bool  { return mkvadapter.IsMP4Container(path) }
func (realMetaReader) TrackEndMs(ctx context.Context, path string) (int64, error) {
	return mkvadapter.TrackEndMs(ctx, path)
}

func (realMetaReader) ReadMeta(ctx context.Context, path string) (*extmkv.Container, error) {
	if mkvadapter.IsMP4Container(path) {
		return readMP4Meta(ctx, path)
	}
	// WithInBandColourFallback: same bare-hvcC HEVC recovery for Matroska - lets
	// HDR10/HLG/PQ in-band titles fast-path instead of bailing to ffprobe; the
	// mapper's no-colour gate is the last resort if even this yields nothing.
	// WithBitrate: follow the head SeekHead to the Tags element for the per-track
	// BPS bitrate (one bounded seek, still head-only) so a fast-pathed MKV carries
	// the same Bitrate the ffprobe fallback reads from TAG:BPS.
	c, err := reader.OpenMeta(ctx, path, reader.WithInBandColourFallback(), reader.WithBitrate())
	if errors.Is(err, reader.ErrNotMatroska) {
		// A .mkv/.webm extension on an ISO-BMFF (MP4) payload - a mislabeled file.
		// mkvgo typed the input rather than returning a cryptic EBML error, so read
		// it as the mp4 it actually is instead of forking ffprobe for it.
		return readMP4Meta(ctx, path)
	}
	return c, err
}

// readMP4Meta reads ISO-BMFF metadata head-only (no sample table); keyframes are
// derived on demand by the keyframe backfill worker via mp4.Options{Keyframes:
// true}. InBandColour recovers colour for bare-hvcC HEVC (no SPS in the sample
// description): a bounded first-sample read extracts the SPS VUI + the HLG ATC
// SEI. Only fires when head-only colour is missing, so files with header colour
// stay pure head-only.
func readMP4Meta(ctx context.Context, path string) (*extmkv.Container, error) {
	c, dropped, err := mp4.OpenMeta(ctx, path, mp4.Options{InBandColour: true})
	return gateMP4Drops(c, dropped, err)
}

// gateMP4Drops decides whether an mp4.OpenMeta result is safe to fast-path given
// the tracks the reader could not surface. A dropped VIDEO or AUDIO track (e.g.
// an s263/h263 sample entry in an old camera mp4) means the file holds more
// media than the returned container shows - scanning it would record the file
// as audio-only and the playback decision would be wrong - so those files go to
// ffprobe (the authority) instead. Benign drops keep the fast-path: non-media
// handlers (hint/timecode/metadata, Type == "") and subtitle drops (QuickTime
// chapter text - surfaced as Chapters, and the subtitle serve path has its own
// codec guard with ffmpeg fallback).
func gateMP4Drops(c *extmkv.Container, dropped []mp4.DroppedTrack, err error) (*extmkv.Container, error) {
	if err != nil {
		return nil, err
	}
	for _, d := range dropped {
		if d.Type == extmkv.VideoTrack || d.Type == extmkv.AudioTrack {
			return nil, fmt.Errorf("mp4 fast-path: dropped %s track %d (%s): %s - deferring to ffprobe",
				d.Type, d.ID, d.Codec, d.Reason)
		}
	}
	return c, nil
}
