package transcoding

// White-box: the negative cache (k.failed) used to be append-only - one stale
// entry per failed media for the process lifetime. These tests pin the two
// purge points: lazy delete when the retry window expired, and delete on a
// later success.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type purgeProber struct {
	mu  sync.Mutex
	kf  []int64
	err error
}

func (p *purgeProber) ProbeKeyframes(context.Context, string) ([]int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.kf, p.err
}

type purgeStore struct{}

func (purgeStore) ListVideoKeyframesMissing(context.Context, int, []string, int16) ([]string, error) {
	return nil, nil
}
func (purgeStore) SetKeyframes(context.Context, shared.MediaItemID, []int64) (int, error) {
	return 1, nil
}
func (purgeStore) MarkKeyframesUnindexable(context.Context, shared.MediaItemID, int16) (int, error) {
	return 1, nil
}

func waitFailedState(t *testing.T, k *KeyframeOnDemand, id string, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		k.mu.Lock()
		_, ok := k.failed[id]
		inflight := len(k.inflight)
		k.mu.Unlock()
		if ok == want && inflight == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("failed[%s] never reached presence=%v", id, want)
}

func TestKeyframeOnDemand_ExpiredNegativeEntryIsPurged(t *testing.T) {
	prober := &purgeProber{err: errors.New("still broken")}
	k := NewKeyframeOnDemand(purgeStore{}, prober, nil)

	// Seed an EXPIRED tombstone: the next TryIndex must drop it (and is then
	// free to re-probe; the re-probe fails and lays a FRESH one).
	k.mu.Lock()
	k.failed["m1"] = time.Now().Add(-keyframeOnDemandRetryAfter - time.Minute)
	k.mu.Unlock()

	k.TryIndex(context.Background(), "m1", "/media/movie.mkv")
	waitFailedState(t, k, "m1", true) // fresh tombstone from the re-probe

	k.mu.Lock()
	fresh := time.Since(k.failed["m1"]) < time.Minute
	k.mu.Unlock()
	if !fresh {
		t.Fatal("the expired tombstone must be replaced by a fresh one, not kept")
	}
}

func TestKeyframeOnDemand_SuccessDropsTheTombstone(t *testing.T) {
	prober := &purgeProber{kf: []int64{0, 3000, 6000}}
	k := NewKeyframeOnDemand(purgeStore{}, prober, nil)

	k.mu.Lock()
	k.failed["m1"] = time.Now().Add(-keyframeOnDemandRetryAfter - time.Minute)
	k.mu.Unlock()

	k.TryIndex(context.Background(), "m1", "/media/movie.mkv")
	waitFailedState(t, k, "m1", false)
}
