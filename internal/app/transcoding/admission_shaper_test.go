package transcoding

import (
	"fmt"
	"sync"
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

	h, forced := shaper.CeilingFor("s1", LadderClassSoftware, ladder)

	assert.Equal(t, 1440, h)
	assert.False(t, forced, "an empty host trims nothing")
}

// The whole feature: a second 1440p viewer is served 1080p instead of refused.
func TestAdmissionShaper_SecondViewer_ForcedDownToWhatFits(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440), "first viewer holds 0.530")

	h, forced := shaper.CeilingFor("s2", LadderClassSoftware, ladder)

	assert.Equal(t, 1080, h, "0.530 used + 0.431 = 0.961 fits; 1440 (1.060) does not")
	assert.True(t, forced)
}

func TestAdmissionShaper_NothingFits_NoPinAndNoCeiling(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	require.True(t, limiter.TryAcquire("h1", LadderClassSoftware, 1440))
	require.True(t, limiter.TryAcquire("h2", LadderClassSoftware, 1080), "0.961 used, 0.039 left")

	_, forced := shaper.CeilingFor("s3", LadderClassSoftware, ladder)

	assert.False(t, forced, "the ladder stays whole; the shed paths answer this viewer")
	h, _ := shaper.CeilingFor("s3", LadderClassSoftware, ladder)
	assert.Equal(t, 0, h, "and no stale grant was pinned")
}

// The cache-hash invariant: once granted, the ceiling must not move while the
// stream is alive - even if the budget frees up entirely.
func TestAdmissionShaper_PinSurvivesBudgetFreeingWhileFresh(t *testing.T) {
	shaper, limiter, _ := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440))
	h, _ := shaper.CeilingFor("s2", LadderClassSoftware, ladder)
	require.Equal(t, 1080, h)

	limiter.Release("hash-first") // the 1440p viewer stops; budget is empty again

	h, forced := shaper.CeilingFor("s2", LadderClassSoftware, ladder)
	assert.Equal(t, 1080, h, "a fresh pin never moves: the player's URLs address these bytes")
	assert.True(t, forced)
}

// A paused player fetches nothing, so its pin goes cold - but its session
// still holds a slot. Re-evaluating would price the stream's own cost against
// itself and lower it mid-play.
func TestAdmissionShaper_ColdPinHeldByALiveSession_DoesNotMove(t *testing.T) {
	shaper, limiter, clock := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440))
	h, _ := shaper.CeilingFor("s2", LadderClassSoftware, ladder)
	require.Equal(t, 1080, h)
	require.True(t, limiter.TryAcquire("hash-s2-final", LadderClassSoftware, 1080), "s2's session spawned")
	shaper.Bind("s2", "hash-s2-final")

	*clock = clock.Add(10 * time.Minute) // long pause, pin is cold

	h, _ = shaper.CeilingFor("s2", LadderClassSoftware, ladder)
	assert.Equal(t, 1080, h, "a held stream's ceiling is frozen, cold or not")
}

// The anti-poison property: a viewer shed at spawn must NOT be locked to their
// losing grant. Their pin holds nothing; once cold it re-evaluates against the
// budget of the retry's moment.
func TestAdmissionShaper_AbandonedPinReEvaluatesOnRetry(t *testing.T) {
	shaper, limiter, clock := shaperFixture(t)
	require.True(t, limiter.TryAcquire("hash-first", LadderClassSoftware, 1440))
	h, _ := shaper.CeilingFor("s2", LadderClassSoftware, ladder)
	require.Equal(t, 1080, h)

	limiter.Release("hash-first")          // the other viewer finished
	*clock = clock.Add(defaultPinFreshFor) // s2 never spawned; pin went cold

	h, forced := shaper.CeilingFor("s2", LadderClassSoftware, ladder)
	assert.Equal(t, 1440, h, "the retry sees the budget of ITS moment")
	assert.False(t, forced)
}

func TestAdmissionShaper_BindIsIdempotentAndBounded(t *testing.T) {
	shaper, _, _ := shaperFixture(t)
	shaper.CeilingFor("s1", LadderClassSoftware, ladder)

	for range 3 {
		shaper.Bind("s1", "same-hash")
	}
	for i := range 20 {
		shaper.Bind("s1", fmt.Sprintf("hash-%d", i))
	}

	shaper.mu.Lock()
	defer shaper.mu.Unlock()
	assert.LessOrEqual(t, len(shaper.pins["s1"].boundHashes), maxBoundHashes)
	assert.Equal(t, "same-hash", shaper.pins["s1"].boundHashes[0])
}

func TestAdmissionShaper_NilReceiverAndNilLimiter_AreInert(t *testing.T) {
	var s *VideoAdmissionShaper
	h, forced := s.CeilingFor("k", LadderClassSoftware, ladder)
	assert.Equal(t, 0, h)
	assert.False(t, forced)
	s.Bind("k", "h")

	s2 := &VideoAdmissionShaper{}
	h, forced = s2.CeilingFor("k", LadderClassSoftware, ladder)
	assert.Equal(t, 0, h)
	assert.False(t, forced)
}

func TestAdmissionShaper_PinMapStaysBounded(t *testing.T) {
	shaper, _, _ := shaperFixture(t)
	shaper.maxPins = 8

	for i := range 50 {
		shaper.CeilingFor(fmt.Sprintf("s%d", i), LadderClassSoftware, []int{720})
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
			shaper.CeilingFor(key, LadderClassSoftware, ladder)
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

	h, forced := shaper.CeilingFor("viewer", LadderClassHW, []int{2160, 1440, 1080, 720})
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

	h, _ := shaper.CeilingFor("stream", LadderClassHW, []int{1440})
	require.Equal(t, 1440, h, "granted at hw's 1440p cost (0.300), well under budget")

	h2, forced := shaper.CeilingFor("stream", LadderClassSoftware, []int{1440})
	assert.Equal(t, 1440, h2, "a fresh pin ignores a later request's class and returns its OWN grant")
	assert.False(t, forced)
}
