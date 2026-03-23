package library

import (
	"context"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type cacheEntry struct {
	item      *library.MediaItem
	expiresAt time.Time
}

type CachedMediaItemQueryRepo struct {
	inner library.MediaItemQueryRepo
	ttl   time.Duration

	mu    sync.RWMutex
	items map[string]cacheEntry
}

func NewCachedMediaItemQueryRepo(inner library.MediaItemQueryRepo, ttl time.Duration) *CachedMediaItemQueryRepo {
	c := &CachedMediaItemQueryRepo{
		inner: inner,
		ttl:   ttl,
		items: make(map[string]cacheEntry),
	}
	go c.evictLoop()
	return c
}

func (c *CachedMediaItemQueryRepo) GetByID(ctx context.Context, id string) (*library.MediaItem, error) {
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

func (c *CachedMediaItemQueryRepo) Invalidate(id string) {
	c.mu.Lock()
	delete(c.items, id)
	c.mu.Unlock()
}

func (c *CachedMediaItemQueryRepo) GetByIDs(ctx context.Context, ids []string) ([]*library.MediaItem, error) {
	return c.inner.GetByIDs(ctx, ids)
}

func (c *CachedMediaItemQueryRepo) List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return c.inner.List(ctx, filter)
}

func (c *CachedMediaItemQueryRepo) Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	return c.inner.Count(ctx, filter)
}

func (c *CachedMediaItemQueryRepo) GetCastByMediaItem(ctx context.Context, mediaItemID string) ([]library.CastMember, error) {
	return c.inner.GetCastByMediaItem(ctx, mediaItemID)
}

func (c *CachedMediaItemQueryRepo) GetCastForItems(ctx context.Context, mediaItemIDs []string) (map[string][]library.CastMember, error) {
	return c.inner.GetCastForItems(ctx, mediaItemIDs)
}

func (c *CachedMediaItemQueryRepo) evictLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
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
