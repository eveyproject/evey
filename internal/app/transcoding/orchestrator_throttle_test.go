package transcoding

import (
	"context"
	"testing"

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
