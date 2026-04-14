package postgres_test

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaPersonRepo_Create_GetByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mp-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("mp-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	person := &library.Person{ID: "mp-p1", Name: "Actor One", ProviderIDs: map[string]string{}}
	require.NoError(t, personRepo.Create(ctx, person))

	charName := "Hero"
	mp := &library.MediaPerson{
		MediaItemID:   string(item.ID),
		PersonID:      person.ID,
		RoleType:      "actor",
		CharacterName: &charName,
		SortOrder:     0,
	}
	require.NoError(t, mpRepo.Create(ctx, mp))

	list, err := mpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "actor", list[0].RoleType)
	assert.Equal(t, person.ID, list[0].PersonID)
}

func TestMediaPersonRepo_DeleteByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mp-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("mp-mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	person := &library.Person{ID: "mp-p2", Name: "Actor Two", ProviderIDs: map[string]string{}}
	require.NoError(t, personRepo.Create(ctx, person))

	mp := &library.MediaPerson{
		MediaItemID: string(item.ID),
		PersonID:    person.ID,
		RoleType:    "director",
		SortOrder:   0,
	}
	require.NoError(t, mpRepo.Create(ctx, mp))
	require.NoError(t, mpRepo.DeleteByMediaItemID(ctx, item.ID))

	list, err := mpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}
