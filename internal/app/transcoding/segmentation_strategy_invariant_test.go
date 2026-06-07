package transcoding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// These tests pin the runs-map invariant: the map tracks ONLY in-flight muxes;
// the DISK (a complete playlist with #EXT-X-ENDLIST) is the source of truth. A
// terminal state — start failure, mid-mux crash, or completion-then-eviction —
// is NEVER cached, so the hash can never be poisoned into a permanent 503/504.

func newWholeFileTest(t *testing.T) (*WholeFileStrategy, *fakeProcess) {
	t.Helper()
	cache, err := NewSegmentCache(t.TempDir())
	require.NoError(t, err)
	proc := newFakeProcess()
	ws := NewWholeFileStrategy(cache, proc, stubSource{path: "/media/x.mkv"}, nil)
	t.Cleanup(ws.Close)
	return ws, proc
}

func wfFingerprint() DecisionFingerprint {
	return DecisionFingerprint{
		MediaID:    "m1",
		SourcePath: "/media/x.mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
		UseFMP4:    true,
	}
}

// waitRunGone polls until the background goroutine has dropped the in-flight
// marker for hash (it deletes on any exit), so the next EnsureSegment re-derives
// from disk rather than racing the still-registered run.
func waitRunGone(t *testing.T, ws *WholeFileStrategy, hash string) {
	t.Helper()
	for range 400 {
		ws.mu.Lock()
		_, ok := ws.runs[hash]
		ws.mu.Unlock()
		if !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("in-flight run for %s never removed from the map", hash)
}

// A mux that ends WITHOUT writing #EXT-X-ENDLIST (mid-mux crash / shutdown) must
// re-spawn on the next request — not be cached as "started-OK-done".
func TestWholeFile_IncompleteMuxRespawns(t *testing.T) {
	ws, proc := newWholeFileTest(t)
	fp := wfFingerprint()
	// failOnExit closes the handle's Done immediately, but the fake writes no
	// playlist — so the disk holds no ENDLIST = an incomplete mux.
	proc.failOnExit = transcoding.FailureTransient

	require.NoError(t, ws.EnsureSegment(context.Background(), fp, 0))
	waitRunGone(t, ws, fp.Hash())
	require.NoError(t, ws.EnsureSegment(context.Background(), fp, 0))
	waitRunGone(t, ws, fp.Hash())

	assert.Equal(t, 2, proc.startCount(),
		"an incomplete mux (no ENDLIST) must re-spawn, never poison the hash")
}

// A complete playlist on disk must short-circuit straight to cache hits, never
// re-spawning ffmpeg.
func TestWholeFile_CompletePlaylistShortCircuits(t *testing.T) {
	ws, proc := newWholeFileTest(t)
	fp := wfFingerprint()
	dir, err := ws.cache.DirFor(fp.Hash())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0.m3u8"),
		[]byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0o644))

	require.NoError(t, ws.EnsureSegment(context.Background(), fp, 0))

	assert.Equal(t, 0, proc.startCount(),
		"a complete playlist (ENDLIST) must short-circuit from disk, never spawn ffmpeg")
}

// A transient start failure (NAS unmount, EMFILE) must retry on the next
// request, never cache the error until process restart.
func TestWholeFile_StartFailureRetries(t *testing.T) {
	ws, proc := newWholeFileTest(t)
	fp := wfFingerprint()
	proc.startErr = errors.New("transient spawn failure")

	require.Error(t, ws.EnsureSegment(context.Background(), fp, 0))
	waitRunGone(t, ws, fp.Hash())
	require.Error(t, ws.EnsureSegment(context.Background(), fp, 0))
	waitRunGone(t, ws, fp.Hash())

	assert.Equal(t, 2, proc.startCount(),
		"a transient start failure must retry, never poison the hash until restart")
}
