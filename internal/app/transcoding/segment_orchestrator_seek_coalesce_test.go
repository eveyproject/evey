package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The seek-coalescing feature (opts.SeekCoalesceWindow > 0) debounces the
// unserved-seek spawn path so a burst of rapid seeks on one fingerprint spawns
// ONE encoder at the final position instead of one per seek that the next seek
// LRU-evicts before it writes its first segment (prod 2026-09-15: a heavy rung
// under rapid seeking = a storm of kills + 503s and wasted encode work). These
// tests pin the before/after: spawns, kills (startCount - survivors), the last
// seek producing, and the limiter draining to zero with no zombie session.

func (o *SegmentOrchestrator) pendingSeekCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.pendingSeeks)
}

// A SINGLE seek still spawns exactly one rafale at its position - only deferred
// by the window (~tens of ms), which is under a fragment RTT and invisible in
// practice. The spawn must NOT be synchronous with the call (that is the whole
// debounce), and it must land at the requested segment.
func TestOrchestrator_SeekCoalesce_SingleSeekSpawnsOnceAtPosition(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekCoalesceWindow = 60 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 100))
	// Deferred: nothing has exec'd ffmpeg yet the instant the call returns.
	assert.Equal(t, 0, proc.startCount(), "the spawn must be debounced, not synchronous")
	assert.Equal(t, 1, o.pendingSeekCount(), "the seek must be parked pending the window")

	require.Eventually(t, func() bool { return proc.startCount() == 1 }, time.Second, 5*time.Millisecond,
		"one rafale must spawn once the window goes quiet")
	// Give any (erroneous) second spawn a chance to appear.
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, 1, proc.startCount(), "a single seek is a single spawn")
	assert.Equal(t, 1, o.ActiveCount())
	assert.Equal(t, 0, o.pendingSeekCount(), "the pending entry is cleared once it fires")
	assert.Equal(t, 100, proc.lastOpts().StartNumber, "the rafale must start at the requested segment")
}

// A BURST of rapid distant seeks collapses to ONE spawn at the LAST target.
// This is the fix: no encoder is started (then killed) for the intermediate
// positions the viewer flew past.
func TestOrchestrator_SeekCoalesce_BurstCollapsesToOneSpawnAtLastTarget(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(8)
	opts := testOpts()
	opts.SeekCoalesceWindow = 150 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	o.WithVideoSessionLimiter(limiter)
	defer o.Close()

	fp := minimalFingerprint(t)

	// 12 seeks spaced beyond a rafale's [start, start+LookAhead+nearAhead] reach,
	// so each would be a genuinely uncovered spawn with coalescing OFF. Fired in
	// a tight loop, they all land inside the debounce window.
	const seeks = 12
	var lastSeg int
	for i := range seeks {
		lastSeg = 100 + i*100
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, lastSeg))
	}

	require.Eventually(t, func() bool { return proc.startCount() >= 1 }, 2*time.Second, 5*time.Millisecond,
		"the coalesced spawn must eventually fire")
	// Let any extra spawn surface before asserting exactly one.
	time.Sleep(2 * opts.SeekCoalesceWindow)

	assert.Equal(t, 1, proc.startCount(), "a burst of %d seeks must exec ONE ffmpeg, not one per seek", seeks)
	assert.Equal(t, 1, o.ActiveCount(), "exactly one live rafale, no zombies from killed-before-produce")
	assert.Equal(t, 0, o.pendingSeekCount())
	assert.Equal(t, lastSeg, proc.lastOpts().StartNumber, "last target wins: the surviving rafale starts at the final seek")
	assert.Equal(t, 1, limiter.Active(), "one encode slot held for the single survivor")

	// Stopping playback (Close) must drain the limiter fully: no slot leak.
	o.Close()
	assert.Equal(t, 0, limiter.Active(), "the limiter returns to zero once the session is torn down")
	assert.InDelta(t, 0.0, limiter.Used(), 1e-9, "no reserved cost leaks past teardown")
}

