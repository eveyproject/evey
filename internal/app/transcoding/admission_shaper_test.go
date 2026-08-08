package transcoding

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The measured software rungs of a real dev box: 720p costs 0.366 of the
// machine, 1080p 0.431, 1440p 0.530. Budget 1.0.
func shaperFixture(t *testing.T) (*VideoAdmissionShaper, *VideoSessionLimiter, *time.Time) {
	t.Helper()
	samples := []transcoding.BenchRungSample{
		{Ladder: "software", Height: 720, FPS: 78.7, RequiredFPS: 28.8, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 66.9, RequiredFPS: 28.8, Passing: true},
		{Ladder: "software", Height: 1440, FPS: 54.4, RequiredFPS: 28.8, Passing: true},
	}
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(samples))
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }
	return shaper, limiter, &clock
}

var ladder = []int{1440, 1080, 720}

func TestAdmissionShaper_EmptyHost_GrantsTheRequestedTop(t *testing.T) {
	shaper, _, _ := shaperFixture(t)

	h, forced := shaper.CeilingFor("s1", LadderClassSoftware, ladder, transcoding.SourceClass{})

	assert.Equal(t, 1440, h)
	assert.False(t, forced, "an empty host trims nothing")
}

// The whole feature: a second 1440p viewer is served 1080p instead of refused.
func TestAdmissionShaper_SecondViewer_ForcedDownToWhatFits(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440), "first viewer holds 0.530")

	h, forced := shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})

	assert.Equal(t, 1080, h, "0.530 used + 0.431 = 0.961 fits; 1440 (1.060) does not")
	assert.True(t, forced)
}

func TestAdmissionShaper_NothingFits_NoPinAndNoCeiling(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	require.True(t, limiter.TryAcquire("h1", LadderClassSoftware, 1440))
	require.True(t, limiter.TryAcquire("h2", LadderClassSoftware, 1080), "0.961 used, 0.039 left")

	_, forced := shaper.CeilingFor("s3", LadderClassSoftware, ladder, transcoding.SourceClass{})

	assert.False(t, forced, "the ladder stays whole; the shed paths answer this viewer")
	h, _ := shaper.CeilingFor("s3", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Equal(t, 0, h, "and no stale grant was pinned")
}

// The cache-hash invariant: once granted, the ceiling must not move while the
// stream is alive - even if the budget frees up entirely.
func TestAdmissionShaper_PinSurvivesBudgetFreeingWhileFresh(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440))
	h, _ := shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	require.Equal(t, 1080, h)

	limiter.Release("hash-first") // the 1440p viewer stops; budget is empty again

	h, forced := shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Equal(t, 1080, h, "a fresh pin never moves: the player's URLs address these bytes")
	assert.True(t, forced)
}

// A paused player fetches nothing, so its pin goes cold - but its session
// still holds a slot. Re-evaluating would price the stream's own cost against
// itself and lower it mid-play.
func TestAdmissionShaper_ColdPinHeldByALiveSession_DoesNotMove(t *testing.T) {
	shaper, limiter, clock := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440))
	h, _ := shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	require.Equal(t, 1080, h)
	require.True(t, limiter.TryAcquire("hash-s2-final", LadderClassSoftware, 1080), "s2's session spawned")
	shaper.Bind("s2", "hash-s2-final")

	*clock = clock.Add(10 * time.Minute) // long pause, pin is cold

	h, _ = shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Equal(t, 1080, h, "a held stream's ceiling is frozen, cold or not")
}

// The anti-poison property: a viewer shed at spawn must NOT be locked to their
// losing grant. Their pin holds nothing; once cold it re-evaluates against the
// budget of the retry's moment.
func TestAdmissionShaper_AbandonedPinReEvaluatesOnRetry(t *testing.T) {
	shaper, limiter, clock := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440))
	h, _ := shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	require.Equal(t, 1080, h)

	limiter.Release("hash-first")          // the other viewer finished
	*clock = clock.Add(defaultPinFreshFor) // s2 never spawned; pin went cold

	h, forced := shaper.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Equal(t, 1440, h, "the retry sees the budget of ITS moment")
	assert.False(t, forced)
}

