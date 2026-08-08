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
		require.True(t, l.TryAcquire(fmt.Sprintf("stream-%d", i), LadderClassSoftware, 1080))
	}
	assert.Equal(t, 50, l.Active(), "unlimited admits every distinct stream")
	assert.Equal(t, 0, l.Limit())
}

func TestVideoSessionLimiter_AcquireUntilLimitThenRefuse(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(2)
	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1080))
	require.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 1080))
	assert.False(t, l.TryAcquire("stream-3", LadderClassSoftware, 1080), "a third distinct stream must be refused at limit=2")
	assert.Equal(t, 2, l.Active())
}

func TestVideoSessionLimiter_ReleaseFreesASlot(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1080))
	require.False(t, l.TryAcquire("stream-2", LadderClassSoftware, 1080), "a second distinct stream is refused at limit=1")
	l.Release("stream-1")
	assert.Equal(t, 0, l.Active())
	require.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 1080))
}

func TestVideoSessionLimiter_DoubleReleaseNeverGoesNegative(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	require.True(t, l.TryAcquire("h1", LadderClassSoftware, 1080))
	l.Release("h1")
	l.Release("h1") // stray extra release
	assert.Equal(t, 0, l.Active())
	// still usable, not corrupted into a negative-active state that would
	// let unbounded acquires through.
	require.True(t, l.TryAcquire("h1", LadderClassSoftware, 1080))
	assert.Equal(t, 1, l.Active())
}

func TestVideoSessionLimiter_SetLimitAppliesLiveWithoutKillingActive(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(5)
	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1080))
	require.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 1080))
	assert.Equal(t, 2, l.Active())

	l.SetLimit(2)
	assert.Equal(t, 2, l.Active(), "SetLimit never kills already-acquired slots")
	assert.False(t, l.TryAcquire("stream-3", LadderClassSoftware, 1080), "at the new lower limit, a NEW stream is refused")
	assert.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1080), "an existing stream's sibling encoder still joins its own slot")
	l.Release("stream-1")

	l.Release("stream-1")
	require.True(t, l.TryAcquire("stream-3", LadderClassSoftware, 1080), "a freed slot is usable again under the new limit")
}

func TestVideoSessionLimiter_SetLimitNegativeClampsToUnlimited(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	l.SetLimit(-3)
	assert.Equal(t, 0, l.Limit())
	for range 10 {
		require.True(t, l.TryAcquire("h1", LadderClassSoftware, 1080))
	}
}

func TestVideoSessionLimiter_ConcurrentAcquireRelease_Race(t *testing.T) {
	l := NewVideoSessionLimiter(4)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if l.TryAcquire("h1", LadderClassSoftware, 1080) {
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

	if !l.TryAcquire("stream-A", LadderClassSoftware, 1080) {
		t.Fatal("first encoder of the only allowed stream must be admitted")
	}
	if !l.TryAcquire("stream-A", LadderClassSoftware, 1080) {
		t.Fatal("an ABR sibling of the SAME stream must never be refused by its own slot")
	}
	if l.Active() != 1 {
		t.Errorf("two encoders of one stream = 1 slot, got %d", l.Active())
	}
	if l.TryAcquire("stream-B", LadderClassSoftware, 1080) {
		t.Error("a DIFFERENT stream must still be refused at capacity")
	}

	// The slot frees only once the stream's last encoder is gone.
	l.Release("stream-A")
	if l.Active() != 1 {
		t.Errorf("slot must survive while one sibling encoder remains, got %d", l.Active())
	}
	if l.TryAcquire("stream-B", LadderClassSoftware, 1080) {
		t.Error("still at capacity: the stream has not fully released")
	}
	l.Release("stream-A")
	if l.Active() != 0 {
		t.Errorf("last encoder gone = slot freed, got %d", l.Active())
	}
	if !l.TryAcquire("stream-B", LadderClassSoftware, 1080) {
		t.Error("a new stream must be admitted once the slot freed")
	}
}

func TestVideoSessionLimiter_ReleaseOfUnknownStreamIsNoop(t *testing.T) {
	l := NewVideoSessionLimiter(2)
	l.Release("never-acquired")
	if l.Active() != 0 {
		t.Fatalf("stray release must not corrupt the count, got %d", l.Active())
	}
	if !l.TryAcquire("a", LadderClassSoftware, 1080) || !l.TryAcquire("b", LadderClassSoftware, 1080) || l.TryAcquire("c", LadderClassSoftware, 1080) {
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

	require.True(t, l.TryAcquire("stream-1440", LadderClassSoftware, 1440), "1440p alone must fit under budget 1.0")
	require.True(t, l.TryAcquire("stream-1080", LadderClassSoftware, 1080), "1440p (0.530) + 1080p (0.431) = 0.961 <= 1.0 must both fit")
	assert.Equal(t, 2, l.Active())
	assert.InDelta(t, 0.961, l.Used(), 0.01)
}

func TestVideoSessionLimiter_CostModel_Two1440p_SecondRefused(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1440))
	assert.False(t, l.TryAcquire("stream-2", LadderClassSoftware, 1440), "2x 1440p = 1.060 > budget 1.0, the second must be refused")
	assert.Equal(t, 1, l.Active())
}

