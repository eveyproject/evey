package transcoding

import (
	"fmt"
	"sync"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoSessionLimiter_UnlimitedByDefault(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0)
	for i := range 50 {
		require.True(t, l.TryAcquire(fmt.Sprintf("stream-%d", i), 1080))
	}
	assert.Equal(t, 50, l.Active(), "unlimited admits every distinct stream")
	assert.Equal(t, 0, l.Limit())
}

func TestVideoSessionLimiter_AcquireUntilLimitThenRefuse(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(2)
	require.True(t, l.TryAcquire("stream-1", 1080))
	require.True(t, l.TryAcquire("stream-2", 1080))
	assert.False(t, l.TryAcquire("stream-3", 1080), "a third distinct stream must be refused at limit=2")
	assert.Equal(t, 2, l.Active())
}

func TestVideoSessionLimiter_ReleaseFreesASlot(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire("stream-1", 1080))
	require.False(t, l.TryAcquire("stream-2", 1080), "a second distinct stream is refused at limit=1")
	l.Release("stream-1")
	assert.Equal(t, 0, l.Active())
	require.True(t, l.TryAcquire("stream-2", 1080))
}

func TestVideoSessionLimiter_DoubleReleaseNeverGoesNegative(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire("h1", 1080))
	l.Release("h1")
	l.Release("h1") // stray extra release
	assert.Equal(t, 0, l.Active())
	// still usable, not corrupted into a negative-active state that would
	// let unbounded acquires through.
	require.True(t, l.TryAcquire("h1", 1080))
	assert.Equal(t, 1, l.Active())
}

func TestVideoSessionLimiter_SetLimitAppliesLiveWithoutKillingActive(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(5)
	require.True(t, l.TryAcquire("stream-1", 1080))
	require.True(t, l.TryAcquire("stream-2", 1080))
	assert.Equal(t, 2, l.Active())

	l.SetLimit(2)
	assert.Equal(t, 2, l.Active(), "SetLimit never kills already-acquired slots")
	assert.False(t, l.TryAcquire("stream-3", 1080), "at the new lower limit, a NEW stream is refused")
	assert.True(t, l.TryAcquire("stream-1", 1080), "an existing stream's sibling encoder still joins its own slot")
	l.Release("stream-1")

	l.Release("stream-1")
	require.True(t, l.TryAcquire("stream-3", 1080), "a freed slot is usable again under the new limit")
}

func TestVideoSessionLimiter_SetLimitNegativeClampsToUnlimited(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	l.SetLimit(-3)
	assert.Equal(t, 0, l.Limit())
	for range 10 {
		require.True(t, l.TryAcquire("h1", 1080))
	}
}

func TestVideoSessionLimiter_ConcurrentAcquireRelease_Race(t *testing.T) {
	l := NewVideoSessionLimiter(4)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if l.TryAcquire("h1", 1080) {
				l.Release("h1")
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 0, l.Active())
}

// The bug that live testing caught: with the cap at 1 and slots counted per
// ffmpeg process, one viewer whose player probed a second ABR rung refused
// ITSELF (503). Slots are per STREAM: sibling encoders of the same decision
// hash join the stream's slot instead of competing for a new one.
func TestVideoSessionLimiter_SameStreamJoinsItsOwnSlot(t *testing.T) {
	l := NewVideoSessionLimiter(1)

	if !l.TryAcquire("stream-A", 1080) {
		t.Fatal("first encoder of the only allowed stream must be admitted")
	}
	if !l.TryAcquire("stream-A", 1080) {
		t.Fatal("an ABR sibling of the SAME stream must never be refused by its own slot")
	}
	if l.Active() != 1 {
		t.Errorf("two encoders of one stream = 1 slot, got %d", l.Active())
	}
	if l.TryAcquire("stream-B", 1080) {
		t.Error("a DIFFERENT stream must still be refused at capacity")
	}

	// The slot frees only once the stream's last encoder is gone.
	l.Release("stream-A")
	if l.Active() != 1 {
		t.Errorf("slot must survive while one sibling encoder remains, got %d", l.Active())
	}
	if l.TryAcquire("stream-B", 1080) {
		t.Error("still at capacity: the stream has not fully released")
	}
	l.Release("stream-A")
	if l.Active() != 0 {
		t.Errorf("last encoder gone = slot freed, got %d", l.Active())
	}
	if !l.TryAcquire("stream-B", 1080) {
		t.Error("a new stream must be admitted once the slot freed")
	}
}

