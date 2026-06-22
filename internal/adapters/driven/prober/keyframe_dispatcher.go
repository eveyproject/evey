package prober

import (
	"context"

	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mp4"

	mkvadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// KeyframeProber is the slow ffprobe packet-scan this dispatcher wraps as its
// fallback (scans every packet header — tens of seconds to minutes per film).
type KeyframeProber interface {
	ProbeKeyframes(ctx context.Context, filePath string) ([]int64, error)
}

// KeyframeDispatcher returns a video file's keyframe presentation times (ms).
// When the matching fast path is enabled it reads the container's own seek index
// head-only via mkvgo — MP4 stss (edit-list-adjusted) / Matroska Cues — turning
// a multi-minute packet scan into a few-millisecond head read. It falls back to
// the ffprobe packet scan whenever the fast path is gated off, the container is
// unsupported, the parse fails or panics, or the container carries no seek index
// (a sparse/absent Cues table). Every timestamp mkvgo returns is a real
// keyframe, so a -c:v copy segment can start there; the fast path is an
// optimization, never a correctness requirement. Gated by the same funcs as the
// metadata Dispatcher: keyframes are trusted to mkvgo exactly where its metadata
// already is.
type KeyframeDispatcher struct {
	fallback   KeyframeProber
	mkvEnabled func() bool
	mp4Enabled func() bool
}

var _ KeyframeProber = (*KeyframeDispatcher)(nil)

// NewKeyframeDispatcher wraps the ffprobe fallback. mkvEnabled/mp4Enabled gate
// each container family's fast path (nil == always on).
func NewKeyframeDispatcher(fallback KeyframeProber, mkvEnabled, mp4Enabled func() bool) *KeyframeDispatcher {
	return &KeyframeDispatcher{fallback: fallback, mkvEnabled: mkvEnabled, mp4Enabled: mp4Enabled}
}

func (d *KeyframeDispatcher) ProbeKeyframes(ctx context.Context, filePath string) ([]int64, error) {
	ctx, span := tracing.StartSpan(ctx, "prober.keyframe.dispatch")
	defer span.End()
	if kf := d.mkvgoKeyframes(ctx, filePath); len(kf) > 0 {
		span.AddEvent("keyframes-fastpath")
		return kf, nil
	}
	span.AddEvent("keyframes-ffprobe-fallback")
	return d.fallback.ProbeKeyframes(ctx, filePath)
}

// mkvgoKeyframes reads the seek index head-only, or returns nil to signal the
// caller to fall back to the ffprobe packet scan. A malformed/adversarial
// container must never crash the backfill worker, so an in-process parse panic
// is absorbed here (mkvgo is fuzz-tested and fail-soft, but the fast path is
// never required).
func (d *KeyframeDispatcher) mkvgoKeyframes(ctx context.Context, filePath string) (kf []int64) {
	defer func() {
		if r := recover(); r != nil {
			kf = nil
		}
	}()
	switch {
	case mkvadapter.IsMP4Container(filePath) && gateOpen(d.mp4Enabled):
		c, _, err := mp4.OpenMeta(ctx, filePath, mp4.Options{Keyframes: true})
		if err != nil || c == nil {
			return nil
		}
		return c.Keyframes
	case mkvadapter.IsEBMLContainer(filePath) && gateOpen(d.mkvEnabled):
		c, err := reader.OpenMeta(ctx, filePath) // Matroska fills Keyframes from Cues
		if err != nil || c == nil {
			return nil
		}
		return c.Keyframes
	}
	return nil
}
