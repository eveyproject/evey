package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Prod, 2026-09-09: an audio-track switch left the OLD track's AAC encoder
// running, and the wasm decoder's handover left the relay's running too - each
// reading a 70 GB source at 8× real-time until the end of the film, next to the
// viewer's video rafale. Nothing reaped them: a session only ended when ffmpeg
// did. A rendition nobody asks for must now die on the inactivity fuse.
func TestAudioOrchestrator_IdleRenditionIsReaped(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	var handle *audioFakeHandle
	proc.onStart = func(transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, 0)
		handle = newAudioFakeHandle()
		return handle
	}

	o := newAudioSegmentOrchestrator(cache, proc, 30*time.Millisecond, 5*time.Millisecond)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))
	require.Equal(t, 1, o.ActiveCount())

	// Nobody asks again: the session must be stopped and forgotten.
	require.Eventually(t, func() bool {
		return o.ActiveCount() == 0 && handle != nil && handle.isStopped()
	}, 2*time.Second, 5*time.Millisecond, "an unrequested rendition must be reaped")
	assert.Equal(t, 1, proc.startCount(), "reaping must not respawn anything")
}

func (h *audioFakeHandle) isStopped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
}

// The other half: a rendition that IS being played is served from disk on
// every request (the encoder runs ahead of the player), so the cache-hit path
// has to count as activity or steady playback would look idle.
func TestAudioOrchestrator_CacheHitsKeepTheSessionAlive(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	proc.onStart = func(transcoding.AudioProcessOpts) *audioFakeHandle {
		for i := range 5 {
			writeAudioSegment(t, cache, fp, i)
		}
		return newAudioFakeHandle()
	}

	o := newAudioSegmentOrchestrator(cache, proc, 40*time.Millisecond, 5*time.Millisecond)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))

	// Keep asking, always from cache, for several fuses' worth of time.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 3))
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, 1, o.ActiveCount(), "a rendition served from cache is live, not idle")
	assert.Equal(t, 1, proc.startCount())
}

// A session started AHEAD of the request is a forward seek the viewer walked
// away from: the request behind it must not keep it alive.
func TestAudioOrchestrator_TouchIgnoresSessionsStartedAhead(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}

	o := newAudioSegmentOrchestrator(cache, proc, 40*time.Millisecond, 5*time.Millisecond)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 500)) // far seek: its own session
	require.Equal(t, 2, o.ActiveCount())

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0)) // back at the start
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, 1, o.ActiveCount(), "only the session behind the request survives")
}
