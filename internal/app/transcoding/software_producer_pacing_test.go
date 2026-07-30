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
	p.NotifyClientHint(fp.MediaID, 0, true, 0)

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
	p.NotifyClientHint(fp.MediaID, 0, true, 200)

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

	p.NotifyClientHint(fp.MediaID, 90_000, false, 0)

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
		p.NotifyClientHint("media-nobody-is-serving", 0, true, 0)
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
	p.NotifyClientHint(fp.MediaID, 0, true, 10)

	assert.True(t, swIsPaused(survivors[0]),
		"a session starting past the requested segment must not be woken to serve it")
}