func TestAdmissionShaper_BindIsIdempotentAndBounded(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	shaper.CeilingFor("s1", LadderClassSoftware, ladder, transcoding.SourceClass{})

	// "same-hash" is LIVE: it must survive any compaction the churn causes.
	require.True(t, limiter.TryAcquire("same-hash", LadderClassSoftware, 720))
	for range 3 {
		shaper.Bind("s1", "same-hash")
	}
	// Twenty dead variants churn through; compaction may drop them freely.
	for i := range 20 {
		shaper.Bind("s1", fmt.Sprintf("hash-%d", i))
	}

	shaper.mu.Lock()
	defer shaper.mu.Unlock()
	assert.LessOrEqual(t, len(shaper.pins["s1"].boundHashes), maxBoundHashes)
	assert.Contains(t, shaper.pins["s1"].boundHashes, "same-hash",
		"a live hash must never be compacted away by dead churn")
	assert.Equal(t, 1, countOf(shaper.pins["s1"].boundHashes, "same-hash"), "idempotent")
}

// ============================================================================
// StreamPlan reification (Étape 2): the granted decision is an immutable value
// with its own content hash, minted once and replaced (never mutated) on
// supersession.
// ============================================================================

func TestStreamPlan_Hash_IdentityAndDistinctness(t *testing.T) {
	t.Parallel()
	base := StreamPlan{Class: LadderClassSoftware, Ceiling: 1080}
	assert.Equal(t, base.Hash(), StreamPlan{Class: LadderClassSoftware, Ceiling: 1080}.Hash(),
		"the same (class, ceiling) hashes identically")
	assert.NotEqual(t, base.Hash(), StreamPlan{Class: LadderClassSoftware, Ceiling: 1440}.Hash(),
		"a different ceiling is a different plan")
	assert.NotEqual(t, base.Hash(), StreamPlan{Class: LadderClassHW, Ceiling: 1080}.Hash(),
		"a different route (class) is a different plan")
}

func TestAdmissionShaper_PlanFor_ReturnsLivePlanMatchingGrant(t *testing.T) {
	shaper, _, _ := shaperFixture(t)

	g := shaper.GrantRoute("s1", LadderClassSoftware, ladder, nil, transcoding.SourceClass{})
	require.True(t, g.OK)

	plan, ok := shaper.PlanFor("s1")
	require.True(t, ok, "a live stream has a readable plan")
	assert.Equal(t, LadderClassSoftware, plan.Class)
	assert.Equal(t, g.Ceiling, plan.Ceiling)
	assert.Equal(t, g.PlanHash, plan.Hash(), "the grant's PlanHash IS the stored plan's hash")

	_, ok = shaper.PlanFor("never-admitted")
	assert.False(t, ok, "no plan for a stream that was never admitted")
}

// The immutability property that makes the cache-hash invariant structural: a
// live plan's hash never changes across grants; only a supersession (cold +
// unheld, budget changed) mints a NEW plan with a NEW hash. The plan is never
// mutated in place.
func TestAdmissionShaper_PlanHash_StableWhileLive_ChangesOnlyOnSupersession(t *testing.T) {
	shaper, limiter, clock := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440), "first viewer holds 0.530")

	g1 := shaper.GrantRoute("s2", LadderClassSoftware, ladder, nil, transcoding.SourceClass{})
	require.Equal(t, 1080, g1.Ceiling, "forced to 1080 by the first viewer's load")

	// While fresh, every re-grant returns the SAME immutable plan/hash - even
	// after the budget frees entirely.
	limiter.Release("hash-first")
	g2 := shaper.GrantRoute("s2", LadderClassSoftware, ladder, nil, transcoding.SourceClass{})
	assert.Equal(t, g1.PlanHash, g2.PlanHash, "a live plan's hash is frozen; it is never mutated")
	assert.Equal(t, 1080, g2.Ceiling)

	// Cold and unheld: the plan is superseded by a NEW one against the now-empty
	// budget - a different ceiling, hence a different hash.
	*clock = clock.Add(defaultPinFreshFor)
	g3 := shaper.GrantRoute("s2", LadderClassSoftware, ladder, nil, transcoding.SourceClass{})
	require.Equal(t, 1440, g3.Ceiling, "the retry sees the budget of its moment")
	assert.NotEqual(t, g1.PlanHash, g3.PlanHash, "supersession mints a new plan identity")
	assert.Equal(t, StreamPlan{Class: LadderClassSoftware, Ceiling: 1440}.Hash(), g3.PlanHash)
}

