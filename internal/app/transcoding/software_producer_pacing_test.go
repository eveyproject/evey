package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// swSessionsOf snapshots the producer's live sessions for a hash, sorted by
// startSeg so a test can name them positionally.
func swSessionsOf(t *testing.T, p *SoftwareProducer, hash string) []*swSession {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]*swSession(nil), p.sessions[hash]...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].startSeg < out[j-1].startSeg; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func swIsPaused(s *swSession) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// twoPausedSessions builds the shape the client-hint selection rule exists for:
// a from-0 encoder abandoned by a deep forward seek, plus the seek encoder at
// 169, BOTH suspended. Returns them ordered [start 0, start 169].
func twoPausedSessions(t *testing.T) (*SoftwareProducer, DecisionFingerprint, []*swSession) {
	t.Helper()
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	t.Cleanup(p.Close)

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 169))

	sessions := swSessionsOf(t, p, fp.Hash())
	require.Len(t, sessions, 2)
	require.Equal(t, 0, sessions[0].startSeg)
	require.Equal(t, 169, sessions[1].startSeg)

	p.pauseEach(sessions)
	require.True(t, swIsPaused(sessions[0]))
	require.True(t, swIsPaused(sessions[1]))
	return p, fp, sessions
}

// The regression this selection rule exists for: a post-seek starvation hint
// (empty buffer -> CMCD bs=1) used to resume EVERY paused sibling, and the
// abandoned from-0 copy session then raced back toward EOF REWRITING the active
// session's cache files. Only the session whose window [startSeg, +inf) actually
// serves the requested index may wake.
func TestSWNotifyClientHint_ResumesOnlyTheSessionCoveringTheRequestedSegment(t *testing.T) {
	p, fp, sessions := twoPausedSessions(t)

	// The viewer is back at the beginning: only the from-0 encoder serves it.
	p.NotifyClientHint(fp.MediaID, 0, 0, true, 0)

	assert.False(t, swIsPaused(sessions[0]), "the covering session must wake on a starvation hint")
	assert.True(t, swIsPaused(sessions[1]),
		"a session starting AFTER the requested segment does not serve it and must stay suspended - "+
			"waking it is the cache-rewrite storm")
}

// When several sessions cover the index, the CLOSEST behind wins: it is the one
// whose frontier is nearest the playhead, so it delivers first. Waking the
// from-0 encoder instead would let it re-produce hundreds of segments the seek
// encoder already owns.
func TestSWNotifyClientHint_PicksTheClosestSessionBehindThePlayhead(t *testing.T) {
	p, fp, sessions := twoPausedSessions(t)

	// Segment 200 is covered by both (0 <= 200 and 169 <= 200).
	p.NotifyClientHint(fp.MediaID, 0, 0, true, 200)

	assert.True(t, swIsPaused(sessions[0]), "the far-behind session must stay suspended when a closer one covers")
	assert.False(t, swIsPaused(sessions[1]), "the closest session behind the playhead is the one that wakes")
}

// A hint without starvation is a liveness signal, not a resume trigger: it must
// keep the sessions warm for the GC without restarting a suspended encoder.
func TestSWNotifyClientHint_WithoutStarvationKeepsWarmButResumesNothing(t *testing.T) {
	p, fp, sessions := twoPausedSessions(t)

	past := time.Now().Add(-time.Hour)
	for _, s := range sessions {
		s.mu.Lock()
		s.lastAccess = past
		s.mu.Unlock()
	}

	p.NotifyClientHint(fp.MediaID, 0, 90_000, false, 0)

	for i, s := range sessions {
		assert.True(t, swIsPaused(s), "session %d must stay suspended: nothing reported starvation", i)
		s.mu.Lock()
		bumped := s.lastAccess.After(past)
		s.mu.Unlock()
		assert.True(t, bumped, "session %d must have its lastAccess bumped - every match is kept warm", i)
	}
}

// A hint naming a media nobody is producing must be a silent no-op. The id
// comes straight off the wire, so an unknown one is expected traffic, not an
// error path.
func TestSWNotifyClientHint_UnknownMediaTouchesNothing(t *testing.T) {
	p, _, sessions := twoPausedSessions(t)

	require.NotPanics(t, func() {
		p.NotifyClientHint("media-nobody-is-serving", 0, 0, true, 0)
	})
	for i, s := range sessions {
		assert.True(t, swIsPaused(s), "session %d woke on a hint for another media", i)
	}
}

// Nothing covers a segment BELOW every session's start, so no encoder may wake:
// resuming the wrong one would produce forward from a position the viewer has
// already left behind.
func TestSWNotifyClientHint_NoCoveringSessionResumesNobody(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 169))
	sessions := swSessionsOf(t, p, fp.Hash())
	// Drop the from-0 encoder so the only survivor starts at 169.
	p.killSession(sessions[0], "test: leave only the seek encoder")
	survivors := swSessionsOf(t, p, fp.Hash())
	require.Len(t, survivors, 1)
	p.pauseEach(survivors)

	// The viewer is at segment 10, ahead of nothing: 169 > 10, so it is not covered.
	p.NotifyClientHint(fp.MediaID, 0, 0, true, 10)

	assert.True(t, swIsPaused(survivors[0]),
		"a session starting past the requested segment must not be woken to serve it")
}

