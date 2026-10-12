package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestArtwork(id, mediaItemID string, sortOrder int) *metadata.ArtworkRecord {
	src := "https://example.com/" + id + ".jpg"
	lang := "en"
	w, h := 1000, 1500
	return &metadata.ArtworkRecord{
		ID:          id,
		MediaItemID: shared.MediaItemID(mediaItemID),
		Type:        "poster",
		Provider:    "tmdb",
		SourceURL:   &src,
		Width:       &w,
		Height:      &h,
		Language:    &lang,
		SortOrder:   sortOrder,
	}
}

func TestArtworkRepo_Create_GetByMediaItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("artlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("artmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	a1 := newTestArtwork("art1a", string(item.ID), 1)
	a2 := newTestArtwork("art1b", string(item.ID), 0)
	require.NoError(t, artRepo.Create(ctx, a1))
	require.NoError(t, artRepo.Create(ctx, a2))

	got, err := artRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "art1b", got[0].ID)
	assert.Equal(t, 0, got[0].SortOrder)
	assert.Equal(t, "art1a", got[1].ID)
	assert.Equal(t, 1, got[1].SortOrder)
}

func TestArtworkRepo_GetByPerson(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("artlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("artmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	person := &library.Person{ID: "person1", Name: "Test Actor", ProviderIDs: map[string]string{}}
	require.NoError(t, personRepo.Create(ctx, person))

	personID := person.ID
	a := newTestArtwork("art2a", string(item.ID), 0)
	a.PersonID = &personID
	require.NoError(t, artRepo.Create(ctx, a))

	got, err := artRepo.GetByPerson(ctx, person.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "art2a", got[0].ID)
	require.NotNil(t, got[0].PersonID)
	assert.Equal(t, person.ID, *got[0].PersonID)
}

func TestArtworkRepo_UpdateSortOrder(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("artlib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("artmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	a := newTestArtwork("art3a", string(item.ID), 0)
	require.NoError(t, artRepo.Create(ctx, a))

	require.NoError(t, artRepo.UpdateSortOrder(ctx, "art3a", 5))

	got, err := artRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 5, got[0].SortOrder)
}

func TestArtworkRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("artlib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("artmi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	a := newTestArtwork("art4a", string(item.ID), 0)
	require.NoError(t, artRepo.Create(ctx, a))

	require.NoError(t, artRepo.Delete(ctx, "art4a"))

	got, err := artRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestArtworkRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewArtworkRepository(db)
	err := repo.Delete(context.Background(), "art-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestArtworkRepo_UpdateSortOrder_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewArtworkRepository(db)
	err := repo.UpdateSortOrder(context.Background(), "art-missing", 1)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestArtworkRepo_UpdateWebPPath_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewArtworkRepository(db)
	err := repo.UpdateWebPPath(context.Background(), "art-missing", "/tmp/x.webp")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestArtworkRepo_DeleteByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("artlib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("artmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, artRepo.Create(ctx, newTestArtwork("art5a", string(item.ID), 0)))
	require.NoError(t, artRepo.Create(ctx, newTestArtwork("art5b", string(item.ID), 1)))

	require.NoError(t, artRepo.DeleteByMediaItemID(ctx, string(item.ID)))

	got, err := artRepo.GetByMediaItem(ctx, string(item.ID))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestArtworkRepo_DeleteByMediaItemIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	artRepo := postgres.NewArtworkRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("artlib6")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("artmi6a", lib.ID)
	item2 := newTestMediaItem("artmi6b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	require.NoError(t, artRepo.Create(ctx, newTestArtwork("art6a", string(item1.ID), 0)))
	require.NoError(t, artRepo.Create(ctx, newTestArtwork("art6b", string(item2.ID), 0)))

	require.NoError(t, artRepo.DeleteByMediaItemIDs(ctx, []string{string(item1.ID), string(item2.ID)}))

	got1, err := artRepo.GetByMediaItem(ctx, string(item1.ID))
	require.NoError(t, err)
	assert.Empty(t, got1)
	got2, err := artRepo.GetByMediaItem(ctx, string(item2.ID))
	require.NoError(t, err)
	assert.Empty(t, got2)
}

func TestArtworkRepo_DeleteByMediaItemIDs_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	artRepo := postgres.NewArtworkRepository(db)
	assert.NoError(t, artRepo.DeleteByMediaItemIDs(context.Background(), nil))
}

// The artwork maintenance loops: the download backlog is "has a source, no
// local file", the blurhash backlog is "has a local file, no blurhash", the
// stats count both sides, and the batched read keys records by item.
func TestArtworkRepo_BacklogsStatsAndBatchRead(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("artlib9")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	a, b := newTestMediaItem("artmi9a", lib.ID), newTestMediaItem("artmi9b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, a))
	require.NoError(t, itemRepo.Create(ctx, b))
	artRepo := postgres.NewArtworkRepository(db)

	pending := newTestArtwork("art9-pending", string(a.ID), 0) // source, no local file
	local := newTestArtwork("art9-local", string(a.ID), 1)     // downloaded, no blurhash yet
	done := newTestArtwork("art9-done", string(b.ID), 0)       // downloaded and hashed
	for _, rec := range []*metadata.ArtworkRecord{pending, local, done} {
		require.NoError(t, artRepo.Create(ctx, rec))
	}
	require.NoError(t, artRepo.UpdateLocalPath(ctx, "art9-local", "/data/artworks/a/poster.jpg"))
	require.NoError(t, artRepo.UpdateLocalPath(ctx, "art9-done", "/data/artworks/b/poster.jpg"))
	require.NoError(t, artRepo.UpdateBlurhash(ctx, "art9-done", "LEHV6nWB2yk8"))
	require.Error(t, artRepo.UpdateLocalPath(ctx, "ghost", "/x"), "a vanished record is not a silent success")
	require.Error(t, artRepo.UpdateBlurhash(ctx, "ghost", "x"))

	missingLocal, err := artRepo.ListMissingLocalPath(ctx)
	require.NoError(t, err)
	require.Len(t, missingLocal, 1)
	assert.Equal(t, "art9-pending", missingLocal[0].ID)

	missingHash, err := artRepo.ListMissingBlurhash(ctx)
	require.NoError(t, err)
	require.Len(t, missingHash, 1)
	assert.Equal(t, "art9-local", missingHash[0].ID, "only a downloaded file can be hashed")

	stats, err := artRepo.LocalPathStats(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.TotalRecords)
	assert.EqualValues(t, 2, stats.WithLocalPath)
	assert.EqualValues(t, 1, stats.MissingLocalPath)

	byItem, err := artRepo.GetByMediaItems(ctx, []string{string(a.ID), string(b.ID), "ghost"})
	require.NoError(t, err)
	require.Len(t, byItem[string(a.ID)], 2)
	assert.Equal(t, "art9-pending", byItem[string(a.ID)][0].ID, "sort order within an item")
	require.Len(t, byItem[string(b.ID)], 1)
	require.NotNil(t, byItem[string(b.ID)][0].Blurhash)
	assert.Equal(t, "LEHV6nWB2yk8", *byItem[string(b.ID)][0].Blurhash)
	assert.NotContains(t, byItem, "ghost")
	none, err := artRepo.GetByMediaItems(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
