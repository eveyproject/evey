package transcoding

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ============================================================================
// EnsureSegmentRange: spawning, cache hit, race
// ============================================================================

func TestOrchestrator_EnsureSegmentRange_SpawnsFFmpegOnMiss(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	assert.Equal(t, 1, o.ActiveCount())
	assert.Equal(t, 1, proc.startCount())
	// StartSec must match segmentIdx × segmentSeconds.
	assert.InDelta(t, 0.0, proc.lastOpts().StartSec, 0.001)
}

func TestOrchestrator_EnsureSegmentRange_NoSpawnOnCacheHit(t *testing.T) {
	// If the segment is already on disk under the right key, the orchestrator
	// must NOT start a fresh ffmpeg - that's the whole point of the cache.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	k := SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 5}
	require.NoError(t, os.MkdirAll(filepath.Dir(cache.Path(k, fp.UseFMP4)), 0o755))
	require.NoError(t, os.WriteFile(cache.Path(k, fp.UseFMP4), fmp4Seg(), 0o644))

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 5))
	assert.Equal(t, 0, proc.startCount())
}

func TestOrchestrator_EnsureSegmentRange_SecondCallSameWindowNoRespawn(t *testing.T) {
	// A second request inside the window and within reach of the production
	// point must NOT spawn another ffmpeg - just bump lastAccess.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 9}, fp.UseFMP4)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 9))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 10))

	writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 28}, fp.UseFMP4)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 28))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 29))

	assert.Equal(t, 1, proc.startCount(), "all calls fall within [0, 29] AND within reach of the initial rafale")
}

// ============================================================================
// MaxActiveSessions: global concurrency cap (DoS backstop)
// ============================================================================

func TestOrchestrator_MaxActiveSessions_RejectsAtGlobalCap(t *testing.T) {
	// Global cap = 2: a third DISTINCT-media spawn is refused with
	// ErrCapacityExceeded even though each media is well under MaxRafalesPerHash.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.MaxActiveSessions = 2
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fpFor := func(id string) DecisionFingerprint {
		f := minimalFingerprint(t)
		f.MediaID = id
		return f
	}

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fpFor("media-1"), 0, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fpFor("media-2"), 0, 0))
	require.Equal(t, 2, o.ActiveCount(), "two distinct media -> two live rafales")

	err := o.EnsureSegmentRange(context.Background(), fpFor("media-3"), 0, 0)
	require.ErrorIs(t, err, ErrCapacityExceeded, "third spawn must be refused at the global cap")
	assert.Equal(t, 2, o.ActiveCount(), "rejected spawn must not leak a session")
	assert.Equal(t, 2, proc.startCount(), "ffmpeg must not even start for the rejected spawn")
}

func TestOrchestrator_MaxActiveSessions_SlotFreesWhenSessionExits(t *testing.T) {
	// Cap = 1: once the single slot is held, a second media is refused; after the
	// first session is removed (and its reap goroutine frees the slot) the second
	// media spawns. Proves the slot accounting is released, not leaked.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.MaxActiveSessions = 1
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	first := minimalFingerprint(t)
	first.MediaID = "media-1"
	second := minimalFingerprint(t)
	second.MediaID = "media-2"

	require.NoError(t, o.EnsureSegmentRange(context.Background(), first, 0, 0))
	require.ErrorIs(t, o.EnsureSegmentRange(context.Background(), second, 0, 0), ErrCapacityExceeded)

	// Free the slot: removing media-1 kills its rafale; the reap goroutine then
	// releases the semaphore slot asynchronously.
	o.RemoveSession("media-1")

	require.Eventually(t, func() bool {
		return o.EnsureSegmentRange(context.Background(), second, 0, 0) == nil
	}, 2*time.Second, 10*time.Millisecond, "second media must spawn once the freed slot is released")
}

func TestOrchestrator_MaxActiveSessions_ZeroIsUnlimited(t *testing.T) {
	// MaxActiveSessions unset (0) preserves legacy behaviour: no global cap, every
	// distinct-media spawn succeeds.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	for _, id := range []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8"} {
		f := minimalFingerprint(t)
		f.MediaID = id
		require.NoError(t, o.EnsureSegmentRange(context.Background(), f, 0, 0))
	}
	assert.Equal(t, 8, o.ActiveCount(), "no cap -> all 8 distinct rafales live")
}

