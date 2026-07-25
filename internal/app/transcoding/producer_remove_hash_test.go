package transcoding

import (
	"context"
	"testing"

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

	assert.Equal(t, 1, o.RemoveHash(fpA.Hash(), "superseded"))
	assert.Equal(t, 1, limiter.Active())
	assert.False(t, limiter.Holds(fpA.Hash()))
	assert.True(t, limiter.Holds(fpB.Hash()))
}
