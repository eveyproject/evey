package library_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
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

	byIDs, err := cache.GetByIDs(context.Background(), []shared.MediaItemID{"m1", "m2"})
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

func TestCachedMediaItemQueryRepo_Stop(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)
	cache.Stop()
	// calling Stop twice is safe
	cache.Stop()
}

func TestCachedMediaItemQueryRepo_Register_InvalidatesOnRemoved(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "Original"})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)
	defer cache.Stop()

	bus := sharedfakes.NewFakeEventBus()
	cache.Register(bus)

	// populate cache
	item, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Original", item.Title)

	// update inner + publish removal event
	inner.Seed(&library.MediaItem{ID: "m1", Title: "Updated"})
	bus.Publish(context.Background(), library.MediaItemRemoved{MediaItemID: "m1"})

	item, err = cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "Updated", item.Title)
}

func TestCachedMediaItemQueryRepo_GetCastByMediaItem(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.SeedCast("m1", []library.CastMember{
		{PersonID: "p1", PersonName: "Alice", RoleType: "actor"},
	})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)
	defer cache.Stop()

	cast, err := cache.GetCastByMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.Len(t, cast, 1)
	assert.Equal(t, "Alice", cast[0].PersonName)
}

func TestCachedMediaItemQueryRepo_GetCastForItems(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.SeedCast("m1", []library.CastMember{{PersonID: "p1", PersonName: "Alice"}})
	inner.SeedCast("m2", []library.CastMember{{PersonID: "p2", PersonName: "Bob"}})
	cache := applib.NewCachedMediaItemQueryRepo(inner, 5*time.Second)
	defer cache.Stop()

	castMap, err := cache.GetCastForItems(context.Background(), []shared.MediaItemID{"m1", "m2"})
	require.NoError(t, err)
	assert.Len(t, castMap, 2)
}

func TestCachedMediaItemQueryRepo_EvictLoop_RemovesExpired(t *testing.T) {
	t.Parallel()
	inner := libfakes.NewFakeMediaItemQueryRepo()
	inner.Seed(&library.MediaItem{ID: "m1", Title: "V1"})
	// very short TTL so evict loop cleans it up
	cache := applib.NewCachedMediaItemQueryRepo(inner, 1*time.Millisecond)

	_, _ = cache.GetByID(context.Background(), "m1")

	// wait for eviction (loop runs every 10s, but entry expires immediately)
	// we can verify by checking that after TTL expiry, re-fetch hits inner
	time.Sleep(50 * time.Millisecond)
	inner.Seed(&library.MediaItem{ID: "m1", Title: "V2"})

	item, err := cache.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "V2", item.Title)
	cache.Stop()
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

// countingGenreRepo counts how often the expensive query is really issued.
type countingGenreRepo struct {
	library.MediaItemQueryRepo
	calls  atomic.Int32
	genres []string
}

func (r *countingGenreRepo) ListGenres(context.Context, shared.LibraryID) ([]string, error) {
	r.calls.Add(1)
	return append([]string(nil), r.genres...), nil
}

// The genre list is O(library) to compute - it unnests every item's genres and
// DISTINCTs them, which no index can serve - and it is fetched every time a
// user opens the genre filter. Uncached, the cheapest interaction in the UI
// was one of the most expensive queries on the server.
func TestCachedMediaItemQueryRepo_ListGenresIsCachedPerLibrary(t *testing.T) {
	inner := &countingGenreRepo{genres: []string{"Drama", "Sci-Fi"}}
	c := applib.NewCachedMediaItemQueryRepo(inner, time.Minute)
	defer c.Stop()
	ctx := context.Background()

	first, err := c.ListGenres(ctx, "lib-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range 5 {
		if _, e := c.ListGenres(ctx, "lib-1"); e != nil {
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if got := inner.calls.Load(); got != 1 {
		t.Errorf("the underlying query ran %d times for 6 calls, want 1", got)
	}

	// A different library is a different answer and must not be served the
	// first one's cache.
	if _, err := c.ListGenres(ctx, "lib-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := inner.calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2 (one per library)", got)
	}

	// The cached slice must be a copy: a caller that sorts or appends must not
	// corrupt what everyone else reads.
	first[0] = "MUTATED"
	again, againErr := c.ListGenres(ctx, "lib-1")
	if againErr != nil {
		t.Fatalf("unexpected error: %v", againErr)
	}
	if again[0] == "MUTATED" {
		t.Error("a caller mutated the cached genre list for every other caller")
	}
}