// ============================================================================
// Shared video-session capacity limiter (VideoSessionLimiter)
// ============================================================================

func TestOrchestrator_VideoLimiter_GatedAtCapacity(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	limiter := NewVideoSessionLimiter(1)
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	o.WithVideoSessionLimiter(limiter)
	defer o.Close()

	fpFor := func(id string) DecisionFingerprint {
		f := minimalFingerprint(t)
		f.MediaID = id
		return f
	}

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fpFor("media-1"), 0, 0))
	assert.Equal(t, 1, limiter.Active())

	err := o.EnsureSegmentRange(context.Background(), fpFor("media-2"), 0, 0)
	require.ErrorIs(t, err, ErrVideoCapacityExhausted)
	assert.Equal(t, 1, o.ActiveCount(), "the refused spawn must not leak a session")
	assert.Equal(t, 1, limiter.Active())
}

func TestOrchestrator_VideoLimiter_ReleasedOnNaturalExit(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	limiter := NewVideoSessionLimiter(1)
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	o.WithVideoSessionLimiter(limiter)
	defer o.Close()

	first := minimalFingerprint(t)
	first.MediaID = "media-1"
	hash := first.Hash()
	require.NoError(t, o.EnsureSegmentRange(context.Background(), first, 0, 0))
	require.Equal(t, 1, limiter.Active())

	o.mu.Lock()
	handle := o.sessions[hash][0].handle.(*fakeHandle)
	o.mu.Unlock()
	require.NoError(t, handle.Stop()) // simulate the ffmpeg process exiting on its own

	require.Eventually(t, func() bool {
		return limiter.Active() == 0
	}, time.Second, 10*time.Millisecond, "the reap goroutine must free the video slot on natural exit")

	second := minimalFingerprint(t)
	second.MediaID = "media-2"
	require.NoError(t, o.EnsureSegmentRange(context.Background(), second, 0, 0),
		"the freed slot must admit a fresh rafale")
}

func TestOrchestrator_VideoLimiter_ReleasedOnKill(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	limiter := NewVideoSessionLimiter(1)
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	o.WithVideoSessionLimiter(limiter)
	defer o.Close()

	first := minimalFingerprint(t)
	first.MediaID = "media-1"
	require.NoError(t, o.EnsureSegmentRange(context.Background(), first, 0, 0))
	require.Equal(t, 1, limiter.Active())

	o.RemoveSession("media-1")
	assert.Equal(t, 0, limiter.Active(), "an explicit kill must free the video slot")
	assert.Equal(t, 0, o.ActiveCount())
}

func TestOrchestrator_VideoLimiter_Unwired_Unlimited(t *testing.T) {
	// No WithVideoSessionLimiter call at all - byte-identical to pre-feature
	// behaviour: never ErrVideoCapacityExhausted.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	for _, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		f := minimalFingerprint(t)
		f.MediaID = id
		require.NoError(t, o.EnsureSegmentRange(context.Background(), f, 0, 0))
	}
	assert.Equal(t, 5, o.ActiveCount())
}

func TestOrchestrator_VideoLimiter_SpawnRaceLoserReleasesExactlyOnce(t *testing.T) {
	// -race: many goroutines racing to spawn a rafale for the exact same
	// (hash, variant, segment). Exactly one wins; every loser must release its
	// acquired slot via the "spawn race lost" early-return path, never via the
	// reap goroutine (it never gets that far), and never double-release.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	limiter := NewVideoSessionLimiter(0) // unlimited - isolates release bookkeeping from acquire refusals
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	o.WithVideoSessionLimiter(limiter)
	defer o.Close()

	fp := minimalFingerprint(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_ = o.EnsureSegmentRange(context.Background(), fp, 0, 0)
		})
	}
	wg.Wait()

	assert.Equal(t, 1, o.ActiveCount(), "exactly one rafale wins the spawn race")
	assert.Equal(t, 1, limiter.Active(), "every loser released its slot; only the winner still holds one")
}

