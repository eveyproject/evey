package transcoding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

type fakeCapOverrideRepo struct {
	mu      sync.Mutex
	rows    map[string]domaintranscoding.CapOverride
	calls   int
	aggRows []domaintranscoding.CapAggregateRow // returned by AggregateStarvation
}

func (r *fakeCapOverrideRepo) AggregateStarvation(_ context.Context, _ time.Duration, _ int) ([]domaintranscoding.CapAggregateRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.aggRows, nil
}

func (r *fakeCapOverrideRepo) Upsert(_ context.Context, o domaintranscoding.CapOverride) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows == nil {
		r.rows = map[string]domaintranscoding.CapOverride{}
	}
	r.rows[o.SourceClass] = o
	return nil
}
func (r *fakeCapOverrideRepo) List(_ context.Context) ([]domaintranscoding.CapOverride, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	out := make([]domaintranscoding.CapOverride, 0, len(r.rows))
	for _, o := range r.rows {
		out = append(out, o)
	}
	return out, nil
}
func (r *fakeCapOverrideRepo) Delete(_ context.Context, sourceClass string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[sourceClass]; !ok {
		return false, nil
	}
	delete(r.rows, sourceClass)
	return true, nil
}

func (r *fakeCapOverrideRepo) DeleteExpired(_ context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	n := 0
	for k, o := range r.rows {
		if !o.ExpiresAt.IsZero() && o.ExpiresAt.Before(now) {
			delete(r.rows, k)
			n++
		}
	}
	return n, nil
}

func TestCapOverrideService_OverrideForHeight_EmptyCacheReturnsFalse(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	_, ok := s.OverrideForHeight("hevc", 2160)
	assert.False(t, ok)
}

func TestCapOverrideService_OverrideForHeight_ExactCodecMatchWins(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	// Seed two buckets: HEVC 2160p starves, H264 2160p doesn't (no row).
	_ = repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "hevc/2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, s.Refresh(context.Background()))

	// HEVC 4K source → hit.
	o, ok := s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok)
	assert.Equal(t, 1080, o.MaxHeight)

	// H264 4K source on same machine → no override (lighter decode).
	_, ok = s.OverrideForHeight("h264", 2160)
	assert.False(t, ok, "per-codec granularity must NOT bleed into other codecs")

	// HEVC 720p → below floor, never hit.
	_, ok = s.OverrideForHeight("hevc", 720)
	assert.False(t, ok)
}

func TestCapOverrideService_OverrideForHeight_WildcardCodecFallback(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	// "any/2160p" row - wildcard (unknown codec at write time).
	_ = repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "any/2160p", MaxHeight: 1440,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, s.Refresh(context.Background()))

	// A typed codec request still matches the wildcard.
	o, ok := s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok, "wildcard codec bucket must match any codec at lookup time")
	assert.Equal(t, 1440, o.MaxHeight)
}

func TestCapOverrideService_OverrideForHeight_LegacyMVPRowMatches(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	// Pre-per-codec MVP row still lingering in the DB during the
	// rolling-upgrade window - should match height-only.
	_ = repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, s.Refresh(context.Background()))

	o, ok := s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok)
	assert.Equal(t, 1080, o.MaxHeight)
}

func TestCapOverrideService_OverrideForHeight_ExactBeatsWildcardBeatsLegacy(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	// All three buckets present → exact codec match wins.
	for _, o := range []domaintranscoding.CapOverride{
		{SourceClass: "hevc/2160p", MaxHeight: 1080, ExpiresAt: time.Now().Add(time.Hour)},
		{SourceClass: "any/2160p", MaxHeight: 1440, ExpiresAt: time.Now().Add(time.Hour)},
		{SourceClass: "2160p", MaxHeight: 720, ExpiresAt: time.Now().Add(time.Hour)},
	} {
		_ = repo.Upsert(context.Background(), o)
	}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, s.Refresh(context.Background()))

	o, ok := s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok)
	assert.Equal(t, 1080, o.MaxHeight, "exact (hevc/2160p=1080) must beat wildcard (1440) and legacy (720)")
}

func TestCapOverrideService_Refresh_UpdatesCache(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	_, ok := s.OverrideForHeight("hevc", 2160)
	assert.False(t, ok, "cache empty before first refresh")

	_ = repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "hevc/2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	assertNoError(t, s.Refresh(context.Background()))
	_, ok = s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok, "refresh must publish new rows to the cache")
}

