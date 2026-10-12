package introproviders

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

type fakeCacheEntry struct {
	payload   []byte
	fetchedAt time.Time
}

type fakeCacheRepo struct {
	store map[string]fakeCacheEntry
	gets  int
	puts  int
}

func newFakeCacheRepo() *fakeCacheRepo {
	return &fakeCacheRepo{store: map[string]fakeCacheEntry{}}
}

func (r *fakeCacheRepo) key(tmdbID, season, episode int) string {
	return string(rune(tmdbID)) + ":" + string(rune(season)) + ":" + string(rune(episode))
}

func (r *fakeCacheRepo) Get(_ context.Context, tmdbID, season, episode int) ([]byte, time.Time, error) {
	r.gets++
	entry, ok := r.store[r.key(tmdbID, season, episode)]
	if !ok {
		return nil, time.Time{}, nil
	}
	return entry.payload, entry.fetchedAt, nil
}

func (r *fakeCacheRepo) Put(_ context.Context, tmdbID, season, episode int, _ string, payload []byte) error {
	r.puts++
	r.store[r.key(tmdbID, season, episode)] = fakeCacheEntry{payload: payload, fetchedAt: time.Now()}
	return nil
}

// setEntry inserts an entry with an explicit fetchedAt for TTL tests.
func (r *fakeCacheRepo) setEntry(tmdbID, season, episode int, payload []byte, fetchedAt time.Time) {
	r.store[r.key(tmdbID, season, episode)] = fakeCacheEntry{payload: payload, fetchedAt: fetchedAt}
}

type fakeInner struct {
	segs  []domainlibrary.MediaSegment
	err   error
	calls int
}

func (f *fakeInner) GetSegments(_ context.Context, _ *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	f.calls++
	return f.segs, f.err
}

// First call hits the inner provider and stores; second call uses the
// cache and never calls the inner. This is the daily-quota shield.
func TestCached_HitOnSecondCall(t *testing.T) {
	t.Parallel()
	episode, repoBackend := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repoBackend)
	cache := newFakeCacheRepo()
	inner := &fakeInner{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
	}}

	cached := NewCached(inner, resolver, cache)

	// 1st call → miss, inner runs, cache populated.
	got, err := cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, inner.calls)
	assert.Equal(t, 1, cache.puts)

	// 2nd call → hit, inner stays silent.
	got, err = cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, inner.calls, "inner must not be re-invoked on cache hit")
	assert.Equal(t, 0.0, 0.0)
}

// Empty inner result → cached as `[]` so we negative-cache "neither DB
// knows this item" and stop hammering upstream.
func TestCached_NegativeCaching(t *testing.T) {
	t.Parallel()
	episode, repoBackend := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repoBackend)
	cache := newFakeCacheRepo()
	inner := &fakeInner{segs: nil}

	cached := NewCached(inner, resolver, cache)

	got, err := cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Empty(t, got)
	require.Equal(t, 1, cache.puts, "negative result must be persisted")

	// Second call should not re-invoke inner - even empty results were cached.
	got, err = cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, 1, inner.calls, "inner must not be re-invoked for negative cache hits")
}

// A bad blob in the cache (corrupted JSON) must trigger refetch, not a
// pipeline failure. This guards against schema migrations / partial
// writes corrupting the cache.
func TestCached_CorruptCacheRefetches(t *testing.T) {
	t.Parallel()
	episode, repoBackend := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repoBackend)
	cache := newFakeCacheRepo()
	cache.setEntry(1396, 1, 5, []byte("not-json"), time.Now())

	inner := &fakeInner{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
	}}
	cached := NewCached(inner, resolver, cache)

	got, err := cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, inner.calls)
	// Refetch must rewrite the cache with valid JSON.
	var parsed []domainlibrary.MediaSegment
	require.NoError(t, json.Unmarshal(cache.store[cache.key(1396, 1, 5)].payload, &parsed))
	assert.Len(t, parsed, 1)
}

// A negative cache row older than negativeTTL must be treated as a miss
// and the upstream retried - TIDB may have new data since the initial scan.
func TestCached_NegativeCacheRespectsTTL(t *testing.T) {
	t.Parallel()
	episode, repoBackend := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repoBackend)
	cache := newFakeCacheRepo()
	// Seed a negative row that is 31 days old.
	staleTime := time.Now().Add(-31 * 24 * time.Hour)
	cache.setEntry(1396, 1, 5, []byte("[]"), staleTime)

	inner := &fakeInner{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
	}}
	cached := NewCached(inner, resolver, cache).WithNegativeTTL(30 * 24 * time.Hour)

	got, err := cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1, "stale negative row must be bypassed and inner called")
	assert.Equal(t, 1, inner.calls, "inner must be called when negative TTL expired")
}

// A negative cache row younger than negativeTTL must still block the
// upstream call - within the window, we trust the cached 404.
func TestCached_NegativeCacheWithinTTL_HitsCache(t *testing.T) {
	t.Parallel()
	episode, repoBackend := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repoBackend)
	cache := newFakeCacheRepo()
	// Seed a negative row that is only 29 days old.
	recentTime := time.Now().Add(-29 * 24 * time.Hour)
	cache.setEntry(1396, 1, 5, []byte("[]"), recentTime)

	inner := &fakeInner{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
	}}
	cached := NewCached(inner, resolver, cache).WithNegativeTTL(30 * 24 * time.Hour)

	got, err := cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Empty(t, got, "fresh negative row must be honoured")
	assert.Equal(t, 0, inner.calls, "inner must not be called when negative TTL is still valid")
}

// With negativeTTL=0 a negative row never expires, preserving legacy behaviour.
func TestCached_NegativeCacheTTLZero_ForeverHit(t *testing.T) {
	t.Parallel()
	episode, repoBackend := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repoBackend)
	cache := newFakeCacheRepo()
	// Seed an ancient negative row.
	cache.setEntry(1396, 1, 5, []byte("[]"), time.Now().Add(-365*24*time.Hour))

	inner := &fakeInner{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
	}}
	// TTL=0 is the default - explicit zero disables expiry.
	cached := NewCached(inner, resolver, cache).WithNegativeTTL(0)

	got, err := cached.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Empty(t, got, "TTL=0 must honour negative row regardless of age")
	assert.Equal(t, 0, inner.calls, "inner must never be called when negativeTTL=0")
}