// ============================================================================
// EnsureSegmentRange: cancel-on-seek
// ============================================================================

func TestOrchestrator_EnsureSegmentRange_SeekSpawnsAdditionalRafale(t *testing.T) {
	// Multi-rafale per hash: a seek outside the active window spawns a
	// new ffmpeg ALONGSIDE the existing one instead of cancelling it.
	// Lets two clients on the same media at different positions
	// coexist (two browser tabs, PC + mobile by the same user). The
	// older rafale only retires when the LRU cap is reached.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())

	// Seek to segment 500 → far outside window [0, 29] of rafale 1.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 500))

	// Both rafales should now coexist (MaxRafalesPerHash default = 3).
	require.Eventually(t, func() bool {
		return o.ActiveCount() == 2 && proc.startCount() == 2
	}, time.Second, 10*time.Millisecond)

	// The 2nd spawn's StartSec reflects the seek target, and StartNumber
	// names the segment it starts at: that is what routes its 0-based fMP4
	// output through the rebase pump. A rafale with StartSec alone published
	// fragments stamped from 0 under absolute segment names, poisoning the
	// cache for every later viewer of the title (prod, 2026-09-02/03).
	assert.InDelta(t, 500.0*3.0, proc.lastOpts().StartSec, 0.001)
	assert.Equal(t, 500, proc.lastOpts().StartNumber)
}

func TestOrchestrator_EnsureSegmentRange_LRUEvictsOldestBeyondCap(t *testing.T) {
	// MaxRafalesPerHash=2: spawn 3 distant rafales, the 1st (oldest)
	// must be LRU-evicted so we stay at the cap. Validates the
	// multi-rafale ceiling - a runaway client (10 seeks/min) can't
	// pile up ffmpeg processes indefinitely.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.MaxRafalesPerHash = 2
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	// Tiny pause between spawns so the LRU timestamps order
	// predictably (clock-based ordering, not insert order).
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 500))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 1000))

	require.Eventually(t, func() bool {
		return o.ActiveCount() == 2 && proc.startCount() == 3
	}, time.Second, 10*time.Millisecond, "third rafale should evict the oldest, capping at 2")
}

// ============================================================================
// Inactivity GC
// ============================================================================

func TestOrchestrator_InactivityGC_ReapsIdleSessions(t *testing.T) {
	// With a tiny InactivityTimeout, a spawned session is reaped soon after if
	// nothing touches it. Validates the GC sweeper runs and acts.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.InactivityTimeout = 50 * time.Millisecond
	opts.GCInterval = 20 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, o.ActiveCount())

	require.Eventually(t, func() bool {
		return o.ActiveCount() == 0
	}, time.Second, 10*time.Millisecond, "GC must reap the idle session")
}

func TestOrchestrator_InactivityGC_KeepsLiveSessions(t *testing.T) {
	// A session whose lastAccess is bumped within the inactivity window MUST
	// survive the GC sweep. Otherwise actively-streamed sessions would die.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.InactivityTimeout = 150 * time.Millisecond
	opts.GCInterval = 30 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	// Touch it repeatedly under the timeout - must stay alive.
	stop := time.After(400 * time.Millisecond)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			assert.Equal(t, 1, o.ActiveCount())
			return
		case <-tick.C:
			require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
		}
	}
}

// ============================================================================
// Close
// ============================================================================

// ============================================================================
// Throttle: pause-when-ahead policy
// ============================================================================

