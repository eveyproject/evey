package feed_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appfeed "github.com/eveyproject/evey/internal/app/feed"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_700_000_000, 0)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func TestCachedAnalyzer_ReturnsCachedGenres(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"action", "comedy"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)

	ctx := context.Background()
	g1, err := cached.GetFavoriteGenres(ctx, "p1", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"action", "comedy"}, g1)

	fake.Genres = []string{"drama"}
	g2, err := cached.GetFavoriteGenres(ctx, "p1", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"action", "comedy"}, g2, "should return cached value")
}

func TestCachedAnalyzer_DifferentKeys(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"action"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)

	ctx := context.Background()
	_, _ = cached.GetFavoriteGenres(ctx, "p1", 3)

	fake.Genres = []string{"horror"}
	g2, err := cached.GetFavoriteGenres(ctx, "p2", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"horror"}, g2, "different profile should not use cache")
}

func TestCachedAnalyzer_Invalidate(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"action"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)

	ctx := context.Background()
	_, _ = cached.GetFavoriteGenres(ctx, "p1", 3)

	fake.Genres = []string{"drama"}
	cached.Invalidate()

	g, err := cached.GetFavoriteGenres(ctx, "p1", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"drama"}, g, "after invalidation should fetch fresh")
}

func TestCachedAnalyzer_TTLExpiry(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"action"}
	clk := newFakeClock()
	cached := appfeed.NewCachedAnalyzer(fake, 50*time.Millisecond, appfeed.CacheWithClock(clk))

	ctx := context.Background()
	_, _ = cached.GetFavoriteGenres(ctx, "p1", 3)

	fake.Genres = []string{"drama"}
	clk.Advance(60 * time.Millisecond)

	g, err := cached.GetFavoriteGenres(ctx, "p1", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"drama"}, g, "expired cache should fetch fresh")
}

func TestCachedAnalyzer_PassthroughMethods(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Watched = true
	fake.Watching = true
	fake.Relevance = 5
	fake.Genres = []string{"action"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)

	ctx := context.Background()

	watched, err := cached.HasWatchedSeries(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, watched)

	watching, err := cached.IsCurrentlyWatchingSeries(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, watching)

	score, err := cached.CalculateRelevanceScore(ctx, "p1", []string{"action"})
	require.NoError(t, err)
	assert.Equal(t, 5, score)

	batch, err := cached.GetFavoriteGenresBatch(ctx, []shared.ProfileID{"p1", "p2"}, 3)
	require.NoError(t, err)
	assert.Len(t, batch, 2)
}

type countingAnalyzer struct {
	*notiffakes.FakeWatchHistoryAnalyzer
	calls atomic.Int32
}

func (c *countingAnalyzer) GetFavoriteGenres(ctx context.Context, profileID shared.ProfileID, minCount int) ([]string, error) {
	c.calls.Add(1)
	return c.FakeWatchHistoryAnalyzer.GetFavoriteGenres(ctx, profileID, minCount)
}

func TestCachedAnalyzer_ReducesDBCalls(t *testing.T) {
	fake := &countingAnalyzer{FakeWatchHistoryAnalyzer: notiffakes.NewFakeWatchHistoryAnalyzer()}
	fake.Genres = []string{"action"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)

	ctx := context.Background()
	for range 10 {
		_, err := cached.GetFavoriteGenres(ctx, "p1", 3)
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), fake.calls.Load(), "should only call DB once for repeated requests")
}