func TestLowerCapFor_StepsDownOneRungAtATime(t *testing.T) {
	// New format: "<codec>/<height>"
	assert.Equal(t, 1080, lowerCapFor("hevc/2160p"))
	assert.Equal(t, 1080, lowerCapFor("h264/1440p"))
	assert.Equal(t, 720, lowerCapFor("av1/1080p"))
	assert.Equal(t, 0, lowerCapFor("hevc/720p"), "below 1080p we don't override (handler floor protects)")
	// Legacy format also tolerated (rolling upgrade window).
	assert.Equal(t, 1080, lowerCapFor("2160p"))
	assert.Equal(t, 720, lowerCapFor("1080p"))
	assert.Equal(t, 0, lowerCapFor("unknown"))
	assert.Equal(t, 0, lowerCapFor(""))
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// ============================================================================
// Admin-facing surface: reset, snapshot, and the on-demand tick
// ============================================================================

// The admin "reset cap" button must be visible IMMEDIATELY: the handler reads
// the cache, not the table, so a delete that skips the refresh leaves
// /admin/qoe showing - and the ladder still enforcing - an override the admin
// just removed.
func TestCapOverrideService_ResetOverride_RefreshesTheCacheAtOnce(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "hevc/2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}))
	assertNoError(t, s.Refresh(context.Background()))
	_, ok := s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok, "premise: the override is enforced before the reset")

	removed, err := s.ResetOverride(context.Background(), "hevc/2160p")
	assertNoError(t, err)
	assert.True(t, removed, "an existing row must report as removed")

	_, ok = s.OverrideForHeight("hevc", 2160)
	assert.False(t, ok, "the handler must stop enforcing the override without waiting for the next tick")
}

// Resetting a class that carries no override is not an error - the admin UI
// offers the button per source class, not per existing row.
func TestCapOverrideService_ResetOverride_UnknownClassIsNotAnError(t *testing.T) {
	s := NewCapOverrideService(&fakeCapOverrideRepo{}, CapOverrideServiceOpts{}, nil)

	removed, err := s.ResetOverride(context.Background(), "av1/1080p")
	assertNoError(t, err)
	assert.False(t, removed, "nothing was there to remove")
}

// ListActive hands the admin panel a copy: what it shows IS what the handler
// enforces, so a caller mutating the returned slice (or the JSON encoder
// reordering it) must not be able to corrupt the enforced snapshot.
func TestCapOverrideService_ListActive_ReturnsADefensiveCopy(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "hevc/2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}))
	assertNoError(t, s.Refresh(context.Background()))

	snapshot := s.ListActive()
	assert.Len(t, snapshot, 1)
	snapshot[0].MaxHeight = 240 // a caller scribbling on its copy

	again := s.ListActive()
	assert.Equal(t, 1080, again[0].MaxHeight,
		"the enforced snapshot must survive a caller mutating an earlier copy")
	cap, ok := s.OverrideForHeight("hevc", 2160)
	assert.True(t, ok)
	assert.Equal(t, 1080, cap.MaxHeight, "the handler must still read the real ceiling")
}

// Before the first refresh there is nothing to show - and nothing to panic on.
func TestCapOverrideService_ListActive_EmptyBeforeAnyRefresh(t *testing.T) {
	s := NewCapOverrideService(&fakeCapOverrideRepo{}, CapOverrideServiceOpts{}, nil)
	assert.Empty(t, s.ListActive())
}

// Tick is the admin "recompute now" action and the loop body: expire, then
// aggregate, then publish. The publish is what makes the pass observable - a
// tick that aggregates without refreshing writes a row nobody enforces until
// the NEXT tick.
func TestCapOverrideService_Tick_ExpiresThenPublishes(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)

	// One live override and one that expired an hour ago.
	assertNoError(t, repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "hevc/2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}))
	assertNoError(t, repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "h264/1080p", MaxHeight: 720,
		AppliedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour),
	}))
	assertNoError(t, s.Refresh(context.Background()))
	assert.Len(t, s.ListActive(), 2, "premise: both rows are cached before the tick")

	assertNoError(t, s.Tick(context.Background()))

	active := s.ListActive()
	assert.Len(t, active, 1, "the expired override must be gone from the enforced snapshot")
	assert.Equal(t, "hevc/2160p", active[0].SourceClass)
}