func TestOrchestrator_Throttle_PausesWhenClientFarBehind(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 6
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 50 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, o.ActiveCount())

	// Simulate ffmpeg producing 10 segments while the client is still at 0.
	// useFMP4=true (matches minimalFingerprint) → .m4s. These must be real
	// segments: the throttle now measures the buffer with IsReady, so a
	// name-only stub counts as not-yet-produced (which is the point - a
	// truncated or styp-only file gives the player nothing to decode).
	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := range 10 {
		name := filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s"))
		require.NoError(t, os.WriteFile(name, fmp4Seg(), 0o644))
	}

	// Reach into the session to retrieve the handle for assertion.
	o.mu.Lock()
	sessList := o.sessions[fp.Hash()]
	var sess *orchestratorSession
	if len(sessList) > 0 {
		sess = sessList[0]
	}
	o.mu.Unlock()
	require.NotNil(t, sess)
	handle := sess.handle.(*fakeHandle)

	// Wait up to 1s for the throttle loop to detect the buffer and pause.
	require.Eventually(t, func() bool {
		p, _, paused := handle.counts()
		return p >= 1 && paused
	}, time.Second, 20*time.Millisecond, "throttle should pause ffmpeg when produced - client >= PauseAhead")

	// Client catches up to within ResumeBelow → throttle must resume.
	o.touchSession(fp.Hash(), 0, 9)

	require.Eventually(t, func() bool {
		_, r, paused := handle.counts()
		return r >= 1 && !paused
	}, time.Second, 20*time.Millisecond, "throttle should resume ffmpeg when client catches up")
}

// A reader served from disk past the rafale's window must slide that window:
// the throttle only probes production up to endSeg, so a frozen window hid the
// running ffmpeg from it and the next miss respawned a duplicate encoder.
func TestOrchestrator_CacheHitPastWindow_SlidesWindowAndThrottleSeesProduction(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.LookAheadSegments = 10
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 6
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 20 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := range 26 {
		require.NoError(t, os.WriteFile(filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s")), fmp4Seg(), 0o644))
	}
	for i := 0; i <= 15; i++ {
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, i))
	}
	require.Equal(t, int32(1), proc.count.Load(), "every read was a cache hit")

	o.mu.Lock()
	sess := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	sess.mu.Lock()
	endSeg := sess.endSeg
	sess.mu.Unlock()
	assert.GreaterOrEqual(t, endSeg, 15, "the window must follow the reader")
	require.Eventually(t, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.producedHighWater == endSeg
	}, time.Second, 10*time.Millisecond, "the throttle must see production past the original window")
}