func TestVideoSessionLimiter_CostModel_SiblingOfSameStreamAlwaysAdmittedEvenAtFullBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1440))
	require.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 1080))
	// budget is full (0.961 used, another 1440p or 1080p would overflow it)
	assert.False(t, l.TryAcquire("stream-3", LadderClassSoftware, 1080))
	// but a sibling encoder of an EXISTING stream must still join for free
	assert.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1440), "ABR sibling of an admitted stream must join regardless of budget")
	assert.Equal(t, 2, l.Active())
}

func TestVideoSessionLimiter_CostModel_ReleaseGivesBackBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1440))
	require.False(t, l.TryAcquire("stream-2", LadderClassSoftware, 1440))
	l.Release("stream-1")
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon)
	assert.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 1440), "released budget must be reusable")
}

func TestVideoSessionLimiter_CostModel_SetLimitStillEnforcedAlongsideBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 720), "720p alone (cost 0.366) easily fits the budget")
	assert.False(t, l.TryAcquire("stream-2", LadderClassSoftware, 720), "the counter cap (SetLimit 1) must still refuse a second stream even though the budget has room")
}

func TestVideoSessionLimiter_NilCostModel_MatchesOldCounterBehaviour(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(2) // no WithCostModel call at all

	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 2160))
	require.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 2160))
	assert.False(t, l.TryAcquire("stream-3", LadderClassSoftware, 2160), "with no cost model, admission is purely by counter regardless of height")
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon, "Used stays 0 with no cost model wired")
}

func TestVideoSessionLimiter_CostModel_DoubleReleaseDoesNotUnderflowBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))

	require.True(t, l.TryAcquire("stream-1", LadderClassSoftware, 1440))
	l.Release("stream-1")
	l.Release("stream-1") // stray extra release
	assert.InDelta(t, 0.0, l.Used(), budgetEpsilon)
	require.True(t, l.TryAcquire("stream-2", LadderClassSoftware, 1440))
	require.True(t, l.TryAcquire("stream-3", LadderClassSoftware, 1080))
}

