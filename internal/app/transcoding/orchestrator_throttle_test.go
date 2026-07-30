package transcoding

import (
	"context"
	"testing"

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

		o.NotifyClientHint(fp.MediaID, 89_000, false, 0) // 29 segments
		if got := endSegOf(t, o, fp); got != before {
			t.Errorf("endSeg %d -> %d: a buffer below one lookahead must not stretch the window", before, got)
		}
	})

	t.Run("exactly one lookahead stretches", func(t *testing.T) {
		o, fp := hintedOrchestrator(t)
		before := endSegOf(t, o, fp)

		o.NotifyClientHint(fp.MediaID, 90_000, false, 0) // 30 segments
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

	o.NotifyClientHint(fp.MediaID, 9_000, false, 12)
	require.Equal(t, 12, clientHighWaterOf(t, o, fp))

	o.NotifyClientHint(fp.MediaID, 9_000, false, 4) // stale / reordered
	o.NotifyClientHint(fp.MediaID, 9_000, false, 12)
	// A negative index is nonsense on the wire and must clamp to zero, never
	// reach the comparison as a rewind.
	o.NotifyClientHint(fp.MediaID, 9_000, false, -5)

	if got := clientHighWaterOf(t, o, fp); got != 12 {
		t.Errorf("clientHighWater = %d, want 12 - a later hint must never rewind it", got)
	}
}

// A zero or absent bl means "no information", not "zero buffer": it must leave
// the window untouched rather than compute a stretch from nothing.
func TestNotifyClientHint_AbsentBufferLengthIsIgnored(t *testing.T) {
	o, fp := hintedOrchestrator(t)
	before := endSegOf(t, o, fp)

	o.NotifyClientHint(fp.MediaID, 0, false, 0)
	o.NotifyClientHint(fp.MediaID, -1, false, 0)

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
func TestEnsureSegmentRange_CoverageWindowEdges(t *testing.T) {
	opts := testOpts() // LookAheadSegments 30
	look := opts.LookAheadSegments

	newOrch := func(t *testing.T) (*SegmentOrchestrator, *fakeProcess, DecisionFingerprint, int) {
		t.Helper()
		cache, _ := newTestCache(t)
		proc := newFakeProcess()
		o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
		t.Cleanup(o.Close)

		fp := minimalFingerprint(t)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
		require.Equal(t, 1, proc.startCount(), "premise: exactly one encoder is running")
		return o, proc, fp, endSegOf(t, o, fp)
	}

	t.Run("the last segment inside the window reuses", func(t *testing.T) {
		o, proc, fp, end := newOrch(t)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, end))
		assert.Equal(t, 1, proc.startCount(), "a segment AT endSeg is covered - no second encoder")
	})

	t.Run("the far edge of the near-ahead band reuses and extends", func(t *testing.T) {
		o, proc, fp, end := newOrch(t)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, end+look))
		assert.Equal(t, 1, proc.startCount(),
			"a request exactly one lookahead past endSeg is still reachable - the rafale extends instead of respawning")
		assert.Equal(t, end+look+look, endSegOf(t, o, fp),
			"the window must be pushed to the request plus a fresh lookahead")
	})

	t.Run("one segment past the band spawns a fresh encoder", func(t *testing.T) {
		o, proc, fp, end := newOrch(t)
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