func TestOrchestrator_NotifyClientHint_ResumesPausedOnStarvation(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 4
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 50 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := range 8 {
		require.NoError(t, os.WriteFile(filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s")), fmp4Seg(), 0o644))
	}
	o.mu.Lock()
	sessList := o.sessions[fp.Hash()]
	var sess *orchestratorSession
	if len(sessList) > 0 {
		sess = sessList[0]
	}
	o.mu.Unlock()
	handle := sess.handle.(*fakeHandle)

	require.Eventually(t, func() bool { _, _, paused := handle.counts(); return paused }, time.Second, 10*time.Millisecond)

	// CMCD bs=1 arrives - we must SIGCONT immediately, not wait the tick.
	o.NotifyClientHint(fp.MediaID, 0, 0, true, 1)

	_, resumes, paused := handle.counts()
	assert.GreaterOrEqual(t, resumes, 1, "starvation hint must resume")
	assert.False(t, paused, "starvation hint must clear paused state")
}

func TestOrchestrator_NotifyClientHint_NoOpOnUnknownMedia(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	o.NotifyClientHint("nobody-here", 0, 12000, true, 5)
	assert.Equal(t, 0, o.ActiveCount())
}

func TestOrchestrator_NotifyClientHint_ExtendsEndSegOnFatBuffer(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.LookAheadSegments = 10
	opts.SegmentSeconds = 3.0
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	o.mu.Lock()
	sessList := o.sessions[fp.Hash()]
	var sess *orchestratorSession
	if len(sessList) > 0 {
		sess = sessList[0]
	}
	o.mu.Unlock()
	sess.mu.Lock()
	endBefore := sess.endSeg
	sess.mu.Unlock()

	// 45s buffer ≥ LookAhead × segmentSeconds (10×3=30s) → stretch endSeg.
	o.NotifyClientHint(fp.MediaID, 0, 45000, false, 0)

	sess.mu.Lock()
	endAfter := sess.endSeg
	sess.mu.Unlock()
	assert.Greater(t, endAfter, endBefore, "fat buffer hint should stretch endSeg")
}

func TestOrchestrator_NotifyClientHint_ForgedBufferClamped(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.LookAheadSegments = 10
	opts.SegmentSeconds = 3.0
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	// A forged CMCD bl of ~277 hours must NOT stretch endSeg to encode the whole
	// film: bufferAheadSegs is capped at 2×LookAhead, so endSeg stays within
	// consumableIdx + 3×LookAhead (0 + 30 here), not ~333k.
	o.NotifyClientHint(fp.MediaID, 0, 999_999_999, false, 0)

	o.mu.Lock()
	sess := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	sess.mu.Lock()
	endAfter := sess.endSeg
	sess.mu.Unlock()
	assert.LessOrEqual(t, endAfter, 3*opts.LookAheadSegments,
		"forged bl must not stretch endSeg past the clamped window")
}

func TestOrchestrator_Throttle_NoFlapWithHysteresis(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 4
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 30 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := range 5 {
		require.NoError(t, os.WriteFile(filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s")), fmp4Seg(), 0o644))
	}

	o.mu.Lock()
	sessList := o.sessions[fp.Hash()]
	var sess *orchestratorSession
	if len(sessList) > 0 {
		sess = sessList[0]
	}
	o.mu.Unlock()
	handle := sess.handle.(*fakeHandle)

	// Hold steady at buffer=5-0=5: ≥ PauseAhead(4), so we pause exactly once.
	// Many ticks must NOT produce many pauses (hysteresis: stay paused).
	require.Eventually(t, func() bool { p, _, _ := handle.counts(); return p >= 1 }, time.Second, 10*time.Millisecond)
	// Hysteresis: once paused, no further pause and no resume may fire while the
	// buffer stays above ResumeBelow. assert.Never fails fast on a stray
	// pause/resume instead of a blind sleep-then-check.
	assert.Never(t, func() bool { p, r, _ := handle.counts(); return p > 1 || r > 0 }, 200*time.Millisecond, 20*time.Millisecond,
		"no double-pause and no resume while buffer stays above ResumeBelow")
}

func formatVariantSegment(variantIdx, segIdx int, ext string) string {
	return "stream_" + itoa(variantIdx) + "_" + zeroPad3(segIdx) + ext
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [10]byte
	n := 0
	for i > 0 {
		b[n] = byte('0' + i%10)
		i /= 10
		n++
	}
	out := make([]byte, n)
	for j := range n {
		out[j] = b[n-1-j]
	}
	return string(out)
}

func zeroPad3(i int) string {
	if i < 10 {
		return "00" + itoa(i)
	}
	if i < 100 {
		return "0" + itoa(i)
	}
	return itoa(i)
}

func TestOrchestrator_Close_KillsAllSessions(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, o.ActiveCount())

	o.Close()
	assert.Equal(t, 0, o.ActiveCount())
}

func TestOrchestrator_Close_CleansDiskSegments(t *testing.T) {
	// The auditor's concern: a graceful shutdown must purge the segments of
	// every live session - cancel-on-seek keeps them warm because rewind
	// might hit them, but Close has no "future" to keep them around for.
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hashDir, "stream_0_000.ts"), []byte("xx"), 0o644))

	o.Close()

	_, err := os.Stat(hashDir)
	assert.True(t, os.IsNotExist(err), "Close must remove the hash directory")
}

// ============================================================================
// SingleVariant flag: procOpts invariants
// ============================================================================

// TestOrchestrator_SingleVariant_FlagOff_MultiVariantProfiles asserts the hard
// FLAG-OFF invariant: with SingleVariant=false the spawned session's procOpts
// must carry ALL profiles (multi-variant, byte-identical to today's behaviour).
func TestOrchestrator_SingleVariant_FlagOff_MultiVariantProfiles(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}

	opts := testOpts()
	opts.SingleVariant = false // explicitly exercise the multi-variant ladder path
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := multiProfileFingerprint(t) // 3-rung ladder
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 1, 0))

	require.Equal(t, 1, proc.startCount(), "one ffmpeg spawned for all rungs")
	got := proc.lastOpts()
	assert.Len(t, got.Profiles, len(fp.Profiles),
		"SingleVariant=false: all profiles must reach the ffmpeg adapter")
	// VariantIndex is irrelevant to the multi-variant builder (it names rungs
	// itself via -var_stream_map), but we confirm it stays at 0.
	assert.Equal(t, 0, got.VariantIndex,
		"SingleVariant=false: VariantIndex must be 0 (multi-variant builder convention)")
}

