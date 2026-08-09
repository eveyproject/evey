package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RemoveHash is the surgical counterpart of RemoveSession: a viewer who
// switched audio track left ONE hash behind, and the other hashes of the same
// title (a second tab on another track) must survive it.
func TestSoftwareProducer_RemoveHash_KillsOnlyThatHashAndFreesItsSlot(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(2)
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	p.WithVideoSessionLimiter(limiter)
	defer p.Close()

	fpA := minimalFingerprint(t)
	fpB := minimalFingerprint(t)
	fpB.AudioTrack = fpA.AudioTrack + 1 // same title, the track the viewer switched to
	require.NotEqual(t, fpA.Hash(), fpB.Hash())

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fpA, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fpB, 0, 0))
	require.Equal(t, 2, limiter.Active())
	// The switch happened after the viewer stopped fetching the old track -
	// past reclaimMinIdle, so the reclaim is allowed to take it.
	ageSWLastAccess(p, fpA.Hash(), 2*reclaimMinIdle)

	killed := p.RemoveHash(fpA.Hash(), "superseded")
	assert.Equal(t, 1, killed)
	assert.Equal(t, 1, limiter.Active(), "the abandoned hash must give its video slot back at once")
	assert.Equal(t, 1, p.ActiveSessionCount(), "the sibling hash must survive")
	assert.False(t, limiter.Holds(fpA.Hash()))
	assert.True(t, limiter.Holds(fpB.Hash()))
}

func TestSoftwareProducer_RemoveHash_UnknownHashIsNoop(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

	assert.Equal(t, 0, p.RemoveHash("not-a-live-hash", "superseded"))
	assert.Equal(t, 0, p.RemoveHash("", "superseded"))
	assert.Equal(t, 1, p.ActiveSessionCount())
}

func TestSegmentOrchestrator_RemoveHash_KillsOnlyThatHash(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(2)
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/m.mkv"}, OrchestratorOptions{}, discardLogger())
	o.WithVideoSessionLimiter(limiter)
	defer o.Close()

	fpA := minimalFingerprint(t)
	fpB := minimalFingerprint(t)
	fpB.AudioTrack = fpA.AudioTrack + 1
	require.NotEqual(t, fpA.Hash(), fpB.Hash())

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fpA, 0, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fpB, 0, 0))
	require.Equal(t, 2, limiter.Active())
	ageOrchLastAccess(o, fpA.Hash(), 2*reclaimMinIdle)

	assert.Equal(t, 1, o.RemoveHash(fpA.Hash(), "superseded"))
	assert.Equal(t, 1, limiter.Active())
	assert.False(t, limiter.Holds(fpA.Hash()))
	assert.True(t, limiter.Holds(fpB.Hash()))
}

// The reclaim decision comes from the StreamHashRegistry, which can only see
// streams carrying a stream token: a client authenticating its HLS fetches with
// a plain JWT (API consumer, plugin, third-party player) holds a real encoder
// while being invisible to it, and another viewer's track switch would kill it.
// A hash somebody is genuinely still fetching is spared - the identity-free half
// of the guarantee.
func TestRemoveHash_SparesAHashStillBeingFetched(t *testing.T) {
	t.Run("software", func(t *testing.T) {
		cache, _ := newTestCache(t)
		p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
		defer p.Close()

		fp := minimalFingerprint(t)
		require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

		assert.Equal(t, 0, p.RemoveHash(fp.Hash(), "superseded"), "just fetched: not dead weight")
		assert.Equal(t, 1, p.ActiveSessionCount())
	})

	t.Run("orchestrator", func(t *testing.T) {
		cache, _ := newTestCache(t)
		o := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, testOpts(), discardLogger())
		defer o.Close()

		fp := minimalFingerprint(t)
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

		assert.Equal(t, 0, o.RemoveHash(fp.Hash(), "superseded"), "just fetched: not dead weight")
		assert.Equal(t, 1, o.ActiveCount())
	})
}

func ageSWLastAccess(p *SoftwareProducer, hash string, by time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.sessions[hash] {
		s.mu.Lock()
		s.lastAccess = s.lastAccess.Add(-by)
		s.mu.Unlock()
	}
}

func ageOrchLastAccess(o *SegmentOrchestrator, hash string, by time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, s := range o.sessions[hash] {
		s.mu.Lock()
		s.lastAccess = s.lastAccess.Add(-by)
		s.mu.Unlock()
	}
}