func TestVideoSessionLimiter_CostModel_ConcurrentAcquireRelease_Race(t *testing.T) {
	l := NewVideoSessionLimiter(0).WithCostModel(costModelFixture(t))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			key := fmt.Sprintf("stream-%d", i)
			if l.TryAcquire(key, LadderClassSoftware, 1080) {
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
	assert.InDelta(t, 1.0, l.BudgetFor(PoolCPU), budgetEpsilon)
	l.SetBudget(PoolCPU, -1)
	assert.InDelta(t, 1.0, l.BudgetFor(PoolCPU), budgetEpsilon)
	l.SetBudget(PoolCPU, 0)
	assert.InDelta(t, 1.0, l.BudgetFor(PoolCPU), budgetEpsilon)
	l.SetBudget(PoolCPU, 2.0)
	assert.InDelta(t, 2.0, l.BudgetFor(PoolCPU), budgetEpsilon)
	// The other pool is untouched by SetBudget(PoolCPU, ...).
	assert.InDelta(t, 1.0, l.BudgetFor(PoolGPU), budgetEpsilon)
}

// ============================================================================
// Pool separation (CPU vs GPU) - Phase 2, hw ladder class
// ============================================================================

// devBoxCostModel wires the same measured dev-box table as devBoxSamples
// (see video_capacity_test.go): software 720=0.366/1080=0.431/1440=0.530,
// hw 720=0.663/1080=0.245/1440=0.300/2160=0.465.
func devBoxCostModel(t *testing.T) *VideoCostModel {
	t.Helper()
	m := NewVideoCostModel(devBoxSamples())
	require.NotNil(t, m)
	return m
}

// The headline scenario this phase exists for: 4 hw streams and 2 software
// streams run AT THE SAME TIME without stealing budget from each other,
// because they bill two independent pools.
func TestVideoSessionLimiter_PoolSeparation_HWAndCPUDoNotShareBudget(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(devBoxCostModel(t))

	// 4x hw @ 1080p = 4 * 0.245 = 0.98, fits under PoolGPU's 1.0 budget.
	for i := range 4 {
		key := fmt.Sprintf("hw-%d", i)
		require.True(t, l.TryAcquire(key, LadderClassHW, 1080), "hw stream %d must be admitted", i)
	}
	// 2x software @ 1080p = 2 * 0.431 = 0.862, fits under PoolCPU's 1.0
	// budget, concurrently with the 4 hw streams above.
	for i := range 2 {
		key := fmt.Sprintf("sw-%d", i)
		require.True(t, l.TryAcquire(key, LadderClassSoftware, 1080), "software stream %d must be admitted", i)
	}
	assert.Equal(t, 6, l.Active())
	assert.InDelta(t, 0.98, l.UsedFor(PoolGPU), 0.01)
	assert.InDelta(t, 0.862, l.UsedFor(PoolCPU), 0.01)

	// A 5th hw stream overflows PoolGPU's budget (0.98 + 0.245 > 1.0) and is
	// refused, even though PoolCPU still has room.
	assert.False(t, l.TryAcquire("hw-4", LadderClassHW, 1080), "5th hw stream must overflow the GPU pool's budget")
	assert.Equal(t, 6, l.Active())
}

func TestVideoSessionLimiter_GlobalCounterCap_AppliesAcrossClasses(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(3).WithCostModel(devBoxCostModel(t))

	require.True(t, l.TryAcquire("hw-0", LadderClassHW, 1080))
	require.True(t, l.TryAcquire("hw-1", LadderClassHW, 1080))
	require.True(t, l.TryAcquire("sw-0", LadderClassSoftware, 720))
	// Both pools have plenty of budget left, but the counter cap (3) is
	// global across every class.
	assert.False(t, l.TryAcquire("sw-1", LadderClassSoftware, 720), "the counter cap is global, not per-pool")
	assert.Equal(t, 3, l.Active())
}

func TestVideoSessionLimiter_Release_CreditsTheCorrectPool(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(devBoxCostModel(t))

	require.True(t, l.TryAcquire("hw-0", LadderClassHW, 1080))
	require.True(t, l.TryAcquire("sw-0", LadderClassSoftware, 1080))
	assert.InDelta(t, 0.245, l.UsedFor(PoolGPU), 0.001)
	assert.InDelta(t, 0.431, l.UsedFor(PoolCPU), 0.001)

	l.Release("hw-0")
	assert.InDelta(t, 0.0, l.UsedFor(PoolGPU), budgetEpsilon, "releasing the hw stream must not touch the CPU pool")
	assert.InDelta(t, 0.431, l.UsedFor(PoolCPU), 0.001, "the software stream's reservation is untouched")

	l.Release("sw-0")
	assert.InDelta(t, 0.0, l.UsedFor(PoolCPU), budgetEpsilon)
}

func TestVideoSessionLimiter_DoubleRelease_DoesNotUnderflowEitherPool(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(devBoxCostModel(t))

	require.True(t, l.TryAcquire("hw-0", LadderClassHW, 1080))
	require.True(t, l.TryAcquire("sw-0", LadderClassSoftware, 1080))
	l.Release("hw-0")
	l.Release("hw-0") // stray extra release
	l.Release("sw-0")
	l.Release("sw-0") // stray extra release

	assert.InDelta(t, 0.0, l.UsedFor(PoolGPU), budgetEpsilon)
	assert.InDelta(t, 0.0, l.UsedFor(PoolCPU), budgetEpsilon)
	// Still usable: budgets were not corrupted into a negative state that
	// would let unbounded acquires through.
	require.True(t, l.TryAcquire("hw-1", LadderClassHW, 1080))
	require.True(t, l.TryAcquire("sw-1", LadderClassSoftware, 1080))
}

func TestVideoSessionLimiter_PoolSeparation_ConcurrentMixed_Race(t *testing.T) {
	l := NewVideoSessionLimiter(0).WithCostModel(devBoxCostModel(t))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			class := LadderClassSoftware
			if i%2 == 0 {
				class = LadderClassHW
			}
			key := fmt.Sprintf("stream-%d", i)
			if l.TryAcquire(key, class, 1080) {
				l.Release(key)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 0, l.Active())
	assert.InDelta(t, 0.0, l.UsedFor(PoolCPU), budgetEpsilon)
	assert.InDelta(t, 0.0, l.UsedFor(PoolGPU), budgetEpsilon)
}

func TestVideoSessionLimiter_ClassesAndRungsFor_NoCostModel_Empty(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0)
	assert.Nil(t, l.Classes())
	assert.Empty(t, l.RungsFor(LadderClassSoftware))
}