// ============================================================================
// Option defaults and the legacy-class height buckets
// ============================================================================

// The defaults are what a partially-configured service runs on. A TickInterval
// of 0 panics time.NewTicker; a MinSamples of 0 lets a single unlucky playback
// cap a whole source class; a StarvedThreshold of 0 caps EVERY class (0 >= 0).
func TestCapOverrideServiceOpts_ApplyDefaults_FillsEveryZeroField(t *testing.T) {
	var o CapOverrideServiceOpts
	o.applyDefaults()

	assert.Equal(t, 30*time.Minute, o.TickInterval)
	assert.Equal(t, 24*time.Hour, o.AggregationWindow)
	assert.Equal(t, 50, o.MinSamples)
	assert.InDelta(t, 0.30, o.StarvedThreshold, 0.0001)
	assert.Equal(t, 7*24*time.Hour, o.OverrideTTL)
}

// An operator's value must survive - including one that happens to match a
// default, and one deliberately far from it.
func TestCapOverrideServiceOpts_ApplyDefaults_KeepsCallerValues(t *testing.T) {
	o := CapOverrideServiceOpts{
		TickInterval:      time.Minute,
		AggregationWindow: time.Hour,
		MinSamples:        5,
		StarvedThreshold:  0.9,
		OverrideTTL:       time.Hour,
	}
	want := o
	o.applyDefaults()

	assert.Equal(t, want, o, "a fully specified options struct must come back unchanged")
}

// ⚠️ Documented, not endorsed: the guards test `== 0`, not `<= 0`, so a NEGATIVE
// value passes straight through. A negative TickInterval panics time.NewTicker
// in Start. Nothing in the repo passes one today (the only caller builds the
// struct literally), so this pins the real behaviour rather than papering over
// it - if a knob ever becomes operator-settable, the guard must become `<= 0`.
func TestCapOverrideServiceOpts_ApplyDefaults_NegativesPassThroughUnguarded(t *testing.T) {
	o := CapOverrideServiceOpts{TickInterval: -time.Second, MinSamples: -1, StarvedThreshold: -1}
	o.applyDefaults()

	assert.Equal(t, -time.Second, o.TickInterval, "the `== 0` guard does not catch negatives")
	assert.Equal(t, -1, o.MinSamples)
	assert.InDelta(t, -1.0, o.StarvedThreshold, 0.0001)
}

// The legacy (pre-per-codec) class strings bucket by height with >= boundaries.
// Reading a boundary one rung off would make a 1440p source match the "2160p"
// override - capping a stream that was never measured as starving.
func TestCapOverrideService_OverrideForHeight_LegacyBucketBoundaries(t *testing.T) {
	cases := []struct {
		height    int
		wantClass string
	}{
		{2160, "2160p"},
		{2159, "1440p"}, // one below the 4K boundary drops a bucket
		{1440, "1440p"},
		{1439, "1080p"},
		{1080, "1080p"},
	}

	for _, tc := range cases {
		t.Run(tc.wantClass+"/"+itoa(tc.height), func(t *testing.T) {
			repo := &fakeCapOverrideRepo{}
			s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
			assertNoError(t, repo.Upsert(context.Background(), domaintranscoding.CapOverride{
				SourceClass: tc.wantClass, MaxHeight: 720,
				AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
			}))
			assertNoError(t, s.Refresh(context.Background()))

			got, ok := s.OverrideForHeight("hevc", tc.height)
			assert.True(t, ok, "height %d must fall in the %s legacy bucket", tc.height, tc.wantClass)
			assert.Equal(t, tc.wantClass, got.SourceClass)
		})
	}
}

// Below 1080p there is no legacy bucket at all: those heights were never
// capped by the pre-per-codec worker, so a legacy row must not match them.
func TestCapOverrideService_OverrideForHeight_NoLegacyBucketBelow1080(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	assertNoError(t, repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "1080p", MaxHeight: 720,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}))
	assertNoError(t, s.Refresh(context.Background()))

	_, ok := s.OverrideForHeight("hevc", 1079)
	assert.False(t, ok, "a 1079-tall source has no legacy bucket and must not inherit the 1080p cap")
}
