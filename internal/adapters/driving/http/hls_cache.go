package httpapi

import (
	"context"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
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

// CachedLibraryLister caches the library list for hlsCacheTTL. Direct play
// and trickplay call it on every Range request only to confine the served
// path to the library roots, which change on the order of days.
type CachedLibraryLister struct {
	inner  LibraryLister
	c      *cache.TTLCache[[]*domainlibrary.Library]
	stopFn func()
}

func NewCachedLibraryLister(inner LibraryLister) *CachedLibraryLister {
	c, stop := cache.New[[]*domainlibrary.Library](60 * time.Second)
	return &CachedLibraryLister{inner: inner, c: c, stopFn: stop}
}

func (ch *CachedLibraryLister) Close() { ch.stopFn() }

// ListLibraries returns the cached list; the slice is shared between callers
// and must be treated as read-only.
func (ch *CachedLibraryLister) ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error) {
	const key = "libraries"
	if v, ok := ch.c.Get(key); ok {
		return v, nil
	}
	libs, err := ch.inner.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	ch.c.Set(key, libs, hlsCacheTTL)
	return libs, nil
}