// ============================================================================
// Frozen ladder (Étape 2 read-verbatim): the shaped ladder is emitted once and
// read back verbatim while the stream is live, so a mid-stream drift of the
// mutable shaping inputs (complexity, capability, CMCD/av1 caps) cannot break the
// cache-hash invariant. Lifecycle matches the pin exactly.
// ============================================================================

func ladderProfiles(heights ...int) []transcoding.HLSProfile {
	out := make([]transcoding.HLSProfile, len(heights))
	for i, h := range heights {
		out[i] = transcoding.HLSProfile{Height: h, Width: h * 16 / 9}
	}
	return out
}

func TestAdmissionShaper_FrozenLadder_ReturnedWhileFresh_IsACopy(t *testing.T) {
	shaper, _, _ := shaperFixture(t)
	require.True(t, shaper.GrantRoute("s1", LadderClassSoftware, ladder, nil, transcoding.SourceClass{}).OK)

	src := ladderProfiles(1440, 1080, 720)
	shaper.FreezeLadder("s1", src)
	src[0].Height = 99 // mutate the caller's slice after freezing

	got, ok := shaper.FrozenLadder("s1")
	require.True(t, ok)
	assert.Equal(t, []int{1440, 1080, 720}, heightsOf(got), "FreezeLadder stores a copy, immune to caller mutation")

	got[1].Height = 42 // mutate the returned slice
	got2, _ := shaper.FrozenLadder("s1")
	assert.Equal(t, 1080, got2[1].Height, "FrozenLadder returns a copy, immune to reader mutation")
}

func TestAdmissionShaper_FrozenLadder_NoPinOrNoLadder(t *testing.T) {
	shaper, _, _ := shaperFixture(t)

	// No pin at all: freeze is a no-op, read is empty.
	shaper.FreezeLadder("ghost", ladderProfiles(1080))
	_, ok := shaper.FrozenLadder("ghost")
	assert.False(t, ok, "no pin => nothing frozen")

	// Pin exists but nothing frozen yet.
	require.True(t, shaper.GrantRoute("s1", LadderClassSoftware, ladder, nil, transcoding.SourceClass{}).OK)
	_, ok = shaper.FrozenLadder("s1")
	assert.False(t, ok, "a pin with no frozen ladder returns false")
}

func TestAdmissionShaper_FrozenLadder_ColdUnheld_ReturnsFalse(t *testing.T) {
	shaper, _, clock := shaperFixture(t)
	require.True(t, shaper.GrantRoute("s1", LadderClassSoftware, ladder, nil, transcoding.SourceClass{}).OK)
	shaper.FreezeLadder("s1", ladderProfiles(1080, 720))

	*clock = clock.Add(defaultPinFreshFor) // cold, and no session bound to it

	_, ok := shaper.FrozenLadder("s1")
	assert.False(t, ok, "an abandoned (cold + unheld) stream re-derives, it does not read a stale frozen ladder")
}

func TestAdmissionShaper_FrozenLadder_ColdButHeld_StaysFrozen(t *testing.T) {
	shaper, limiter, clock := shaperFixture(t)
	require.True(t, shaper.GrantRoute("s1", LadderClassSoftware, ladder, nil, transcoding.SourceClass{}).OK)
	shaper.FreezeLadder("s1", ladderProfiles(1080, 720))

	require.True(t, limiter.TryAcquire("s1-hash", LadderClassSoftware, 1080))
	shaper.Bind("s1", "s1-hash")
	*clock = clock.Add(10 * time.Minute) // paused: cold, but its session still holds a slot

	got, ok := shaper.FrozenLadder("s1")
	require.True(t, ok, "a held stream's ladder is frozen, cold or not")
	assert.Equal(t, []int{1080, 720}, heightsOf(got))
}

func heightsOf(ps []transcoding.HLSProfile) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Height
	}
	return out
}

func countOf(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}

