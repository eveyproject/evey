package httpapi

import (
	"context"
	"errors"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// countingMediaItemGetter counts calls to GetMediaItem.
type countingMediaItemGetter struct {
	item  *domainlibrary.MediaItem
	err   error
	calls int
}

func (c *countingMediaItemGetter) GetMediaItem(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	c.calls++
	return c.item, c.err
}

// countingAccessChecker counts calls to HasAccess.
type countingAccessChecker struct {
	allowed bool
	err     error
	calls   int
}

func (c *countingAccessChecker) HasAccess(_ context.Context, _ shared.UserID, _ shared.LibraryID) (bool, error) {
	c.calls++
	return c.allowed, c.err
}

func TestHLSCacheMediaItemGetter_CachesResult(t *testing.T) {
	inner := &countingMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	g := NewCachedMediaItemGetter(inner)
	defer g.Close()

	for i := 0; i < 5; i++ {
		item, err := g.GetMediaItem(context.Background(), "m1")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if item.ID != "m1" {
			t.Fatalf("want m1 got %v", item.ID)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 inner call, got %d", inner.calls)
	}
}

func TestHLSCacheMediaItemGetter_DistinctKeys(t *testing.T) {
	inner := &countingMediaItemGetter{item: &domainlibrary.MediaItem{ID: "x", LibraryID: "lib1"}}
	g := NewCachedMediaItemGetter(inner)
	defer g.Close()

	_, _ = g.GetMediaItem(context.Background(), "a")
	_, _ = g.GetMediaItem(context.Background(), "b")
	_, _ = g.GetMediaItem(context.Background(), "a")
	if inner.calls != 2 {
		t.Fatalf("expected 2 inner calls, got %d", inner.calls)
	}
}

func TestHLSCacheMediaItemGetter_ErrorNotCached(t *testing.T) {
	inner := &countingMediaItemGetter{err: errors.New("boom")}
	g := NewCachedMediaItemGetter(inner)
	defer g.Close()

	if _, err := g.GetMediaItem(context.Background(), "m1"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := g.GetMediaItem(context.Background(), "m1"); err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 2 {
		t.Fatalf("errors should not be cached, got %d calls", inner.calls)
	}
}

func TestHLSCacheAccessChecker_CachesResult(t *testing.T) {
	inner := &countingAccessChecker{allowed: true}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	for i := 0; i < 3; i++ {
		ok, err := ch.HasAccess(context.Background(), shared.UserID("u1"), shared.LibraryID("lib1"))
		if err != nil || !ok {
			t.Fatalf("want allowed, got ok=%v err=%v", ok, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 inner call, got %d", inner.calls)
	}
}

func TestHLSCacheAccessChecker_KeyedByUserAndLibrary(t *testing.T) {
	inner := &countingAccessChecker{allowed: true}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	_, _ = ch.HasAccess(context.Background(), "u1", "lib1")
	_, _ = ch.HasAccess(context.Background(), "u2", "lib1")
	_, _ = ch.HasAccess(context.Background(), "u1", "lib2")
	_, _ = ch.HasAccess(context.Background(), "u1", "lib1")
	if inner.calls != 3 {
		t.Fatalf("expected 3 distinct inner calls, got %d", inner.calls)
	}
}

func TestHLSCacheAccessChecker_ErrorNotCached(t *testing.T) {
	inner := &countingAccessChecker{err: errors.New("db down")}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	if _, err := ch.HasAccess(context.Background(), "u1", "lib1"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ch.HasAccess(context.Background(), "u1", "lib1"); err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 2 {
		t.Fatalf("errors should not be cached, got %d calls", inner.calls)
	}
}
