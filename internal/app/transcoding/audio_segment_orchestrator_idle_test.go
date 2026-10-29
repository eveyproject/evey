package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
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
	}, eventuallyWait, 5*time.Millisecond, "an unrequested rendition must be reaped")
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

// The init is written by whichever session is live; with none, the session
// must start at the viewer's position, not at 0 (prod 2026-09-10: a wasted
// encoder at 0 on every track switch).
func TestAudioOrchestrator_InitStartsAtTheHintedSegment(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioInit(t, cache, fp)
		return newAudioFakeHandle()
	}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioInitAt(context.Background(), fp, 130))
	require.Equal(t, 1, proc.startCount())
	assert.Equal(t, 130, proc.optsAt(0).StartNumber, "the init session starts where the viewer is")

	// The segment request at that position rides the same session.
	writeAudioSegment(t, cache, fp, 130)
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 130))
	assert.Equal(t, 1, proc.startCount(), "no second session for the position the init already started at")
}

func TestAudioOrchestrator_PreWarmStartsTheSessionInTheBackground(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	o.PreWarmAudio(fp, 42)
	require.Eventually(t, func() bool { return proc.startCount() == 1 }, eventuallyWait, 5*time.Millisecond)
	assert.Equal(t, 42, proc.optsAt(0).StartNumber)
	o.PreWarmAudio(fp, 44) // within lookahead of the live session: nothing new
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, proc.startCount())
}

// The per-hash index is what every served segment walks; it must forget a
// session exactly when the flat map does (reap) and be emptied by Close, or a
// dead rendition would keep looking covered and never respawn.
func TestAudioOrchestrator_HashIndexStaysInStepWithSessions(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	proc.onStart = func(transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, 0)
		return newAudioFakeHandle()
	}
	o := newAudioSegmentOrchestrator(cache, proc, 30*time.Millisecond, 5*time.Millisecond)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))
	hash := fp.Hash()
	o.mu.Lock()
	indexed := len(o.byHash[hash])
	covered := o.coveringSessionLocked(hash, 0)
	o.mu.Unlock()
	require.Equal(t, 1, indexed, "a spawned session is indexed under its hash")
	require.True(t, covered, "the index is what answers 'is this segment covered'")

	require.Eventually(t, func() bool { return o.ActiveCount() == 0 }, eventuallyWait, 5*time.Millisecond)
	o.mu.Lock()
	_, still := o.byHash[hash]
	covered = o.coveringSessionLocked(hash, 0)
	o.mu.Unlock()
	assert.False(t, still, "a reaped session leaves no index entry behind")
	assert.False(t, covered, "a reaped rendition is no longer covered, so the next miss respawns")

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))
	require.NoError(t, o.Close())
	o.mu.Lock()
	defer o.mu.Unlock()
	assert.Empty(t, o.byHash, "Close empties the index with the sessions")
	assert.Empty(t, o.sessions)
}
