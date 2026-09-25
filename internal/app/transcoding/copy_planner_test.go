package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type fakeCopyStreams struct {
	streams []*domainlibrary.MediaStream
	err     error
	calls   int
}

func (f *fakeCopyStreams) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) ([]*domainlibrary.MediaStream, error) {
	f.calls++
	return f.streams, f.err
}

type fakeCopyItems struct {
	item *domainlibrary.MediaItem
	err  error
}

func (f *fakeCopyItems) GetByID(_ context.Context, _ shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	return f.item, f.err
}

func videoStreamWithKeyframes(kf []int64) []*domainlibrary.MediaStream {
	return []*domainlibrary.MediaStream{{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Keyframes: kf}}
}

func itemWithDurationMs(ms int64) *domainlibrary.MediaItem {
	d, _ := shared.NewDurationMs(ms)
	return &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Duration: d}}
}

func TestCopyPlanner_Plan_RebuildsWhenFileReplaced(t *testing.T) {
	// A replaced file (same media id, new size/fingerprint after a re-download
	// or repair) re-probes to DIFFERENT keyframes. The cached plan must not
	// survive the swap: serving the old boundaries cuts -c:v copy segments at
	// offsets that no longer are keyframes.
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 3000, 6000})}
	item := itemWithDurationMs(9000)
	item.File.Size = 1000
	item.File.Fingerprint = "fp-old"
	items := &fakeCopyItems{item: item}
	p := NewCopyPlanner(streams, items)

	first, err := p.Plan(context.Background(), "mi-1")
	require.NoError(t, err)
	require.Len(t, first, 3)

	// Swap the file: new fingerprint/size, new keyframe index, new duration.
	replaced := itemWithDurationMs(12000)
	replaced.File.Size = 2000
	replaced.File.Fingerprint = "fp-new"
	items.item = replaced
	streams.streams = videoStreamWithKeyframes([]int64{0, 2000, 4000, 6000, 8000, 10000})

	second, err := p.Plan(context.Background(), "mi-1")
	require.NoError(t, err)
	assert.Len(t, second, 6, "plan must be rebuilt from the replaced file's keyframes, not served stale")
	assert.Equal(t, 2, streams.calls, "the replaced file must trigger a re-read of the keyframes")
}

func TestCopyPlanner_Plan_KeyframeAligned(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 2000, 4000, 6000, 8000, 10000})}
	items := &fakeCopyItems{item: itemWithDurationMs(12000)}
	p := NewCopyPlanner(streams, items)

	segs, err := p.Plan(context.Background(), "mi-1")
	require.NoError(t, err)
	// Every keyframe is a boundary (the `-hls_time 0` contract): 2s GOPs over
	// 12s → 6 segments covering [0, duration).
	require.Len(t, segs, 6)
	assert.EqualValues(t, 0, segs[0].StartMs)
	assert.EqualValues(t, 2000, segs[0].EndMs)
	assert.EqualValues(t, 12000, segs[5].EndMs)
}

func TestCopyPlanner_Plan_RejectsTooDenseKeyframes(t *testing.T) {
	// All-intra-like index (a keyframe every 200ms): an every-keyframe plan
	// would explode into tiny segments, so the planner must error and the
	// handler falls back to the transcode ladder.
	kf := make([]int64, 0, 60)
	for ms := int64(0); ms < 12000; ms += 200 {
		kf = append(kf, ms)
	}
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes(kf)}
	items := &fakeCopyItems{item: itemWithDurationMs(12000)}
	p := NewCopyPlanner(streams, items)
	_, err := p.Plan(context.Background(), "mi-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too dense")
}

func TestCopyPlanner_Plan_CachesPerMedia(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 3000, 6000})}
	items := &fakeCopyItems{item: itemWithDurationMs(9000)}
	p := NewCopyPlanner(streams, items)

	_, err := p.Plan(context.Background(), "mi-1")
	require.NoError(t, err)
	_, err = p.Plan(context.Background(), "mi-1")
	require.NoError(t, err)
	assert.Equal(t, 1, streams.calls, "second Plan must hit the cache, not re-query the repo")
}

func TestCopyPlanner_Plan_NoKeyframesErrors(t *testing.T) {
	// A media without a keyframe index cannot be cut as copy → error → caller
	// falls back to transcode.
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes(nil)}
	items := &fakeCopyItems{item: itemWithDurationMs(12000)}
	p := NewCopyPlanner(streams, items)
	_, err := p.Plan(context.Background(), "mi-1")
	require.Error(t, err)
}

func TestCopyPlanner_Plan_NoDurationErrors(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 6000})}
	items := &fakeCopyItems{item: itemWithDurationMs(0)}
	p := NewCopyPlanner(streams, items)
	_, err := p.Plan(context.Background(), "mi-1")
	require.Error(t, err)
}

// A source whose first keyframe sits late (an MPEG-TS starts ~1.4 s in) is
// planned from that keyframe, as the copy session re-zeroes its output on it:
// segment k starts at kf_k - kf_0, which is the fragments' own timeline
// (measured: cold fragments at 2k s for keyframes at 1.48 + 2k s). The origin
// comes back so a seek aims in the source's time.
func TestCopyPlanner_Plan_CountsFromTheFirstKeyframe(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{1480, 3480, 5480, 7480})}
	items := &fakeCopyItems{item: itemWithDurationMs(8021)}
	p := NewCopyPlanner(streams, items)

	segs, origin, err := p.PlanWithOrigin(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(1480), origin)
	assert.Equal(t, []CopySegment{{0, 2000}, {2000, 4000}, {4000, 6000}, {6000, 8021}}, segs)
}
