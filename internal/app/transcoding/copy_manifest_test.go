package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCopyManifest_KeyframeVariableExtinf(t *testing.T) {
	segs := []CopySegment{
		{StartMs: 0, EndMs: 6000},
		{StartMs: 6000, EndMs: 10000},
		{StartMs: 10000, EndMs: 12000},
	}
	m := string(buildCopyManifest(segs))
	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-VERSION:6",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXT-X-TARGETDURATION:6", // ceil(max EXTINF = 6.0)
		"#EXT-X-MAP:URI=\"stream_0_init.mp4\"",
		"#EXTINF:6.000,\nstream_0_000.m4s",
		"#EXTINF:4.000,\nstream_0_001.m4s",
		"#EXTINF:2.000,\nstream_0_002.m4s",
		"#EXT-X-ENDLIST",
	} {
		assert.Contains(t, m, want)
	}
}

// End of the chain: plan -> manifest for a source whose first keyframe is NOT
// at 0 (44 ms, the prod shape). The playlist must list exactly the segments
// ffmpeg emits at `-hls_time 0` - one per keyframe, none for the leading
// partial GOP - or the client's time<->segment map is a whole GOP off for the
// entire media (the "seek jumps" report). The last EXTINF must still reach the
// media end, so the playlist duration stays the media duration.
func TestBuildCopyManifest_FirstKeyframeAfterZero(t *testing.T) {
	segs := KeyframeSegments([]int64{44, 3253, 6548}, 10000)
	m := string(buildCopyManifest(segs))
	assert.Contains(t, m, "#EXTINF:3.253,\nstream_0_000.m4s")
	assert.Contains(t, m, "#EXTINF:3.295,\nstream_0_001.m4s")
	assert.Contains(t, m, "#EXTINF:3.452,\nstream_0_002.m4s")
	assert.NotContains(t, m, "stream_0_003.m4s", "no phantom segment for the [0,44ms) lead-in")
	var totalMs int64
	for _, s := range segs {
		totalMs += s.DurationMs()
	}
	assert.EqualValues(t, 10000, totalMs, "the playlist must still span the whole media")
}

func TestBuildCopyManifest_Empty(t *testing.T) {
	m := string(buildCopyManifest(nil))
	assert.Contains(t, m, "#EXTM3U")
	assert.Contains(t, m, "#EXT-X-ENDLIST")
	assert.Contains(t, m, "#EXT-X-TARGETDURATION:0")
}

func TestSoftwareProducer_Manifest_GatedByCopyMode(t *testing.T) {
	cache, err := NewSegmentCache(t.TempDir())
	require.NoError(t, err)
	p := NewSoftwareProducer(cache, nil, nil, SoftwareProducerOptions{}, nil)
	defer p.Close()

	// Not in copy mode → Manifest errors so a misroute fails loud (the transcode
	// instance is reached via the selector and serves the precalculated manifest).
	_, err = p.Manifest(context.Background(), DecisionFingerprint{MediaID: "mi-1"})
	require.Error(t, err)

	// Enable copy mode with a planner backed by fakes (keyframes every 6s, 18s).
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 6000, 12000})}
	items := &fakeCopyItems{item: itemWithDurationMs(18000)}
	p.SetCopyMode(NewCopyPlanner(streams, items), nil)

	data, err := p.Manifest(context.Background(), DecisionFingerprint{MediaID: "mi-1"})
	require.NoError(t, err)
	m := string(data)
	assert.Contains(t, m, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, m, "stream_0_000.m4s")
	assert.Contains(t, m, "#EXT-X-ENDLIST")
}