func TestVideoSessionLimiter_ClassesAndRungsFor_ProxyTheWiredCostModel(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(devBoxCostModel(t))

	classes := l.Classes()
	assert.Contains(t, classes, LadderClassSoftware)
	assert.Contains(t, classes, LadderClassHW)

	rungs := l.RungsFor(LadderClassHW)
	assert.NotEmpty(t, rungs, "hw table exists on the dev box fixture")

	assert.Empty(t, l.RungsFor(LadderClassSoftwareAV1), "no av1 table on the dev box fixture")
}

// A negative limit is normalised to 0, and 0 means UNLIMITED here - so getting
// this guard wrong does not merely clamp, it inverts the meaning: a caller
// asking for "-1" would end up with an unlimited admission gate on a weak NAS.
// Both the constructor and the runtime setter carry the guard, and both
// survived mutation testing.
func TestVideoSessionLimiter_NegativeLimitBecomesUnlimited(t *testing.T) {
	l := NewVideoSessionLimiter(-5)
	if l.Limit() != 0 {
		t.Errorf("a negative limit must normalise to 0 (unlimited), got %d", l.Limit())
	}

	l = NewVideoSessionLimiter(3)
	if l.Limit() != 3 {
		t.Fatalf("Limit() = %d, want 3", l.Limit())
	}
	l.SetLimit(-1)
	if l.Limit() != 0 {
		t.Errorf("SetLimit(-1) must normalise to 0, got %d", l.Limit())
	}
	l.SetLimit(0)
	if l.Limit() != 0 {
		t.Errorf("SetLimit(0) must stay 0, got %d", l.Limit())
	}
	l.SetLimit(2)
	if l.Limit() != 2 {
		t.Errorf("SetLimit(2) = %d, want 2", l.Limit())
	}
}

// The used-budget accumulator floors at zero. Without that floor an unbalanced
// release drives the pool NEGATIVE, which reads as "plenty of budget free" and
// over-admits sessions on exactly the hardware the limiter exists to protect.
func TestVideoSessionLimiter_UsedBudgetNeverGoesNegative(t *testing.T) {
	l := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))

	if !l.TryAcquire("s1", LadderClassSoftware, 1080) {
		t.Fatal("the first acquire must succeed")
	}
	used := l.Used()
	if used <= 0 {
		t.Fatalf("an acquired session must consume budget, got %v", used)
	}

	l.Release("s1")
	if got := l.Used(); got < 0 {
		t.Errorf("used budget = %v after release, must never go below zero", got)
	}

	// Releasing a key that is not held must be a no-op, not a decrement.
	l.Release("s1")
	l.Release("never-acquired")
	if got := l.Used(); got < 0 {
		t.Errorf("used budget = %v after spurious releases, must never go below zero", got)
	}
	if l.Active() != 0 {
		t.Errorf("Active() = %d after releasing everything, want 0", l.Active())
	}
}

