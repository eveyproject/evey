package library_test

import (
	"context"
	"testing"
	"time"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedMediaItemQueryRepo_GetByID_CachesResult(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "Cached Film"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)

	item1, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Cached Film", item1.Title)

	inner.Seed(&library.MediaItem{ID: "m1", Title: "Updated Film"})

	item2, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Cached Film", item2.Title, "should return cached value")
}

func TestCachedMediaItemQueryRepo_Invalidate(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "Original"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)

	_, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)

	inner.Seed(&library.MediaItem{ID: "m1", Title: "Updated"})
	cache.Invalidate("m1")

	item, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Updated", item.Title, "should fetch fresh after invalidation")
}

func TestCachedMediaItemQueryRepo_TTLExpiry(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "Original"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 50*time.Millisecond)

	_, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)

	inner.Seed(&library.MediaItem{ID: "m1", Title: "Updated"})
	time.Sleep(100 * time.Millisecond)

	item, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Updated", item.Title, "should fetch fresh after TTL expiry")
}

func TestCachedMediaItemQueryRepo_PassthroughMethods(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "A", Type: "movie"})
	inner.Seed(&library.MediaItem{ID: "m2", Title: "B", Type: "movie"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)

	items, err := cache.List(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Len(t, items, 2)

	count, err := cache.Count(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	byIDs, err := cache.GetByIDs(context.Background(), []string{"m1", "m2"})
	require.NoError(t, err)
	assert.Len(t, byIDs, 2)
}

func TestCachedMediaItemQueryRepo_MissReturnsError(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)

	_, err := cache.GetByID(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestCachedMediaItemQueryRepo_ZeroTTL_NeverCaches(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "V1"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 0)

	_, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)

	inner.Seed(&library.MediaItem{ID: "m1", Title: "V2"})
	time.Sleep(time.Millisecond)

	item, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "V2", item.Title, "zero TTL should always miss cache")
}

func TestCachedMediaItemQueryRepo_ShortTTL_CacheHitBeforeExpiry(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "Original"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 2*time.Second)

	_, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)

	inner.Seed(&library.MediaItem{ID: "m1", Title: "Updated"})

	item, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Original", item.Title, "should still return cached value before TTL")
}
