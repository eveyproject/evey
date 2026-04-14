package httpapi

import (
	"context"
	"fmt"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/cache"
)

const hlsCacheTTL = 30 * time.Second

// CachedMediaItemGetter wraps a MediaItemGetter with a short-TTL in-memory cache.
type CachedMediaItemGetter struct {
	inner   MediaItemGetter
	c       *cache.TTLCache[*domainlibrary.MediaItem]
	stopFn  func()
}

func NewCachedMediaItemGetter(inner MediaItemGetter) *CachedMediaItemGetter {
	c, stop := cache.New[*domainlibrary.MediaItem](60 * time.Second)
	return &CachedMediaItemGetter{inner: inner, c: c, stopFn: stop}
}

func (g *CachedMediaItemGetter) Close() { g.stopFn() }

func (g *CachedMediaItemGetter) GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error) {
	if v, ok := g.c.Get("media:" + id); ok {
		return v, nil
	}
	item, err := g.inner.GetMediaItem(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("CachedMediaItemGetter.GetMediaItem: %w", err)
	}
	g.c.Set("media:"+id, item, hlsCacheTTL)
	return item, nil
}

// CachedLibraryAccessChecker wraps a LibraryAccessChecker with a short-TTL in-memory cache.
type CachedLibraryAccessChecker struct {
	inner  LibraryAccessChecker
	c      *cache.TTLCache[bool]
	stopFn func()
}

func NewCachedLibraryAccessChecker(inner LibraryAccessChecker) *CachedLibraryAccessChecker {
	c, stop := cache.New[bool](60 * time.Second)
	return &CachedLibraryAccessChecker{inner: inner, c: c, stopFn: stop}
}

func (ch *CachedLibraryAccessChecker) Close() { ch.stopFn() }

func (ch *CachedLibraryAccessChecker) HasAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) (bool, error) {
	key := "access:" + string(userID) + ":" + string(libraryID)
	if v, ok := ch.c.Get(key); ok {
		return v, nil
	}
	ok, err := ch.inner.HasAccess(ctx, userID, libraryID)
	if err != nil {
		return false, err
	}
	ch.c.Set(key, ok, hlsCacheTTL)
	return ok, nil
}
