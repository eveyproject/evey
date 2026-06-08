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
	p.SetCopyMode(NewCopyPlanner(streams, items, 6000))

	data, err := p.Manifest(context.Background(), DecisionFingerprint{MediaID: "mi-1"})
	require.NoError(t, err)
	m := string(data)
	assert.Contains(t, m, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, m, "stream_0_000.m4s")
	assert.Contains(t, m, "#EXT-X-ENDLIST")
}

// ManifestProvider is implemented by the copy-mode producer (the handler type-
// asserts it on the copy instance only).
var _ ManifestProvider = (*SoftwareProducer)(nil)