func TestAdmissionShaper_NilReceiverAndNilLimiter_AreInert(t *testing.T) {
	var s *VideoAdmissionShaper
	h, forced := s.CeilingFor("k", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Equal(t, 0, h)
	assert.False(t, forced)
	s.Bind("k", "h")

	s2 := &VideoAdmissionShaper{}
	h, forced = s2.CeilingFor("k", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Equal(t, 0, h)
	assert.False(t, forced)
}

func TestAdmissionShaper_PinMapStaysBounded(t *testing.T) {
	shaper, _, _ := shaperFixture(t)
	shaper.maxPins = 8

	for i := range 50 {
		shaper.CeilingFor(fmt.Sprintf("s%d", i), LadderClassSoftware, []int{720}, transcoding.SourceClass{})
	}

	shaper.mu.Lock()
	defer shaper.mu.Unlock()
	assert.LessOrEqual(t, len(shaper.pins), 8)
}

func TestAdmissionShaper_ConcurrentAccess_Race(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("s%d", i%5)
			shaper.CeilingFor(key, LadderClassSoftware, ladder, transcoding.SourceClass{})
			shaper.Bind(key, fmt.Sprintf("hash-%d", i))
			if limiter.TryAcquire(fmt.Sprintf("hash-%d", i), LadderClassSoftware, 720) {
				limiter.Release(fmt.Sprintf("hash-%d", i))
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================================
// LadderClass-aware demotion (Phase 2, hw ladder)
// ============================================================================

// The exact scenario Phase 2 exists for: a GPU under pressure demotes a
// viewer to a CHEAPER hw rung instead of refusing them outright - and the
// demotion target is picked from the SAME (hw) class's table, not software's.
func TestAdmissionShaper_ForcedDemotion_LandsOnTheRequestedClassCheapestFit(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)

	// Occupy ~0.735 of the GPU pool with 3 other hw 1080p streams (0.245
	// each) - leaves room for another 0.245 (1080p) but not 0.300 (1440p) or
	// 0.465 (2160p).
	for i := range 3 {
		require.True(t, limiter.TryAcquire(fmt.Sprintf("occupant-%d", i), LadderClassHW, 1080))
	}

	h, forced := shaper.CeilingFor("viewer", LadderClassHW, []int{2160, 1440, 1080, 720}, transcoding.SourceClass{})
	assert.Equal(t, 1080, h, "2160 (0.465) and 1440 (0.300) overflow the GPU pool; 1080 (0.245) fits")
	assert.True(t, forced)
}

// The cache-hash invariant extended to classes: a pin granted under one
// class must not move just because a LATER call (e.g. a retry re-routed from
// hw to software after a demotion) asks under a different class, as long as
// the pin is fresh or still held - moving it would desync the already
// dispatched fingerprint hash from what the shaper now reports.
func TestAdmissionShaper_FreshPin_KeepsItsOwnClassIgnoringLaterRequests(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	h, _ := shaper.CeilingFor("stream", LadderClassHW, []int{1440}, transcoding.SourceClass{})
	require.Equal(t, 1440, h, "granted at hw's 1440p cost (0.300), well under budget")

	h2, forced := shaper.CeilingFor("stream", LadderClassSoftware, []int{1440}, transcoding.SourceClass{})
	assert.Equal(t, 1440, h2, "a fresh pin ignores a later request's class and returns its OWN grant")
	assert.False(t, forced)
}

// P5: a hardware stream arriving on a full GPU pool spills to the software
// route instead of being refused - the machine says no only when EVERYTHING
// is really full.
func TestAdmissionShaper_GPUFull_SpillsToSoftwareRoute(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	// Fill the GPU pool so no hw rung fits: 4 x 0.245 = 0.98 leaves 0.02,
	// below the cheapest hw rung (0.245).
	for _, k := range []string{"gpu-1", "gpu-2", "gpu-3", "gpu-4"} {
		require.True(t, limiter.TryAcquire(k, LadderClassHW, 1080))
	}

	swHeights := []int{1440, 1080, 720}
	g := shaper.GrantRoute("newcomer", LadderClassHW, []int{2160, 1440, 1080, 720}, func() []int { return swHeights }, transcoding.SourceClass{})

	require.True(t, g.OK, "CPU pool is empty: the stream must be served, not refused")
	assert.Equal(t, LadderClassSoftware, g.Class, "spilled to the software route")
	assert.True(t, g.Forced)
	assert.Equal(t, 1440, g.Ceiling, "granted the tallest SOFTWARE rung that fits an empty CPU pool")

	// The spill is pinned: manifest and segments must keep seeing it.
	cls, ok := shaper.PinnedClass("newcomer")
	require.True(t, ok)
	assert.Equal(t, LadderClassSoftware, cls)
}

func TestAdmissionShaper_BothPoolsFull_RefusesWithoutPinning(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	for i, k := range []string{"g1", "g2", "g3", "g4"} {
		require.True(t, limiter.TryAcquire(k, LadderClassHW, 1080), "gpu %d", i)
	}
	require.True(t, limiter.TryAcquire("c1", LadderClassSoftware, 1440))
	require.True(t, limiter.TryAcquire("c2", LadderClassSoftware, 1080), "cpu at 0.961")

	g := shaper.GrantRoute("newcomer", LadderClassHW, []int{1080}, func() []int { return []int{1080, 720} }, transcoding.SourceClass{})
	assert.False(t, g.OK, "everything is genuinely full: refuse")
	_, pinned := shaper.PinnedClass("newcomer")
	assert.False(t, pinned, "a refusal must not poison the retry")
}

// The spill pin obeys the same stability rules as a plain ceiling: while
// fresh, it stays software even after the GPU pool frees (the player's URLs
// address software-shaped bytes); once cold and unheld, it re-evaluates and
// the stream goes back to hardware.
func TestAdmissionShaper_SpillPin_StableWhileFresh_BackToHWWhenCold(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	for _, k := range []string{"g1", "g2", "g3", "g4"} {
		require.True(t, limiter.TryAcquire(k, LadderClassHW, 1080))
	}
	natural := []int{2160, 1440, 1080, 720}
	sw := func() []int { return []int{1440, 1080, 720} }

	g := shaper.GrantRoute("s", LadderClassHW, natural, sw, transcoding.SourceClass{})
	require.Equal(t, LadderClassSoftware, g.Class)

	for _, k := range []string{"g1", "g2", "g3", "g4"} {
		limiter.Release(k) // the GPU empties entirely
	}

	g2 := shaper.GrantRoute("s", LadderClassHW, natural, sw, transcoding.SourceClass{})
	assert.Equal(t, LadderClassSoftware, g2.Class, "fresh pin: the route must not move mid-play")

	clock2 := clock.Add(defaultPinFreshFor)
	shaper.now = func() time.Time { return clock2 }

	g3 := shaper.GrantRoute("s", LadderClassHW, natural, sw, transcoding.SourceClass{})
	assert.Equal(t, LadderClassHW, g3.Class, "cold and unheld: back to the hardware route")
	assert.Equal(t, 2160, g3.Ceiling)
	assert.False(t, g3.Forced)
}

// A software-natural stream never spills anywhere: the fallback is a
// hw-only affordance (there is no 'spill software to gpu').
func TestAdmissionShaper_SoftwareNatural_NeverSpills(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	require.True(t, limiter.TryAcquire("c1", LadderClassSoftware, 1440))
	require.True(t, limiter.TryAcquire("c2", LadderClassSoftware, 1080))

	g := shaper.GrantRoute("s", LadderClassSoftware, []int{1440, 1080, 720}, func() []int {
		t.Fatal("fallbackHeights must never be consulted for a software-natural stream")
		return nil
	}, transcoding.SourceClass{})
	assert.False(t, g.OK)
}

// A full pin map must never evict a pin a LIVE session is bound to: moving an
// active stream's ceiling desyncs the player's URLs from the bytes. Cold
// unheld pins are the only legitimate eviction victims; when everything is
// held, the map grows past its soft cap rather than corrupting a stream.
func TestAdmissionShaper_FullMap_NeverEvictsAHeldPin(t *testing.T) {
	t.Parallel()
	// Counter-only limiter: holding 4 sessions is the point, not the budget.
	limiter := NewVideoSessionLimiter(0)
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }
	shaper.maxPins = 4

	// Four HELD pins fill the map: each is bound to a live session.
	for i := range 4 {
		key := fmt.Sprintf("held-%d", i)
		hash := fmt.Sprintf("hash-%d", i)
		g := shaper.GrantRoute(key, LadderClassSoftware, []int{720}, nil, transcoding.SourceClass{})
		require.True(t, g.OK)
		require.True(t, limiter.TryAcquire(hash, LadderClassSoftware, 720))
		shaper.Bind(key, hash)
	}
	// Age everything: freshness alone no longer protects the held pins.
	clock = clock.Add(defaultPinFreshFor * 2)

	// A newcomer wants a pin while the map is full of held ones.
	shaper.GrantRoute("newcomer", LadderClassSoftware, []int{720}, nil, transcoding.SourceClass{})

	for i := range 4 {
		cls, ok := shaper.PinnedClass(fmt.Sprintf("held-%d", i))
		assert.True(t, ok, "held pin %d must survive a full-map insertion", i)
		if ok {
			assert.Equal(t, LadderClassSoftware, cls)
		}
	}
}

// The whole-system property test: many concurrent viewers going through the
// REAL lifecycle (grant -> acquire -> bind -> release) with mixed classes,
// spills, retries and abandonments. Two invariants must hold: no pool's
// reserved cost ever exceeds its budget (+epsilon) at any observed instant,
// and once everyone is gone, both pools and the stream count return exactly
// to zero - any drift here means a leak that would rot into a permanent 503.
func TestAdmission_StressLifecycle_PoolsNeverOverflowAndDrainToZero(t *testing.T) {
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	shaper := NewVideoAdmissionShaper(limiter)

	hwLadder := []int{2160, 1440, 1080, 720}
	swLadder := []int{1440, 1080, 720}
	classes := []LadderClass{LadderClassHW, LadderClassSoftware, LadderClassSoftwareHDR}

	var wg sync.WaitGroup
	var overflowed atomic.Bool
	stop := make(chan struct{})

	// A watcher samples the invariant while the storm runs.
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if limiter.UsedFor(PoolCPU) > limiter.BudgetFor(PoolCPU)+1e-6 ||
				limiter.UsedFor(PoolGPU) > limiter.BudgetFor(PoolGPU)+1e-6 {
				overflowed.Store(true)
				return
			}
		}
	}()

	for v := range 60 {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			natural := classes[v%len(classes)]
			// Named distinctly from the package-level `ladder` fixture: shadowing
			// it here made the intent ambiguous at a glance.
			viewerLadder := swLadder
			if natural == LadderClassHW {
				viewerLadder = hwLadder
			}
			key := fmt.Sprintf("title-%d", v%17) // some viewers share a title
			for range 8 {
				g := shaper.GrantRoute(key, natural, viewerLadder, func() []int { return swLadder }, transcoding.SourceClass{})
				if !g.OK {
					continue // shed; the retry loops
				}
				hash := fmt.Sprintf("fp-%d-%d", v, g.Ceiling)
				if !limiter.TryAcquire(hash, g.Class, g.Ceiling) {
					continue // lost the spawn race; shed at the segment
				}
				shaper.Bind(key, hash)
				if v%5 == 0 {
					// An ABR sibling joins and leaves.
					if limiter.TryAcquire(hash, g.Class, g.Ceiling) {
						limiter.Release(hash)
					}
				}
				limiter.Release(hash)
			}
		}(v)
	}
	// Wait for the viewers only, then stop the watcher.
	wg.Wait()
	close(stop)
	<-watcherDone

	assert.False(t, overflowed.Load(), "a pool exceeded its budget mid-storm")
	assert.Zero(t, limiter.Active(), "every stream must have drained")
	assert.InDelta(t, 0, limiter.UsedFor(PoolCPU), 1e-9, "CPU pool must drain to zero")
	assert.InDelta(t, 0, limiter.UsedFor(PoolGPU), 1e-9, "GPU pool must drain to zero")
}