// TestOrchestrator_SingleVariant_FlagOn_SingleProfileAndVariantIndex asserts
// the FLAG-ON invariant: with SingleVariant=true, procOpts carries exactly ONE
// profile (the requested rung) and VariantIndex equals the requested variantIdx.
func TestOrchestrator_SingleVariant_FlagOn_SingleProfileAndVariantIndex(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}

	opts := testOpts()
	opts.SingleVariant = true
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := multiProfileFingerprint(t) // 3-rung ladder: indices 0, 1, 2
	const wantVariant = 2
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, wantVariant, 0))

	require.Equal(t, 1, proc.startCount(), "one ffmpeg spawned for the requested rung")
	got := proc.lastOpts()
	require.Len(t, got.Profiles, 1,
		"SingleVariant=true: exactly one profile must reach the ffmpeg adapter")
	assert.Equal(t, fp.Profiles[wantVariant].Name, got.Profiles[0].Name,
		"SingleVariant=true: the one profile must be the requested rung's profile")
	assert.Equal(t, wantVariant, got.VariantIndex,
		"SingleVariant=true: VariantIndex must equal the requested variantIdx so the builder names output stream_<variantIdx>_*")
}

// TestOrchestrator_SingleVariant_FlagOn_PerRungIsolation asserts that two
// requests for different rungs under SingleVariant=true each spawn their own
// ffmpeg session (they don't share a session, since that would encode the wrong
// profile). In multi-variant mode a single session serves all rungs.
func TestOrchestrator_SingleVariant_FlagOn_PerRungIsolation(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}

	opts := testOpts()
	opts.SingleVariant = true
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := multiProfileFingerprint(t) // 3-rung ladder
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 1, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 2, 0))

	assert.Equal(t, 3, proc.startCount(),
		"SingleVariant=true: each distinct rung spawns its own ffmpeg")
	assert.Equal(t, 3, o.ActiveCount(),
		"SingleVariant=true: three sessions live (one per rung)")
}

// TestOrchestrator_PreWarm_SingleVariant_TargetsRequestedVariant asserts that
// PreWarm forwards the requested variantIdx instead of hard-coding rung 0.
// Under SingleVariant=true each rung gets its own ffmpeg, so pre-warming the
// variant the player actually asked for (not rung 0, which nobody will ever
// watch) is the whole point of the fix: pre-warming the wrong rung wastes a
// full ffmpeg process (~720MB RSS + a core, measured) that is never consumed.
func TestOrchestrator_PreWarm_SingleVariant_TargetsRequestedVariant(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}

	opts := testOpts()
	opts.SingleVariant = true
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())

	fp := multiProfileFingerprint(t) // 3-rung ladder: indices 0, 1, 2
	const wantVariant = 2
	o.PreWarm(fp, wantVariant, 0)
	// Wait for the spawn BEFORE Close: the prewarm goroutine legitimately
	// bails when it observes shutdownCtx already canceled, so closing right
	// away races the scheduler and intermittently sees zero spawns.
	require.Eventually(t, func() bool { return proc.startCount() == 1 },
		2*time.Second, 5*time.Millisecond, "PreWarm must spawn one ffmpeg")
	o.Close() // drains prewarmWG before returning

	require.Equal(t, 1, proc.startCount(), "PreWarm must spawn exactly one ffmpeg, and Close must not spawn more")
	got := proc.lastOpts()
	assert.Equal(t, wantVariant, got.VariantIndex,
		"PreWarm must target the requested variant, not hard-coded rung 0")
	require.Len(t, got.Profiles, 1,
		"SingleVariant=true: PreWarm must warm only the requested rung's profile")
	assert.Equal(t, fp.Profiles[wantVariant].Name, got.Profiles[0].Name,
		"PreWarm must warm the requested rung's profile, not rung 0's")
}