// Same corpse window as the orchestrator's: killSession claims the session
// (cancelCtx nil) long before removeFromMap drops it, and a claimed encoder can
// no longer produce anything. Reusing one leaves the caller waiting out its full
// budget on a segment nobody will write.
func TestSWDyingSessionIsNotAValidCover(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	t.Cleanup(p.Close)

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())

	sessions := swSessionsOf(t, p, fp.Hash())
	require.Len(t, sessions, 1)
	sessions[0].mu.Lock()
	sessions[0].cancelCtx = nil
	sessions[0].mu.Unlock()

	// Segment 4 is well inside the corpse's seek-reach window.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 4))
	assert.Equal(t, 2, proc.startCount(),
		"an encoder already claimed by killSession must not be reused as the covering session")
}

// A hint carries the rung it was fetched for, and the selection must honour it.
// Right after a level switch both rungs share the mediaID AND startSeg, so a
// mediaID-only match fell back to slice order and could SIGCONT the rung the
// player just left - resurrecting the heavy decode pauseOtherVariants had
// suspended, which is the whole point of that pause on a weak box.
func TestSWNotifyClientHint_ResumesOnlyTheHintedRung(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	t.Cleanup(p.Close)

	fp := minimalFingerprint(t)
	fp.Profiles = append(fp.Profiles,
		ProfileFingerprint{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3000, AudioBitrate: 128})
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0))

	rungs := map[int]*swSession{}
	for _, s := range swSessionsOf(t, p, fp.Hash()) {
		rungs[s.variantIdx] = s
	}
	require.Len(t, rungs, 2)
	p.pauseEach([]*swSession{rungs[0], rungs[1]})

	// The player switched UP: it is now fetching rung 1 at the same position.
	p.NotifyClientHint(fp.MediaID, 1, 0, true, 0)

	assert.False(t, swIsPaused(rungs[1]), "the hinted rung must wake")
	assert.True(t, swIsPaused(rungs[0]),
		"the abandoned rung must stay suspended - same media, same startSeg, but not the rung being fetched")
}

// idleSweepProducer returns a producer with one live session, its handle, and a
// 1-minute idle threshold - the shape sweepPause decides on.
func idleSweepProducer(t *testing.T) (*SoftwareProducer, *swSession, *fakeHandle) {
	t.Helper()
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	t.Cleanup(p.Close)

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	sessions := swSessionsOf(t, p, fp.Hash())
	require.Len(t, sessions, 1)

	s := sessions[0]
	s.mu.Lock()
	h, ok := s.handle.(*fakeHandle)
	s.mu.Unlock()
	require.True(t, ok, "the fake process must hand out a fake handle")
	return p, s, h
}

// sweepPause suspends on idleness alone, so the threshold is the only thing
// standing between a viewer's short pause and a SIGSTOPped encoder.
//
// The exact boundary is deliberately NOT asserted: cutoff is taken from
// time.Now() inside the sweep, strictly after the test stamps lastAccess, so a
// session stamped at exactly the threshold always reads as microseconds older.
// Pinning that would need an injectable clock, which this producer does not
// have. The two cases below are the ones an implementation change can actually
// get wrong.
func TestSWSweepPause_SuspendsOnlyPastTheIdleThreshold(t *testing.T) {
	t.Run("a recently pulled session stays running", func(t *testing.T) {
		p, s, h := idleSweepProducer(t)
		s.mu.Lock()
		s.lastAccess = time.Now().Add(-swTestOpts().PauseIdleThreshold / 2)
		s.mu.Unlock()

		p.sweepPause()

		pauseN, _, _ := h.counts()
		assert.False(t, swIsPaused(s), "a client still inside its idle budget must keep its encoder running")
		assert.Zero(t, pauseN, "no SIGSTOP may be sent to a session pulled from recently")
	})

	t.Run("past the threshold suspends", func(t *testing.T) {
		p, s, h := idleSweepProducer(t)
		s.mu.Lock()
		s.lastAccess = time.Now().Add(-swTestOpts().PauseIdleThreshold - time.Second)
		s.mu.Unlock()

		p.sweepPause()

		pauseN, _, paused := h.counts()
		assert.True(t, swIsPaused(s), "an encoder nobody has pulled from past the threshold must be suspended")
		assert.Equal(t, 1, pauseN)
		assert.True(t, paused)
		s.mu.Lock()
		since := s.pausedSince
		s.mu.Unlock()
		assert.False(t, since.IsZero(), "pausedSince drives the kill sweep and must be stamped")
	})
}

// A second sweep over an already-suspended session must not send a second
// SIGSTOP: pausedSince would be pushed forward on every tick and the
// KillPausedAfter sweep could then never reclaim the encoder's ~1 GB.
func TestSWSweepPause_DoesNotRestampAnAlreadySuspendedSession(t *testing.T) {
	p, s, h := idleSweepProducer(t)
	s.mu.Lock()
	s.lastAccess = time.Now().Add(-time.Hour)
	s.mu.Unlock()

	p.sweepPause()
	s.mu.Lock()
	firstStamp := s.pausedSince
	s.mu.Unlock()

	p.sweepPause()

	pauseN, _, _ := h.counts()
	assert.Equal(t, 1, pauseN, "an already-suspended encoder must not be SIGSTOPped again")
	s.mu.Lock()
	secondStamp := s.pausedSince
	s.mu.Unlock()
	assert.Equal(t, firstStamp, secondStamp, "pausedSince must not be pushed forward by a later sweep")
}