// The 9th fingerprint variant of one title (audio tracks x burn-ins x
// offsets) used to be silently dropped from the pin's bound set: a pin held
// ONLY by that 9th live session looked unheld - evictable, re-evaluable, its
// ceiling mobile under a live stream. Bind now compacts dead hashes out of
// the full set before recording the newcomer.
func TestAdmissionShaper_NinthBoundVariant_StillProtectsThePin(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0)
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	g := shaper.GrantRoute("title", LadderClassSoftware, []int{1080}, nil, transcoding.SourceClass{})
	require.True(t, g.OK)

	// Eight variants come and go: their sessions are all DEAD by now.
	for i := range 8 {
		shaper.Bind("title", fmt.Sprintf("dead-%d", i))
	}
	// The 9th variant is the one actually playing.
	require.True(t, limiter.TryAcquire("live-9th", LadderClassSoftware, 1080))
	shaper.Bind("title", "live-9th")

	clock = clock.Add(defaultPinFreshFor * 2) // cold: only held-ness protects it

	g2 := shaper.GrantRoute("title", LadderClassSoftware, []int{1080}, nil, transcoding.SourceClass{})
	require.True(t, g2.OK)
	assert.Equal(t, 1080, g2.Ceiling, "the pin must survive: its 9th variant is live")

	// And eviction pressure must not kill it either.
	shaper.maxPins = 1
	shaper.mu.Lock()
	evictable := !shaper.anyBoundHeld(shaper.pins["title"])
	shaper.mu.Unlock()
	assert.False(t, evictable, "a pin whose 9th variant is live is held, full set or not")
}

