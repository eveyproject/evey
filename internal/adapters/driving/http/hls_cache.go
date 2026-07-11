package httpapi

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/cache"
)

const hlsCacheTTL = 30 * time.Second

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