// multiProfileFingerprint returns a fingerprint with a 3-rung ABR ladder for
// tests that need to exercise multi-profile behaviour.
func multiProfileFingerprint(t *testing.T) DecisionFingerprint {
	t.Helper()
	fp := minimalFingerprint(t)
	fp.Profiles = []ProfileFingerprint{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3000, AudioBitrate: 128},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 6000, AudioBitrate: 192},
	}
	return fp
}

// ============================================================================
// fakes
// ============================================================================

func testOpts() OrchestratorOptions {
	return OrchestratorOptions{
		LookAheadSegments: 30,
		InactivityTimeout: 1 * time.Minute,
		GCInterval:        100 * time.Millisecond,
		SegmentSeconds:    3.0,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeProcess implements transcoding.HLSProcess for tests. Records every Start
// call. The handle it returns blocks on Stop, mimicking a real ffmpeg that
// runs forever until killed.
type fakeProcess struct {
	mu      sync.Mutex
	started []transcoding.HLSProcessOpts
	count   atomic.Int32
	// failOnExit, when set, makes every spawned handle report this failure
	// class and exit immediately (Done closed) - drives the level-2 safety net.
	failOnExit transcoding.FailureClass
	// startErr, when set, makes Start itself fail (records the attempt, returns
	// the error, no handle) - used by the whole-file retry/invariant tests.
	startErr error
}

func newFakeProcess() *fakeProcess { return &fakeProcess{} }

func (p *fakeProcess) Start(_ context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	p.mu.Lock()
	p.started = append(p.started, opts)
	fail := p.failOnExit
	serr := p.startErr
	p.mu.Unlock()
	p.count.Add(1)
	if serr != nil {
		return nil, serr
	}
	h := &fakeHandle{done: make(chan struct{}), failure: fail}
	if fail != "" {
		// Simulate an immediate process exit with this failure. The handle is
		// already stopped: a Stop() racing the reaper (Close after churn) must
		// not close done twice.
		h.stopped = true
		close(h.done)
	}
	return h, nil
}

func (p *fakeProcess) startCount() int { return int(p.count.Load()) }

func (p *fakeProcess) lastOpts() transcoding.HLSProcessOpts {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.started) == 0 {
		return transcoding.HLSProcessOpts{}
	}
	return p.started[len(p.started)-1]
}

type fakeHandle struct {
	mu      sync.Mutex
	stopped bool
	pauseN  int
	resumeN int
	paused  bool
	done    chan struct{}
	failure transcoding.FailureClass
}

func (h *fakeHandle) Failure() transcoding.FailureClass { return h.failure }

func (h *fakeHandle) WaitReady(_ context.Context) error { return nil }
func (h *fakeHandle) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.stopped = true
		close(h.done)
	}
	return nil
}
func (h *fakeHandle) Done() <-chan struct{} { return h.done }
func (h *fakeHandle) Err() error            { return nil }
func (h *fakeHandle) Pause() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pauseN++
	h.paused = true
	return nil
}
func (h *fakeHandle) Resume() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resumeN++
	h.paused = false
	return nil
}
func (h *fakeHandle) counts() (pauseN, resumeN int, paused bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pauseN, h.resumeN, h.paused
}
func (h *fakeHandle) isStopped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
}

type stubSource struct {
	path    string
	streams []SourceStream
	err     error
}

func (s stubSource) ResolveSource(_ context.Context, _ DecisionFingerprint) (string, []SourceStream, error) {
	return s.path, s.streams, s.err
}

// killSession nulls cancelCtx first and only splices the session out of
// o.sessions after SIGTERM + process wait, so for as long as ffmpeg takes to
// die a corpse sits in the list looking like a valid cover. Matching one hands
// the caller a producer that will never write another segment - and the same
// check runs inside spawnSession, where a corpse match discards the live
// replacement ("spawn race lost") and leaves nobody producing at all.
func TestOrchestrator_DyingSessionIsNotAValidCover(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())

	// Reproduce the window inside killSession: claimed, not yet delisted.
	o.mu.Lock()
	dying := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	dying.mu.Lock()
	dying.cancelCtx = nil
	dying.mu.Unlock()

	// Segment 5 sits inside the corpse's [0, 29] window.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 5))
	assert.Equal(t, 2, proc.startCount(),
		"a session already claimed by killSession must not be reused - the segment would never be produced")
}
