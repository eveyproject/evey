package library

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type cacheEntry struct {
	item      *library.MediaItem
	expiresAt time.Time
}

type CachedMediaItemQueryRepo struct {
	inner library.MediaItemQueryRepo
	ttl   time.Duration

	mu    sync.RWMutex
	items map[shared.MediaItemID]cacheEntry
	done  chan struct{}
}

func NewCachedMediaItemQueryRepo(inner library.MediaItemQueryRepo, ttl time.Duration) *CachedMediaItemQueryRepo {
	c := &CachedMediaItemQueryRepo{
		inner: inner,
		ttl:   ttl,
		items: make(map[shared.MediaItemID]cacheEntry),
		done:  make(chan struct{}),
	}
	go c.evictLoop()
	return c
}

func (c *CachedMediaItemQueryRepo) Stop() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (c *CachedMediaItemQueryRepo) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.CachedMediaItemQueryRepo.GetByID")
	defer span.End()
	c.mu.RLock()
	if e, ok := c.items[id]; ok && time.Now().Before(e.expiresAt) {
		c.mu.RUnlock()
		return e.item, nil
	}
	c.mu.RUnlock()

	item, err := c.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.items[id] = cacheEntry{item: item, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return item, nil
}

func (c *CachedMediaItemQueryRepo) Invalidate(id shared.MediaItemID) {
	c.mu.Lock()
	delete(c.items, id)
	c.mu.Unlock()
}

func (c *CachedMediaItemQueryRepo) Register(bus shared.EventBus) {
	bus.Subscribe("media.item.removed", func(_ context.Context, ev shared.Event) {
		if e, ok := ev.(library.MediaItemRemoved); ok {
			c.Invalidate(shared.MediaItemID(e.MediaItemID))
		}
	})
	bus.Subscribe("media.item.updated", func(_ context.Context, ev shared.Event) {
		if e, ok := ev.(library.MediaItemUpdated); ok {
			c.Invalidate(shared.MediaItemID(e.MediaItemID))
		}
	})
}

func (c *CachedMediaItemQueryRepo) GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error) {
	return c.inner.GetByIDs(ctx, ids)
}

// GetByFilePath is not cached by path: it's only hit on the rare MKV chapter
// skip-intro fallback (once per playback, staleTime:Infinity client-side), so a
// per-path cache would add invalidation surface for no real benefit.
func (c *CachedMediaItemQueryRepo) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	return c.inner.GetByFilePath(ctx, path)
}

func (c *CachedMediaItemQueryRepo) List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return c.inner.List(ctx, filter)
}

func (c *CachedMediaItemQueryRepo) Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	return c.inner.Count(ctx, filter)
}

func (c *CachedMediaItemQueryRepo) GetCastByMediaItem(ctx context.Context, mediaItemID shared.MediaItemID) ([]library.CastMember, error) {
	return c.inner.GetCastByMediaItem(ctx, mediaItemID)
}

func (c *CachedMediaItemQueryRepo) GetCastForItems(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]library.CastMember, error) {
	return c.inner.GetCastForItems(ctx, mediaItemIDs)
}

func (c *CachedMediaItemQueryRepo) SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	return c.inner.SeriesAggregate(ctx, seriesID)
}

func (c *CachedMediaItemQueryRepo) ListGenres(ctx context.Context, libraryID shared.LibraryID) ([]string, error) {
	return c.inner.ListGenres(ctx, libraryID)
}

func (c *CachedMediaItemQueryRepo) evictLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for id, e := range c.items {
				if now.After(e.expiresAt) {
					delete(c.items, id)
				}
			}
			c.mu.Unlock()
		}
	}
}
