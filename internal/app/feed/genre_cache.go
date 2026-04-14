package feed

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
)

type genreCacheEntry struct {
	genres    []string
	fetchedAt time.Time
}

// CachedAnalyzer wraps a WatchHistoryAnalyzer with TTL cache on GetFavoriteGenres.
type CachedAnalyzer struct {
	inner notification.WatchHistoryAnalyzer
	ttl   time.Duration
	mu    sync.RWMutex
	cache map[string]genreCacheEntry // key: profileID:minCount
}

type CachedAnalyzerOption func(*CachedAnalyzer)

func CacheWithEventBus(bus shared.EventBus) CachedAnalyzerOption {
	return func(c *CachedAnalyzer) {
		invalidate := func(_ context.Context, _ shared.Event) {
			c.Invalidate()
			slog.Debug("genre cache invalidated")
		}
		bus.Subscribe("media.item.added", invalidate)
		bus.Subscribe("media.item.updated", invalidate)
		bus.Subscribe("media.item.removed", invalidate)
		bus.Subscribe("library.scan.completed", invalidate)
	}
}

func NewCachedAnalyzer(inner notification.WatchHistoryAnalyzer, ttl time.Duration, opts ...CachedAnalyzerOption) *CachedAnalyzer {
	c := &CachedAnalyzer{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]genreCacheEntry),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *CachedAnalyzer) cacheKey(profileID string, minCount int) string {
	return profileID + ":" + itoa(minCount)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func (c *CachedAnalyzer) GetFavoriteGenres(ctx context.Context, profileID string, minCount int) ([]string, error) {
	key := c.cacheKey(profileID, minCount)

	c.mu.RLock()
	if entry, ok := c.cache[key]; ok && time.Since(entry.fetchedAt) < c.ttl {
		result := make([]string, len(entry.genres))
		copy(result, entry.genres)
		c.mu.RUnlock()
		return result, nil
	}
	c.mu.RUnlock()

	genres, err := c.inner.GetFavoriteGenres(ctx, profileID, minCount)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[key] = genreCacheEntry{genres: genres, fetchedAt: time.Now()}
	c.mu.Unlock()

	return genres, nil
}

func (c *CachedAnalyzer) GetFavoriteGenresBatch(ctx context.Context, profileIDs []string, minCount int) (map[string][]string, error) {
	return c.inner.GetFavoriteGenresBatch(ctx, profileIDs, minCount)
}

func (c *CachedAnalyzer) HasWatchedSeries(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	return c.inner.HasWatchedSeries(ctx, profileID, mediaItemID)
}

func (c *CachedAnalyzer) IsCurrentlyWatchingSeries(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	return c.inner.IsCurrentlyWatchingSeries(ctx, profileID, mediaItemID)
}

func (c *CachedAnalyzer) CalculateRelevanceScore(ctx context.Context, profileID string, genres []string) (int, error) {
	return c.inner.CalculateRelevanceScore(ctx, profileID, genres)
}

// Invalidate clears all cached genre data (call when media is added/modified).
func (c *CachedAnalyzer) Invalidate() {
	c.mu.Lock()
	c.cache = make(map[string]genreCacheEntry)
	c.mu.Unlock()
}

var _ notification.WatchHistoryAnalyzer = (*CachedAnalyzer)(nil)
