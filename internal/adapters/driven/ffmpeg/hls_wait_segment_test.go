package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A seek's session writes into a private dir that its rebase pump empties
// into the cache as soon as a segment is complete. The wait read the private
// dir only, missed segments the pump had already moved, and held the session
// back for its whole timeout while ffmpeg was far ahead.
func TestWaitForSegment_SeesASegmentThePumpAlreadyPublished(t *testing.T) {
	pub := t.TempDir()
	priv := filepath.Join(pub, ".rebase_32_1")
	require.NoError(t, os.MkdirAll(priv, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pub, "stream_0_032.m4s"), []byte("x"), 0o644))

	start := time.Now()
	err := waitForSegment(context.Background(), make(chan struct{}), []string{priv, pub}, "stream_0", 32, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), time.Second)
}

// The cache already holds the segments of the session before the seek: they
// are not this session's first segment.
func TestWaitForSegment_IgnoresSegmentsBeforeTheStart(t *testing.T) {
	pub := t.TempDir()
	for _, n := range []string{"stream_0_000.m4s", "stream_0_013.m4s", "stream_0_032.m4s.tmp", "stream_0_init.mp4"} {
		require.NoError(t, os.WriteFile(filepath.Join(pub, n), []byte("x"), 0o644))
	}
	err := waitForSegment(context.Background(), make(chan struct{}), []string{pub}, "stream_0", 32, 150*time.Millisecond, 10*time.Millisecond)
	require.Error(t, err)
}

func TestSegmentIndex(t *testing.T) {
	for name, want := range map[string]int{"stream_0_032.m4s": 32, "stream_1_000.ts": -1, "stream_0_7.ts": 7} {
		got, ok := segmentIndex(name, "stream_0")
		if want < 0 {
			assert.False(t, ok, name)
			continue
		}
		assert.True(t, ok, name)
		assert.Equal(t, want, got, name)
	}
	_, ok := segmentIndex("stream_0_init.mp4", "stream_0")
	assert.False(t, ok)
}