// The baseline the feature replaces: with the window OFF, the SAME burst spawns
// one encoder per seek and LRU-evicts all but MaxRafalesPerHash - a wall of
// spawns and kills. This is the "avant" the fix removes.
func TestOrchestrator_SeekCoalesce_Off_BurstSpawnsAndKillsPerSeek(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekCoalesceWindow = 0 // OFF: pre-feature behaviour
	opts.MaxRafalesPerHash = 3
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	const seeks = 12
	for i := range seeks {
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 100+i*100))
	}

	assert.Equal(t, seeks, proc.startCount(), "window off: every seek execs its own ffmpeg")
	require.Eventually(t, func() bool { return o.ActiveCount() == opts.MaxRafalesPerHash }, time.Second, 5*time.Millisecond,
		"the cap evicts down to MaxRafalesPerHash")
	// kills = spawns - survivors: 12 - 3 = 9 encoders started then killed before
	// most produced a segment. That is the wasted work + 503 storm.
	assert.Equal(t, seeks-opts.MaxRafalesPerHash, proc.startCount()-o.ActiveCount(),
		"the burst kills every over-cap encoder")
}

// A seek that lands AFTER the timer was already scheduled must push the window
// out: the surviving spawn is the truly-last one, even when seeks straddle the
// window boundary. Guards the deadline re-arm in fireCoalescedSeek.
func TestOrchestrator_SeekCoalesce_LateSeekPushesTheWindow(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	// Wide against the scheduler: under -race on a loaded box a 30 ms sleep
	// overran a 60 ms window, the first timer fired and the test saw 2 spawns.
	opts.SeekCoalesceWindow = 300 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 100))
	// Land a second seek at ~half the window, before the first timer fires: it
	// must move the target AND the deadline, not spawn a second encoder.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 500))

	require.Eventually(t, func() bool { return proc.startCount() == 1 }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(400 * time.Millisecond)
	assert.Equal(t, 1, proc.startCount(), "the late seek re-armed the window, it did not add a spawn")
	assert.Equal(t, 500, proc.lastOpts().StartNumber, "the later seek is the one that wins")
}

// Once the coalesced rafale is live and producing, a follow-on request WITHIN
// its reach rides it - the debounce is only for the uncovered spawn, never for a
// continuation. This proves a coalesced spawn does not wedge the hash: normal
// serving resumes immediately after it lands.
func TestOrchestrator_SeekCoalesce_LiveRafaleServedImmediately(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekCoalesceWindow = 60 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	// Coalesced spawn at 200 -> a rafale covering [200, 200+LookAhead].
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 200))
	require.Eventually(t, func() bool { return o.ActiveCount() == 1 }, time.Second, 5*time.Millisecond)
	require.Equal(t, 1, proc.startCount())

	// A request just ahead, inside the live rafale's window, is served by it -
	// no new pending seek, no second encoder.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 201))
	assert.Equal(t, 0, o.pendingSeekCount(), "a covered request never enters the debounce")
	assert.Equal(t, 1, proc.startCount(), "the live rafale is ridden, not respawned")
	assert.Equal(t, 1, o.ActiveCount())
}

// A seek the host has no encode slot for must be refused at once, not parked
// behind the debounce: the deferred spawn's refusal only reached a log, and the
// viewer waited out the whole segment budget instead of getting a 503.
func TestOrchestrator_SeekCoalesce_AtCapacity_ReturnsCapacityErrorToCaller(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekCoalesceWindow = 60 * time.Millisecond
	limiter := NewVideoSessionLimiter(1)
	require.True(t, limiter.TryAcquire("another-stream", LadderClassSoftware, 720))
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger()).
		WithVideoSessionLimiter(limiter)
	defer o.Close()

	err := o.EnsureSegmentRange(context.Background(), minimalFingerprint(t), 0, 100)
	require.ErrorIs(t, err, ErrVideoCapacityExhausted)
	assert.Equal(t, 0, o.pendingSeekCount(), "a refused seek must not stay parked")
	assert.Equal(t, 0, proc.startCount())
}

// In single-variant mode each rung has its own encoder, so two rungs seeking at
// once are two spawns: keying the debounce by hash alone let the second rung
// overwrite the first, whose request then waited on a segment nobody encodes.
func TestOrchestrator_SeekCoalesce_SingleVariant_RungsDoNotOverwriteEachOther(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SingleVariant = true
	opts.SeekCoalesceWindow = 60 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	fp.Profiles = append(fp.Profiles, ProfileFingerprint{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2500, AudioBitrate: 128})
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 100))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 1, 100))

	require.Eventually(t, func() bool { return proc.startCount() == 2 }, time.Second, 5*time.Millisecond,
		"each rung's seek must spawn its own encoder")
}
