package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// A rendition nobody asks for dies on the inactivity fuse (prod 2026-09-09:
// orphaned AAC encoders read a 70 GB film to the end).
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

// Steady playback is all cache hits (the encoder runs ahead): they must count.
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

// A session started ahead of the request is an abandoned forward seek.
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
