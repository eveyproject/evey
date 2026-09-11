package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// endSegOf reads the single live session's encode window. Panics rather than
// returns an error: every caller here has just created exactly one session, so
// a miss means the test's premise is broken, not the code under test.
func endSegOf(t *testing.T, o *SegmentOrchestrator, fp DecisionFingerprint) int {
	t.Helper()
	o.mu.Lock()
	sess := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.endSeg
}

func clientHighWaterOf(t *testing.T, o *SegmentOrchestrator, fp DecisionFingerprint) int {
	t.Helper()
	o.mu.Lock()
	sess := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.clientHighWater
}

func hintedOrchestrator(t *testing.T) (*SegmentOrchestrator, DecisionFingerprint) {
	t.Helper()
	cache, _ := newTestCache(t)
	o := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	t.Cleanup(o.Close)

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	return o, fp
}

// The prefetch stretch is gated on the client being at least a full lookahead
// ahead. That threshold is exact: one segment short must change nothing, and
// landing exactly on it must extend. Without the second case the guard could be
// off by one and no test would notice.
//
// testOpts: LookAheadSegments 30, SegmentSeconds 3 -> 30 segments = 90000 ms.
func TestNotifyClientHint_PrefetchStretchStartsExactlyAtOneLookahead(t *testing.T) {
	t.Run("one segment short leaves the window alone", func(t *testing.T) {
		o, fp := hintedOrchestrator(t)
		before := endSegOf(t, o, fp)

		o.NotifyClientHint(fp.MediaID, 0, 89_000, false, 0) // 29 segments
		if got := endSegOf(t, o, fp); got != before {
			t.Errorf("endSeg %d -> %d: a buffer below one lookahead must not stretch the window", before, got)
		}
	})

	t.Run("exactly one lookahead stretches", func(t *testing.T) {
		o, fp := hintedOrchestrator(t)
		before := endSegOf(t, o, fp)

		o.NotifyClientHint(fp.MediaID, 0, 90_000, false, 0) // 30 segments
		got := endSegOf(t, o, fp)
		if got != 2*testOpts().LookAheadSegments {
			t.Errorf("endSeg = %d (was %d), want %d: hint index + buffer + lookahead",
				got, before, 2*testOpts().LookAheadSegments)
		}
	})
}

// clientHighWater only ever moves forward. A stale or reordered hint naming an
// earlier segment must not rewind it - the mark is what tells the throttle how
// far the viewer has really consumed, and rewinding it would make a comfortable
// client look starved.
func TestNotifyClientHint_HighWaterOnlyMovesForward(t *testing.T) {
	o, fp := hintedOrchestrator(t)

	o.NotifyClientHint(fp.MediaID, 0, 9_000, false, 12)
	require.Equal(t, 12, clientHighWaterOf(t, o, fp))

	o.NotifyClientHint(fp.MediaID, 0, 9_000, false, 4) // stale / reordered
	o.NotifyClientHint(fp.MediaID, 0, 9_000, false, 12)
	// A negative index is nonsense on the wire and must clamp to zero, never
	// reach the comparison as a rewind.
	o.NotifyClientHint(fp.MediaID, 0, 9_000, false, -5)

	if got := clientHighWaterOf(t, o, fp); got != 12 {
		t.Errorf("clientHighWater = %d, want 12 - a later hint must never rewind it", got)
	}
}

// A zero or absent bl means "no information", not "zero buffer": it must leave
// the window untouched rather than compute a stretch from nothing.
func TestNotifyClientHint_AbsentBufferLengthIsIgnored(t *testing.T) {
	o, fp := hintedOrchestrator(t)
	before := endSegOf(t, o, fp)

	o.NotifyClientHint(fp.MediaID, 0, 0, false, 0)
	o.NotifyClientHint(fp.MediaID, 0, -1, false, 0)

	if got := endSegOf(t, o, fp); got != before {
		t.Errorf("endSeg %d -> %d: a missing bl must not move the window", before, got)
	}
}

// ============================================================================
// Session coverage window: reuse or spawn
// ============================================================================