// Pathological ceiling: 8 variants of one title SIMULTANEOUSLY live plus a
// 9th - the set cannot compact. The pin must fail SAFE: treated as held
// (immortal) rather than silently unprotected.
func TestAdmissionShaper_NineLiveVariants_PinFailsSafeAsHeld(t *testing.T) {
	t.Parallel()
	limiter := NewVideoSessionLimiter(0)
	shaper := NewVideoAdmissionShaper(limiter)
	clock := time.Unix(1_700_000_000, 0)
	shaper.now = func() time.Time { return clock }

	g := shaper.GrantRoute("title", LadderClassSoftware, []int{1080}, nil, transcoding.SourceClass{})
	require.True(t, g.OK)
	for i := range 9 {
		h := fmt.Sprintf("live-%d", i)
		require.True(t, limiter.TryAcquire(h, LadderClassSoftware, 1080))
		shaper.Bind("title", h)
	}

	clock = clock.Add(defaultPinFreshFor * 2)
	shaper.mu.Lock()
	held := shaper.anyBoundHeld(shaper.pins["title"])
	shaper.mu.Unlock()
	assert.True(t, held, "an over-capacity pin with live sessions fails safe as held")
}

// `top` is the MAXIMUM of the requested heights, not the first entry - callers
// pass a ladder and nothing promises it is sorted. The max-scan and the
// "was the ceiling trimmed?" comparisons that depend on it both survived
// mutation testing, and `forced` is what tells the caller its ladder was cut.
func TestAdmissionShaper_CeilingFor_TopIsTheMaxWhateverTheOrder(t *testing.T) {
	shaper, _, _ := shaperFixture(t)

	// Same set, deliberately unsorted: the answer must not depend on order.
	h, forced := shaper.CeilingFor("s1", LadderClassSoftware, []int{720, 1440, 1080}, transcoding.SourceClass{})
	assert.Equal(t, 1440, h, "the tallest rung is the top, wherever it sits in the slice")
	assert.False(t, forced, "granting the tallest requested rung is not a trim")

	// A single-rung ladder: granting it is never forced.
	shaper2, _, _ := shaperFixture(t)
	h2, forced2 := shaper2.CeilingFor("s2", LadderClassSoftware, []int{720}, transcoding.SourceClass{})
	assert.Equal(t, 720, h2)
	assert.False(t, forced2, "serving the only rung asked for is not a trim")
}

// forced must be FALSE when the grant equals the requested top and TRUE only
// when it is strictly below - an off-by-one here either hides a real demotion
// from the operator or reports one that did not happen.
func TestAdmissionShaper_ForcedIsStrictlyBelowTheRequestedTop(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)

	// Ask for exactly what the host can serve alone: no trim.
	h, forced := shaper.CeilingFor("s1", LadderClassSoftware, []int{1080}, transcoding.SourceClass{})
	assert.Equal(t, 1080, h)
	assert.False(t, forced, "a grant equal to the requested top is not forced")

	// Now load the box so the same request can only get 720.
	shaper2, limiter2, _ := shaperFixture(t)
	require.True(t, limiter2.TryAcquire("holder", LadderClassSoftware, 1440))
	h2, forced2 := shaper2.CeilingFor("s2", LadderClassSoftware, ladder, transcoding.SourceClass{})
	assert.Less(t, h2, 1440, "a loaded host must trim")
	assert.True(t, forced2, "a grant below the requested top is forced")

	_ = limiter
}
