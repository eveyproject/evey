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
	mu    sync.Mutex
	rows  map[string]domaintranscoding.CapOverride
	calls int
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
	s := NewCapOverrideService(repo, nil, CapOverrideServiceOpts{}, nil)
	_, ok := s.OverrideForHeight(2160)
	assert.False(t, ok)
}

func TestCapOverrideService_OverrideForHeight_MatchesByBucket(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	// Seed an active override on the 2160p bucket.
	_ = repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "2160p",
		MaxHeight:   1080,
		AppliedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	s := NewCapOverrideService(repo, nil, CapOverrideServiceOpts{}, nil)
	assertNoError(t, s.Refresh(context.Background()))

	// 4K source → 2160p bucket → hit
	o, ok := s.OverrideForHeight(2160)
	assert.True(t, ok)
	assert.Equal(t, 1080, o.MaxHeight)

	// Letterboxed 1600 → 1440p bucket → miss (no row)
	_, ok = s.OverrideForHeight(1600)
	assert.False(t, ok)

	// 720p source → no bucket → never hit
	_, ok = s.OverrideForHeight(720)
	assert.False(t, ok)
}

func TestCapOverrideService_Refresh_UpdatesCache(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	s := NewCapOverrideService(repo, nil, CapOverrideServiceOpts{}, nil)
	_, ok := s.OverrideForHeight(2160)
	assert.False(t, ok, "cache empty before first refresh")

	_ = repo.Upsert(context.Background(), domaintranscoding.CapOverride{
		SourceClass: "2160p", MaxHeight: 1080,
		AppliedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	assertNoError(t, s.Refresh(context.Background()))
	_, ok = s.OverrideForHeight(2160)
	assert.True(t, ok, "refresh must publish new rows to the cache")
}

func TestLowerCapFor_StepsDownOneRungAtATime(t *testing.T) {
	assert.Equal(t, 1080, lowerCapFor("2160p"))
	assert.Equal(t, 1080, lowerCapFor("1440p"))
	assert.Equal(t, 720, lowerCapFor("1080p"))
	assert.Equal(t, 0, lowerCapFor("720p"), "below 1080p we don't override (handler floor protects)")
	assert.Equal(t, 0, lowerCapFor("unknown"))
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