// Release is reference-counted: a stream acquired twice needs two releases, and
// the budget is only returned on the last one. The `n <= 1` boundary decides
// that, and it survived.
func TestVideoSessionLimiter_ReleaseIsReferenceCounted(t *testing.T) {
	l := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))

	if !l.TryAcquire("s1", LadderClassSoftware, 1080) {
		t.Fatal("first acquire must succeed")
	}
	if !l.TryAcquire("s1", LadderClassSoftware, 1080) {
		t.Fatal("a second acquire of the same key must succeed (same stream, another viewer)")
	}
	if l.Active() != 1 {
		t.Fatalf("Active() = %d, want 1 - the same stream is one slot", l.Active())
	}
	afterTwo := l.Used()

	l.Release("s1")
	if !l.Holds("s1") {
		t.Error("one release of a doubly-held key must keep the slot")
	}
	if l.Used() != afterTwo {
		t.Errorf("budget was returned too early: %v -> %v", afterTwo, l.Used())
	}

	l.Release("s1")
	if l.Holds("s1") {
		t.Error("the last release must drop the slot")
	}
}

// ============================================================================
// Source-aware admission (TryAcquireFor / CanAdmitFor)
// ============================================================================

// prodLimiter is the production box of 2026-08-08: one 480p rung, 2.12x
// realtime on the 4K HEVC the bench decodes, 8.43x on a 1080p H.264 source.
func prodLimiter() *VideoSessionLimiter {
	m := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 480, FPS: 50.8, RequiredFPS: 28.8, Passing: true},
	}).WithDecodeEvidence(
		transcoding.SourceClass{Codec: "hevc", Height: 2160},
		[]transcoding.DecodeRungSample{
			{Codec: "hevc", Height: 2160, FPS: 60},
			{Codec: "h264", Height: 2160, FPS: 240},
		},
	)
	return NewVideoSessionLimiter(0).WithCostModel(m)
}

// The measured reality this exists for: the host sustains eight concurrent
// 1080p H.264 sessions at the 480p rung; admission used to stop at one.
func TestVideoSessionLimiter_TryAcquireFor_CheapSource_AdmitsEight(t *testing.T) {
	t.Parallel()
	l := prodLimiter()
	cheap := transcoding.SourceClass{Codec: "h264", Height: 1080}
	for i := range 8 {
		assert.True(t, l.TryAcquireFor(fmt.Sprintf("viewer-%d", i), LadderClassSoftware, 480, cheap),
			"viewer %d must be admitted: the host measurably sustains eight", i)
	}
	assert.False(t, l.TryAcquireFor("viewer-9", LadderClassSoftware, 480, cheap),
		"the ninth must still be refused - the budget is a budget")
}

// The expensive source keeps the old, correct answer. Widening admission for
// cheap sources must not widen it where a third 4K HEVC measured 0.80x.
func TestVideoSessionLimiter_TryAcquireFor_ExpensiveSource_StillOne(t *testing.T) {
	t.Parallel()
	l := prodLimiter()
	heavy := transcoding.SourceClass{Codec: "hevc", Height: 2160}
	assert.True(t, l.TryAcquireFor("first", LadderClassSoftware, 480, heavy))
	assert.False(t, l.TryAcquireFor("second", LadderClassSoftware, 480, heavy),
		"a second 4K HEVC decode does not fit and never did")
}

