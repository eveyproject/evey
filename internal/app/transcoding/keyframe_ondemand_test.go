package transcoding_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

type fakeKfStore struct {
	mu       sync.Mutex
	saved    map[string][]int64
	rows     int
	saveErr  error
	setCalls int
}

func (f *fakeKfStore) ListVideoKeyframesMissing(context.Context, int, []string) ([]string, error) {
	return nil, nil
}

func (f *fakeKfStore) SetKeyframes(_ context.Context, id shared.MediaItemID, kf []int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	if f.saved == nil {
		f.saved = map[string][]int64{}
	}
	f.saved[string(id)] = kf
	return f.rows, nil
}

type fakeKfProber struct {
	kf    []int64
	err   error
	block bool          // when true, blocks until the ctx dies (simulates the ffprobe fallback)
	delay time.Duration // sleeps before answering (simulates the mkvgo cluster walk)
	calls atomic.Int32
}

func (f *fakeKfProber) ProbeKeyframes(ctx context.Context, _ string) ([]int64, error) {
	f.calls.Add(1)
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.kf, f.err
}

func TestKeyframeOnDemand_IndexesFastPath(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{kf: []int64{0, 6000, 12000}}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)

	assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
	assert.Equal(t, []int64{0, 6000, 12000}, store.saved["m1"])
}

func TestKeyframeOnDemand_FailureIsNegativeCached(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{err: errors.New("no cues")}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)

	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"),
		"second attempt within the TTL must not re-probe")
	assert.Equal(t, int32(1), prober.calls.Load(), "the failure is cached, the probe ran once")
}

func TestKeyframeOnDemand_BudgetDetachesSlowProbe(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{block: true}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).WithInlineBudget(50 * time.Millisecond)

	start := time.Now()
	ok := k.TryIndex(context.Background(), "m1", "/f.mkv")
	assert.False(t, ok)
	assert.Less(t, time.Since(start), 2*time.Second,
		"the negotiation must return at the inline budget — the mkvgo cluster walk does not honour ctx cancellation, so the wait and the work are decoupled")
}

// A probe slower than the inline budget still lands in the background: this
// playback transcodes (pinned), the NEXT one negotiates the copy rung.
func TestKeyframeOnDemand_SlowProbePersistsInBackground(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{kf: []int64{0, 6000}, delay: 150 * time.Millisecond}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).WithInlineBudget(20 * time.Millisecond)

	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"), "too slow for THIS negotiation")
	assert.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.saved["m1"]) == 2
	}, 3*time.Second, 10*time.Millisecond, "the detached probe must persist for the next playback")
}

func TestKeyframeOnDemand_WorkerWonRace(t *testing.T) {
	// rows==0 = the NULL-guarded write found the index already present (the
	// background worker beat us): still a success for the caller — the
	// keyframes exist and the decision can use them.
	store := &fakeKfStore{rows: 0}
	prober := &fakeKfProber{kf: []int64{0, 6000}}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)

	assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
}

func TestKeyframeOnDemand_NilAndEmptyPathSafe(t *testing.T) {
	var k *apptranscoding.KeyframeOnDemand
	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"), "nil receiver is a no-op")

	k2 := apptranscoding.NewKeyframeOnDemand(&fakeKfStore{}, &fakeKfProber{}, nil)
	assert.False(t, k2.TryIndex(context.Background(), "m1", ""))
}
