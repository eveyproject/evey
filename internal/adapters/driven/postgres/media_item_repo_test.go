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
		Type:        library.MovieItem,
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

func TestMediaItemRepo_GetByFileFingerprint(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib6")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi6", lib.ID)
	item.FilePath = "/media/fp.mkv"
	item.FileFingerprint = "fp-unique-123"
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByFileFingerprint(ctx, "fp-unique-123")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_GetByLibraryTypeTitle(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib7")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi7", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByLibraryTypeTitle(ctx, lib.ID, string(library.MovieItem), item.Title, "")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_DeleteByIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib8")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi8a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi8b", lib.ID)))
	require.NoError(t, itemRepo.DeleteByIDs(ctx, []string{"mi8a", "mi8b"}))

	_, err := itemRepo.GetByID(ctx, "mi8a")
	assert.Error(t, err)
}

func TestMediaItemRepo_DeleteByIDs_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	require.NoError(t, itemRepo.DeleteByIDs(ctx, []string{}))
}

func TestMediaItemRepo_ListFilePaths(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib9")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi9", lib.ID)
	item.FilePath = "/media/test.mkv"
	require.NoError(t, itemRepo.Create(ctx, item))

	paths, err := itemRepo.ListFilePaths(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "/media/test.mkv", paths["mi9"])
}

func TestMediaItemRepo_ListByLibraryID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib10")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi10a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi10b", lib.ID)))

	items, err := itemRepo.ListByLibraryID(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestMediaItemRepo_ListScanSummaries(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib11")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi11", lib.ID)
	item.FilePath = "/media/scan.mkv"
	item.FileFingerprint = "scan-fp"
	require.NoError(t, itemRepo.Create(ctx, item))

	summaries, err := itemRepo.ListScanSummaries(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "mi11", summaries[0].ID)
	assert.Equal(t, "/media/scan.mkv", summaries[0].FilePath)
}

func TestMediaItemRepo_ListContainerInfo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib12")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi12", lib.ID)))

	rows, err := itemRepo.ListContainerInfo(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, "mi12", rows[0].ID)
}