// A live rafale covers [startSeg, endSeg], plus a LookAhead-wide "near ahead"
// band it will extend into. Where that band ends decides whether a request
// reuses the running encoder or spawns a second one - on a box that fits one
// heavy transcode, a spurious spawn is the difference between playback and two
// encoders splitting the CPU. Both edges are exact, so both are asserted.
// Reuse also needs the request within SeekReachWindowSegs of the production
// point, so each case first drains the cache up to the point it needs.
func TestEnsureSegmentRange_CoverageWindowEdges(t *testing.T) {
	opts := testOpts() // LookAheadSegments 30
	look := opts.LookAheadSegments
	reach := opts.withDefaults().SeekReachWindowSegs

	newOrch := func(t *testing.T) (*SegmentOrchestrator, *SegmentCache, *fakeProcess, DecisionFingerprint, int) {
		t.Helper()
		cache, _ := newTestCache(t)
		proc := newFakeProcess()
		o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
		t.Cleanup(o.Close)

		fp := minimalFingerprint(t)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
		require.Equal(t, 1, proc.startCount(), "premise: exactly one encoder is running")
		return o, cache, proc, fp, endSegOf(t, o, fp)
	}
	// producedThrough moves the production point to seg via a cache hit.
	producedThrough := func(t *testing.T, o *SegmentOrchestrator, cache *SegmentCache, fp DecisionFingerprint, seg int) {
		t.Helper()
		writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: seg}, fp.UseFMP4)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, seg))
	}

	t.Run("the last segment inside the window reuses", func(t *testing.T) {
		o, cache, proc, fp, end := newOrch(t)
		producedThrough(t, o, cache, fp, end-1)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, end))
		assert.Equal(t, 1, proc.startCount(), "a segment AT endSeg is covered - no second encoder")
	})

	t.Run("the far edge of the near-ahead band reuses and extends", func(t *testing.T) {
		o, cache, proc, fp, end := newOrch(t)
		producedThrough(t, o, cache, fp, end)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, end+reach))
		assert.Equal(t, 1, proc.startCount(),
			"a request past endSeg but within reach of the point is still reachable - the rafale extends instead of respawning")
		assert.Equal(t, end+reach+look, endSegOf(t, o, fp),
			"the window must be pushed to the request plus a fresh lookahead")
	})

	t.Run("one segment past the band spawns a fresh encoder", func(t *testing.T) {
		o, cache, proc, fp, end := newOrch(t)
		producedThrough(t, o, cache, fp, end)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, end+look+1))
		assert.Equal(t, 2, proc.startCount(),
			"beyond the band the paced encoder cannot reach in time - seek a fresh one rather than buffer forever")
	})

	t.Run("a backward seek below startSeg spawns alongside", func(t *testing.T) {
		cache, _ := newTestCache(t)
		proc := newFakeProcess()
		o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
		defer o.Close()

		fp := minimalFingerprint(t)
		// Start deep, then ask for something strictly before it.
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 500))
		require.Equal(t, 1, proc.startCount())

		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 499))
		assert.Equal(t, 2, proc.startCount(),
			"the live window is [startSeg, +inf): a request below startSeg is not covered, whatever the distance")
	})
}

// A hint is scoped to the rung it describes. In SingleVariant mode each rung has
// its own rafale, and pushing a rung-0 hint into the rung the player just left
// set that session's clientHighWater to the live playhead: evaluateThrottle then
// read a deeply negative buffer (produced - client) forever, so the
// pause-when-ahead branch never fired again. That branch is the orchestrator's
// ONLY brake (it has no pauseOtherVariants), so the abandoned rung kept a full
// decode running against the live one.
func TestNotifyClientHint_LeavesTheAbandonedRungAlone(t *testing.T) {
	cache, _ := newTestCache(t)
	opts := testOpts()
	opts.SingleVariant = true
	o := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	t.Cleanup(o.Close)

	fp := minimalFingerprint(t)
	fp.Profiles = append(fp.Profiles,
		ProfileFingerprint{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3000, AudioBitrate: 128})
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 1, 0))

	rungs := map[int]*orchestratorSession{}
	o.mu.Lock()
	for _, s := range o.sessions[fp.Hash()] {
		rungs[s.variantIdx] = s
	}
	o.mu.Unlock()
	require.Len(t, rungs, 2, "each rung must own its rafale in SingleVariant mode")

	// The player is consuming rung 0; rung 1 is the one it walked away from.
	o.NotifyClientHint(fp.MediaID, 0, 9_000, false, 12)

	assert.Equal(t, 12, clientHighWaterOfSession(rungs[0]), "the rung being fetched must track the playhead")
	assert.Equal(t, 0, clientHighWaterOfSession(rungs[1]),
		"a hint about another rung must not move this one's cursor - that is what disables its throttle")
}

