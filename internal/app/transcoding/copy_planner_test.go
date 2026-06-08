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

func TestCopyPlanner_Plan_KeyframeAligned(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 2000, 4000, 6000, 8000, 10000})}
	items := &fakeCopyItems{item: itemWithDurationMs(12000)}
	p := NewCopyPlanner(streams, items, 6000)

	segs, err := p.Plan(context.Background(), "mi-1")
	require.NoError(t, err)
	// targetMs=6000 over 2s GOPs → [0,6000),[6000,12000): boundaries are keyframes,
	// the plan covers [0, duration).
	require.Len(t, segs, 2)
	assert.EqualValues(t, 0, segs[0].StartMs)
	assert.EqualValues(t, 6000, segs[0].EndMs)
	assert.EqualValues(t, 12000, segs[1].EndMs)
}

func TestCopyPlanner_Plan_CachesPerMedia(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 3000, 6000})}
	items := &fakeCopyItems{item: itemWithDurationMs(9000)}
	p := NewCopyPlanner(streams, items, 3000)

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
	p := NewCopyPlanner(streams, items, 6000)
	_, err := p.Plan(context.Background(), "mi-1")
	require.Error(t, err)
}

func TestCopyPlanner_Plan_NoDurationErrors(t *testing.T) {
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 6000})}
	items := &fakeCopyItems{item: itemWithDurationMs(0)}
	p := NewCopyPlanner(streams, items, 6000)
	_, err := p.Plan(context.Background(), "mi-1")
	require.Error(t, err)
}
