package postgres_test

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestArtwork(id, mediaItemID string, sortOrder int) *metadata.ArtworkRecord {
	src := "https://example.com/" + id + ".jpg"
	lang := "en"
	w, h := 1000, 1500
	return &metadata.ArtworkRecord{
		ID:          id,
		MediaItemID: mediaItemID,
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
