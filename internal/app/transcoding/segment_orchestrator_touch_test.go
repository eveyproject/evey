package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rafales of one hash share a cache dir: a hit at 24 was written by the
// rafale at 23, not the one at 0.
func TestOrchestrator_CacheHitCreditsTheClosestBehindRafale(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.SeekReachWindowSegs = 8
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 23))
	require.Eventually(t, func() bool { return o.ActiveCount() == 2 }, time.Second, 10*time.Millisecond)

	writeReadySegment(t, cache, SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 24}, fp.UseFMP4)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 24))

	o.mu.Lock()
	var at0, at23 int
	for _, s := range o.sessions[fp.Hash()] {
		s.mu.Lock()
		switch s.startSeg {
		case 0:
			at0 = s.producedHighWater
		case 23:
			at23 = s.producedHighWater
		}
		s.mu.Unlock()
	}
	o.mu.Unlock()
	assert.Equal(t, 24, at23, "the rafale at 23 wrote segment 24")
	assert.Equal(t, -1, at0, "the rafale at 0 has produced nothing and must not be credited")
}