// clientHighWater is monotonic, so ONE forged cursor disables the throttle for
// the whole life of the session. The handler's duration bound cannot be relied
// on: it skips init segments, copy rungs, and any item whose duration was never
// probed. Clamp here instead - a hint has no legitimate reason to name a
// position beyond the window the rafale is producing.
func TestNotifyClientHint_ForgedCursorIsClampedToTheWindow(t *testing.T) {
	o, fp := hintedOrchestrator(t)
	endSeg := endSegOf(t, o, fp)

	o.NotifyClientHint(fp.MediaID, 0, 9_000, false, 999_999)

	assert.Equal(t, endSeg+2*testOpts().LookAheadSegments, clientHighWaterOf(t, o, fp),
		"a stream_0_999999.m4s must not push the cursor past the rafale's own reach")
}

func clientHighWaterOfSession(s *orchestratorSession) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientHighWater
}

// A rafale nobody reads while it is ahead has lost its viewer (a seek landed on
// another rafale, or on cache hits): it must pause at IdlePauseAfter, not at
// PauseAhead. Prod 2026-09-11: the rafale a seek left behind kept a full 2160p
// decode running for a minute next to the one the viewer was waiting on.
func TestEvaluateThrottle_PausesTheRafaleItsViewerLeft(t *testing.T) {
	cache, _ := newTestCache(t)
	opts := testOpts()
	opts.IdlePauseAfter = 8 * time.Second
	o := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	t.Cleanup(o.Close)

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	for seg := range 3 {
		writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: seg}, fp.UseFMP4)
	}
	o.mu.Lock()
	sess := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	handle := sess.handle.(*fakeHandle)

	o.evaluateThrottle()
	if _, _, paused := handle.counts(); paused {
		t.Fatal("a rafale read a moment ago must keep running")
	}

	sess.mu.Lock()
	sess.lastAccess = time.Now().Add(-9 * time.Second)
	sess.mu.Unlock()
	o.evaluateThrottle()
	if _, _, paused := handle.counts(); !paused {
		t.Fatal("unread for 9 s and 2 segments ahead: the rafale must be paused")
	}

	// The viewer comes back inside the window: a cache hit touches the rafale
	// and the buffer (2 - 1) is under ResumeBelow, so the next tick resumes it.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 1))
	o.evaluateThrottle()
	if _, _, paused := handle.counts(); paused {
		t.Fatal("a rafale read again must resume")
	}
}

// Idle but not ahead: the viewer may be waiting on the very next segment, so
// the encoder must keep running.
func TestEvaluateThrottle_IdleRafaleNotAheadKeepsRunning(t *testing.T) {
	cache, _ := newTestCache(t)
	opts := testOpts()
	opts.IdlePauseAfter = 8 * time.Second
	o := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	t.Cleanup(o.Close)

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 0}, fp.UseFMP4)
	o.mu.Lock()
	sess := o.sessions[fp.Hash()][0]
	o.mu.Unlock()
	sess.mu.Lock()
	sess.lastAccess = time.Now().Add(-9 * time.Second)
	sess.mu.Unlock()

	o.evaluateThrottle()
	if _, _, paused := sess.handle.(*fakeHandle).counts(); paused {
		t.Fatal("produced == requested: nothing is ahead, the rafale must not pause")
	}
}
