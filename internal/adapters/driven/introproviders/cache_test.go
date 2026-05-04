package introproviders

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

type fakeCacheRepo struct {
	store map[string][]byte
	gets  int
	puts  int
}

func newFakeCacheRepo() *fakeCacheRepo {
	return &fakeCacheRepo{store: map[string][]byte{}}
}

func (r *fakeCacheRepo) key(tmdbID, season, episode int) string {
	return string(rune(tmdbID)) + ":" + string(rune(season)) + ":" + string(rune(episode))
}

func (r *fakeCacheRepo) Get(_ context.Context, tmdbID, season, episode int) ([]byte, error) {
	r.gets++
	return r.store[r.key(tmdbID, season, episode)], nil
}

func (r *fakeCacheRepo) Put(_ context.Context, tmdbID, season, episode int, _ string, payload []byte) error {
	r.puts++
	r.store[r.key(tmdbID, season, episode)] = payload
	return nil
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

	// Second call should not re-invoke inner — even empty results were cached.
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
	cache.store[cache.key(1396, 1, 5)] = []byte("not-json")

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
	require.NoError(t, json.Unmarshal(cache.store[cache.key(1396, 1, 5)], &parsed))
	assert.Len(t, parsed, 1)
}