func TestVideoSessionLimiter_ReleaseOfUnknownStreamIsNoop(t *testing.T) {
	l := NewVideoSessionLimiter(2)
	l.Release("never-acquired")
	if l.Active() != 0 {
		t.Fatalf("stray release must not corrupt the count, got %d", l.Active())
	}
	if !l.TryAcquire("a", 1080) || !l.TryAcquire("b", 1080) || l.TryAcquire("c", 1080) {
		t.Fatal("capacity must still be exactly 2 after a stray release")
	}
}

// ============================================================================
// Cost-model-aware admission (WithCostModel / SetBudget)
// ============================================================================

// costModelFixture mirrors the real measured "software" ladder from the
// capability audit: 720p -> 0.366, 1080p -> 0.431, 1440p -> 0.530.
func costModelFixture(t *testing.T) *VideoCostModel {
	t.Helper()
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 82.0, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 69.6, RequiredFPS: 30.0, Passing: true},
		{Ladder: "software", Height: 1440, FPS: 56.6, RequiredFPS: 30.0, Passing: true},
	})
	require.NotNil(t, m)
	return m
}

func TestVideoSessionLimiter_CostModel_1440pPlus1080p_BothAdmitted(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1440", 1440), "1440p alone must fit under budget 1.0")
	require.True(t, l.TryAcquire("stream-1080", 1080), "1440p (0.530) + 1080p (0.431) = 0.961 <= 1.0 must both fit")
	assert.Equal(t, 2, l.Active())
	assert.InDelta(t, 0.961, l.Used(), 0.01)
}

func TestVideoSessionLimiter_CostModel_Two1440p_SecondRefused(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", 1440))
	assert.False(t, l.TryAcquire("stream-2", 1440), "2x 1440p = 1.060 > budget 1.0, the second must be refused")
	assert.Equal(t, 1, l.Active())
}

func TestVideoSessionLimiter_CostModel_SiblingOfSameStreamAlwaysAdmittedEvenAtFullBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", 1440))
	require.True(t, l.TryAcquire("stream-2", 1080))
	// budget is full (0.961 used, another 1440p or 1080p would overflow it)
	assert.False(t, l.TryAcquire("stream-3", 1080))
	// but a sibling encoder of an EXISTING stream must still join for free
	assert.True(t, l.TryAcquire("stream-1", 1440), "ABR sibling of an admitted stream must join regardless of budget")
	assert.Equal(t, 2, l.Active())
}

func TestVideoSessionLimiter_CostModel_ReleaseGivesBackBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", 1440))
	require.False(t, l.TryAcquire("stream-2", 1440))
	l.Release("stream-1")
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon)
	assert.True(t, l.TryAcquire("stream-2", 1440), "released budget must be reusable")
}

func TestVideoSessionLimiter_CostModel_SetLimitStillEnforcedAlongsideBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", 720), "720p alone (cost 0.366) easily fits the budget")
	assert.False(t, l.TryAcquire("stream-2", 720), "the counter cap (SetLimit 1) must still refuse a second stream even though the budget has room")
}

func TestVideoSessionLimiter_NilCostModel_MatchesOldCounterBehaviour(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(2) // no WithCostModel call at all

	require.True(t, l.TryAcquire("stream-1", 2160))
	require.True(t, l.TryAcquire("stream-2", 2160))
	assert.False(t, l.TryAcquire("stream-3", 2160), "with no cost model, admission is purely by counter regardless of height")
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon, "Used stays 0 with no cost model wired")
}

func TestVideoSessionLimiter_CostModel_DoubleReleaseDoesNotUnderflowBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", 1440))
	l.Release("stream-1")
	l.Release("stream-1") // stray extra release
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon)
	require.True(t, l.TryAcquire("stream-2", 1440))
	require.True(t, l.TryAcquire("stream-3", 1080))
}

func TestVideoSessionLimiter_CostModel_ConcurrentAcquireRelease_Race(t *testing.T) {
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			key := fmt.Sprintf("stream-%d", i)
			if l.TryAcquire(key, 1080) {
				l.Release(key)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 0, l.Active())
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon)
}

func TestVideoSessionLimiter_SetBudget_ClampsNonPositiveToDefault(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0)
	assert.InDelta(t, 1.0, l.Budget(), budgetEpsilon)
	l.SetBudget(-1)
	assert.InDelta(t, 1.0, l.Budget(), budgetEpsilon)
	l.SetBudget(0)
	assert.InDelta(t, 1.0, l.Budget(), budgetEpsilon)
	l.SetBudget(2.0)
	assert.InDelta(t, 2.0, l.Budget(), budgetEpsilon)
}
