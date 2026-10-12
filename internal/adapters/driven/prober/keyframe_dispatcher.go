package prober

import (
	"context"

	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mp4"

	mkvadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// KeyframeProber is the slow ffprobe packet-scan this dispatcher wraps as its
// fallback (scans every packet header - tens of seconds to minutes per film).
type KeyframeProber interface {
	ProbeKeyframes(ctx context.Context, filePath string) ([]int64, error)
}

// KeyframeDispatcher returns a video file's keyframe presentation times (ms).
// When the matching fast path is enabled it reads the container's own seek index
// head-only via mkvgo - MP4 stss (edit-list-adjusted) / Matroska Cues - turning
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
		// WithKeyframeIndex: use the Cues table when present (head-only, ~ms), else
		// fall to a single sequential structural scan that collects every real video
		// keyframe (SimpleBlock 0x80 / BlockGroup without ReferenceBlock) - complete
		// and equal to ffprobe's list, validated count==ffprobe on real files. No-op
		// when Cues exist, so indexed files stay instant; Cues-less files get the
		// in-process complete index instead of forking ffprobe.
		c, err := reader.OpenMeta(ctx, filePath, reader.WithKeyframeIndex())
		if err != nil || c == nil {
			return nil
		}
		return c.Keyframes
	}
	return nil
}

// Engine names reported by ProbeKeyframesWithEngine. They are what the admin
// log badge shows, so they must name the thing that actually did the work.
const (
	EngineMkvgo   = "mkvgo"
	EngineFFprobe = "ffmpeg"
)

// ProbeKeyframesWithEngine is ProbeKeyframes plus the name of the engine that
// actually served the call. The dispatcher is the only layer that knows - it
// picks the fast path or the fallback at runtime - and without this the caller
// could only guess from its own component name, which is exactly how the admin
// log ended up badging pure-mkvgo work as "ffmpeg".
//
// Kept as a SEPARATE method rather than widening KeyframeProber: the port has
// other implementations (the raw ffprobe prober, test fakes) that have nothing
// to report, and a caller can simply type-assert for this.
func (d *KeyframeDispatcher) ProbeKeyframesWithEngine(ctx context.Context, filePath string) ([]int64, string, error) {
	ctx, span := tracing.StartSpan(ctx, "prober.keyframe.dispatch")
	defer span.End()
	if kf := d.mkvgoKeyframes(ctx, filePath); len(kf) > 0 {
		span.AddEvent("keyframes-fastpath")
		return kf, EngineMkvgo, nil
	}
	span.AddEvent("keyframes-ffprobe-fallback")
	kf, err := d.fallback.ProbeKeyframes(ctx, filePath)
	return kf, EngineFFprobe, err
}
