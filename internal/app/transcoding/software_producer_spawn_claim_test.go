package transcoding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Concurrent misses at the same position must start ONE encoder: without a
// claim taken before the slow source resolution, every racer exec'd its own
// ffmpeg and all but one were stopped right after.
func TestSoftwareProducer_ConcurrentMissesSpawnOneEncoder(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	slowSource := SourceResolverFunc(func(context.Context, DecisionFingerprint) (string, []SourceStream, error) {
		time.Sleep(50 * time.Millisecond)
		return "/media/test.mkv", nil, nil
	})
	p := NewSoftwareProducer(cache, proc, slowSource, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			assert.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 10))
		})
	}
	wg.Wait()
	require.Equal(t, 1, p.ActiveSessionCount())
	assert.Equal(t, 1, proc.startCount(), "one encoder for one position")
}

// A racer parked on a claim must receive the winner's refusal, not nil.
func TestSoftwareProducer_ClaimWaiterGetsTheCapacityError(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	slowSource := SourceResolverFunc(func(context.Context, DecisionFingerprint) (string, []SourceStream, error) {
		time.Sleep(50 * time.Millisecond)
		return "/media/test.mkv", nil, nil
	})
	limiter := NewVideoSessionLimiter(1)
	require.True(t, limiter.TryAcquire("another-stream", LadderClassSoftware, 720))
	p := NewSoftwareProducer(cache, proc, slowSource, swTestOpts(), discardLogger())
	p.WithVideoSessionLimiter(limiter)
	defer p.Close()

	fp := minimalFingerprint(t)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			assert.ErrorIs(t, p.EnsureSegmentRange(context.Background(), fp, 0, 10), ErrVideoCapacityExhausted)
		})
	}
	wg.Wait()
	assert.Equal(t, 0, proc.startCount())
}