// Each stream is billed for what it costs, not for what its neighbour costs.
func TestVideoSessionLimiter_TryAcquireFor_MixedSources_BilledIndependently(t *testing.T) {
	t.Parallel()
	l := prodLimiter()
	heavy := transcoding.SourceClass{Codec: "hevc", Height: 2160}
	cheap := transcoding.SourceClass{Codec: "h264", Height: 1080}
	require.True(t, l.TryAcquireFor("heavy", LadderClassSoftware, 480, heavy))
	// The 4K HEVC viewer took ~0.57 of the pool; the ~0.14 cheap ones still fit.
	assert.True(t, l.TryAcquireFor("cheap-1", LadderClassSoftware, 480, cheap))
	assert.True(t, l.TryAcquireFor("cheap-2", LadderClassSoftware, 480, cheap))
	assert.True(t, l.TryAcquireFor("cheap-3", LadderClassSoftware, 480, cheap))
	assert.False(t, l.TryAcquireFor("cheap-4", LadderClassSoftware, 480, cheap),
		"the pool is full once the heavy stream's share plus three cheap ones is spent")
}

// Release must credit back exactly what was reserved, or the pool leaks.
func TestVideoSessionLimiter_TryAcquireFor_ReleaseCreditsTheSourcePrice(t *testing.T) {
	t.Parallel()
	l := prodLimiter()
	heavy := transcoding.SourceClass{Codec: "hevc", Height: 2160}
	cheap := transcoding.SourceClass{Codec: "h264", Height: 1080}
	require.True(t, l.TryAcquireFor("heavy", LadderClassSoftware, 480, heavy))
	require.False(t, l.TryAcquireFor("heavy-2", LadderClassSoftware, 480, heavy))
	l.Release("heavy")
	for i := range 8 {
		assert.True(t, l.TryAcquireFor(fmt.Sprintf("cheap-%d", i), LadderClassSoftware, 480, cheap),
			"the whole budget must come back on release")
	}
}

// The shed probe must agree with the spawn, or a viewer is turned away for a
// price nothing charges.
func TestVideoSessionLimiter_CanAdmitFor_AgreesWithTryAcquireFor(t *testing.T) {
	t.Parallel()
	l := prodLimiter()
	cheap := transcoding.SourceClass{Codec: "h264", Height: 1080}
	for i := range 8 {
		key := fmt.Sprintf("viewer-%d", i)
		assert.True(t, l.CanAdmitFor(key, LadderClassSoftware, 480, cheap), "probe %d", i)
		require.True(t, l.TryAcquireFor(key, LadderClassSoftware, 480, cheap))
	}
	assert.False(t, l.CanAdmitFor("viewer-9", LadderClassSoftware, 480, cheap),
		"the probe must refuse exactly where the spawn does")
}

// TryAcquire (no source named) must behave exactly as it always did.
func TestVideoSessionLimiter_TryAcquire_UnknownSource_UnchangedBehaviour(t *testing.T) {
	t.Parallel()
	l := prodLimiter()
	assert.True(t, l.TryAcquire("first", LadderClassSoftware, 480))
	assert.False(t, l.TryAcquire("second", LadderClassSoftware, 480),
		"without a named source the old output-rung price applies")
}

// The count cap must come from the limiter's OWN cost model and budgets. It
// used to be computed inline at the wiring site while the admin API derived a
// different number from the tallest bench rung, so the panel reported a limit
// nobody enforced.
func TestVideoSessionLimiter_AutoCountCap_SumsThePoolsCheapestRungs(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))

	// software cheapest = 0.366 -> 2 ; hw cheapest = 0.245 -> 4
	assert.Equal(t, 2, l.AutoCountCapFor(PoolCPU))
	assert.Equal(t, 4, l.AutoCountCapFor(PoolGPU))
	assert.Equal(t, 6, l.AutoCountCap())
}

// No hw evidence must not conjure a GPU allowance.
func TestVideoSessionLimiter_AutoCountCap_NoHWTable_NoGPUAllowance(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "software", Height: 1080, FPS: 60, RequiredFPS: 30, Passing: true},
	}))

	assert.Equal(t, 0, l.AutoCountCapFor(PoolGPU))
	assert.Equal(t, 2, l.AutoCountCap())
}

// No cost model, nothing to derive - the caller decides what to do with 0.
func TestVideoSessionLimiter_AutoCountCap_NoCostModel_Zero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, NewVideoSessionLimiter(0).AutoCountCap())
}
