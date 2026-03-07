package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMediaItem(id, libraryID string) *library.MediaItem {
	return &library.MediaItem{
		ID:          id,
		LibraryID:   libraryID,
		Type:        string(library.MovieItem),
		Title:       "Movie " + id,
		SortTitle:   "movie " + id,
		ProviderIDs: map[string]string{"tmdb": "123"},
		Genres:      []string{"Action"},
		Studios:     []string{"Studio A"},
		Tags:        []string{},
		AddedAt:     time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestMediaItemRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, item.Title, got.Title)
	assert.Equal(t, item.LibraryID, got.LibraryID)
	assert.Equal(t, item.Genres, got.Genres)
}

func TestMediaItemRepo_GetByFilePath(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi2", lib.ID)
	item.FilePath = "/media/movie.mkv"
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByFilePath(ctx, "/media/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib3")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi3a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi3b", lib.ID)))

	libID := lib.ID
	list, err := itemRepo.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestMediaItemRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib4")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	item.Title = "Updated Movie"
	item.Genres = []string{"Drama"}
	item.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, itemRepo.Update(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Movie", got.Title)
	assert.Equal(t, []string{"Drama"}, got.Genres)
}

func TestMediaItemRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib5")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, itemRepo.Delete(ctx, item.ID))

	_, err := itemRepo.GetByID(ctx, item.ID)
	assert.Error(t, err)
}
