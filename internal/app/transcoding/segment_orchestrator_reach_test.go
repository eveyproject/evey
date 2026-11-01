package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Covering is not reaching: a request at 23 waited on a fresh rafale at 0
// whose window reached 29 (prod 2026-09-09, a minute of spinner).
func TestOrchestrator_CoveringRafaleTooFarBehindGetsAFreshOneAtTheRequest(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekReachWindowSegs = 8
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	// The pre-warm: a rafale at 0 that has produced nothing yet.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())

	// The viewer's real position, inside the window but 24 segments past the
	// production point: gets its own rafale, and the first is NOT torn down.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 23))
	require.Eventually(t, func() bool {
		return proc.startCount() == 2 && o.ActiveCount() == 2
	}, eventuallyWait, 10*time.Millisecond)
	assert.Equal(t, 23, proc.lastOpts().StartNumber, "the fresh rafale starts AT the request")
	assert.InDelta(t, 23.0*3.0, proc.lastOpts().StartSec, 0.001)
}

// Ordinary playback draws a segment or two ahead: no spawn.
func TestOrchestrator_RequestWithinReachRidesTheExistingRafale(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekReachWindowSegs = 8
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	// Nothing produced yet (point = -1): segment 7 is the 8th still to produce,
	// the last one inside a reach of 8.
	for seg := 1; seg <= 7; seg++ {
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, seg))
	}
	assert.Equal(t, 1, proc.startCount(), "segments within reach of the point never spawn")
	assert.Equal(t, 1, o.ActiveCount())
}

// The point moves with production (cache hits).
func TestOrchestrator_ReachFollowsTheProductionPoint(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekReachWindowSegs = 8
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	// ffmpeg "produces" through 20; the player consumes 20 from cache.
	writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 20}, fp.UseFMP4)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 20))

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 25))
	assert.Equal(t, 1, proc.startCount(), "25 is 5 past the proven point: ride, don't spawn")
}

// A spawn in flight follows the same rule.
func TestOrchestrator_PendingSpawnFarBehindDoesNotCaptureTheRequest(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekReachWindowSegs = 8
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()
	o.mu.Lock()
	claim := o.claimPendingSpawnLocked(hash, 0, 0)
	near := o.pendingSpawnCoversLocked(hash, 0, 3)
	farAway := o.pendingSpawnCoversLocked(hash, 0, 23)
	o.mu.Unlock()
	o.settlePendingSpawn(hash, claim, nil)

	assert.NotNil(t, near, "3 is within reach of a spawn at 0")
	assert.Nil(t, farAway, "23 is not: it gets its own spawn")
}
