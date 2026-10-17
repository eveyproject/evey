package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeProbeScript drops a tiny shell "ffprobe" that ignores its args and
// echoes fakeJSON - the same fake-binary technique used by prober_test.go, so
// VideoCompositionDelaySec can be exercised without a real ffprobe.
func writeFakeProbeScript(t *testing.T, fakeJSON string) string {
	t.Helper()
	return writeFakeBinary(t, filepath.Join(t.TempDir(), "ffprobe"), "#!/bin/sh\necho '"+fakeJSON+"'\n")
}

// videoCompositionDelayCache must stay bounded exactly like its sibling
// audioDelayCache: on a NAS running for weeks against a library whose files
// get replaced or re-encoded, one entry per stale (path,size,mtime) identity
// kept forever is an unbounded leak. Seeding the counter at the cap and
// proving a pre-existing entry gets dropped on the next Store is the fast way
// to exercise the eviction without actually inserting 8192 entries.
func TestVideoCompositionDelayCache_BoundedLikeSibling(t *testing.T) {
	videoCompositionDelayCache.Clear()
	videoCompositionDelayCacheLen.Store(0)
	t.Cleanup(func() {
		videoCompositionDelayCache.Clear()
		videoCompositionDelayCacheLen.Store(0)
	})

	videoCompositionDelayCache.Store("stale-key", 0.042)
	videoCompositionDelayCacheLen.Store(videoCompositionDelayCacheCap)

	script := writeFakeProbeScript(t, `{"streams":[{"has_b_frames":1,"avg_frame_rate":"24000/1001"}]}`)
	p := filepath.Join(t.TempDir(), "movie.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))

	got := VideoCompositionDelaySec(context.Background(), script, p)
	assert.Greater(t, got, 0.0, "sanity: the probe itself must still work")

	if _, ok := videoCompositionDelayCache.Load("stale-key"); ok {
		t.Fatal("videoCompositionDelayCache grew past its cap without evicting - unbounded growth (the leak this test guards against)")
	}
}

func TestParseAudioStartDelay(t *testing.T) {
	// Shaped like `ffprobe -show_entries stream=codec_type,start_time:
	// stream_disposition=attached_pic -of json` on a delayed-audio repack with
	// a cover-art stream first and two audio tracks.
	out := []byte(`{"streams":[
		{"codec_type":"video","start_time":"0.000000","disposition":{"attached_pic":1}},
		{"codec_type":"video","start_time":"0.005000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.900000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"N/A","disposition":{"attached_pic":0}},
		{"codec_type":"subtitle","disposition":{"attached_pic":0}}
	]}`)

	d, err := parseAudioStartDelay(out, 0)
	require.NoError(t, err)
	assert.InDelta(t, 0.895, d, 1e-9, "delay = audio start - REAL video start (cover art skipped)")

	// Track 1 has no usable start_time: no evidence of a delay -> 0, nil.
	d, err = parseAudioStartDelay(out, 1)
	require.NoError(t, err)
	assert.Zero(t, d)

	// Track index beyond the stream list: same "nothing to cancel" outcome.
	d, err = parseAudioStartDelay(out, 5)
	require.NoError(t, err)
	assert.Zero(t, d)

	// Malformed JSON is a real error (the caller logs and fails soft).
	_, err = parseAudioStartDelay([]byte("not json"), 0)
	assert.Error(t, err)
}

func TestParseMaxAudioStartDelay(t *testing.T) {
	out := []byte(`{"streams":[
		{"codec_type":"video","start_time":"0.000000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.300000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.900000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"N/A","disposition":{"attached_pic":0}}
	]}`)
	d, err := parseMaxAudioStartDelay(out)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, d, 1e-9, "max across audio tracks")

	// Audio EARLIER than video must not go negative (0 = nothing to flag).
	early := []byte(`{"streams":[
		{"codec_type":"video","start_time":"1.000000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.000000","disposition":{"attached_pic":0}}
	]}`)
	d, err = parseMaxAudioStartDelay(early)
	require.NoError(t, err)
	assert.Zero(t, d)

	// No video (music file): nothing to measure against.
	audioOnly := []byte(`{"streams":[
		{"codec_type":"audio","start_time":"0.500000","disposition":{"attached_pic":0}}
	]}`)
	d, err = parseMaxAudioStartDelay(audioOnly)
	require.NoError(t, err)
	assert.Zero(t, d)
}
