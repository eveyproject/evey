package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedComplexityItem inserts one media item with a file of the given type and
// duration. Type and duration are what ListUnclassified must discriminate on.
func seedComplexityItem(t *testing.T, itemRepo *postgres.MediaItemRepository, libID shared.LibraryID, id string, typ library.MediaItemType, duration time.Duration) {
	t.Helper()
	item := newTestMediaItem(id, libID)
	item.Type = typ
	d, err := shared.NewDurationFromTime(duration)
	require.NoError(t, err)
	item.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Duration: d}
	require.NoError(t, itemRepo.Create(context.Background(), item))
}

func TestComplexityRepo_ListUnclassified_OnlyClassifiableItems(t *testing.T) {
	// The classifier samples VIDEO frames. Tracks and photos can never classify,
	// and a zero-duration file gives the sampler nothing to spread over: the
	// worker skips all three WITHOUT persisting, so serving them as candidates
	// re-queues the same IDs forever (observed in prod 2026-07-26: ~24 silent
	// attempts/min around the clock on a library with a music section).
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaComplexityRepo(db)
	ctx := context.Background()

	lib := newTestLibrary("cx-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	seedComplexityItem(t, itemRepo, lib.ID, "cx-movie", library.MovieItem, 90*time.Minute)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-episode", library.Episode, 40*time.Minute)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-track", library.Track, 3*time.Minute)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-photo", library.PhotoItem, 0)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-noduration", library.MovieItem, 0)

	ids, err := repo.ListUnclassified(ctx, 50)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"cx-movie", "cx-episode"}, ids,
		"only video items with a known duration are classifiable candidates")
}
