package transcoding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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
	// "any/2160p" row — wildcard (unknown codec at write time).
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
	// rolling-upgrade window — should match height-only.
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
