package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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
		MediaItemID:   item.ID,
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
		MediaItemID: item.ID,
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

func TestMediaPersonRepo_DeleteByMediaItemIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mp-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("mp-mi3a", lib.ID)
	item2 := newTestMediaItem("mp-mi3b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))
	person := &library.Person{ID: "mp-p3", Name: "Actor Three", ProviderIDs: map[string]string{}}
	require.NoError(t, personRepo.Create(ctx, person))

	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item1.ID, PersonID: person.ID, RoleType: "actor", SortOrder: 0}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item2.ID, PersonID: person.ID, RoleType: "director", SortOrder: 0}))

	require.NoError(t, mpRepo.DeleteByMediaItemIDs(ctx, []shared.MediaItemID{item1.ID, item2.ID}))

	list1, err := mpRepo.GetByMediaItemID(ctx, item1.ID)
	require.NoError(t, err)
	assert.Empty(t, list1)
	list2, err := mpRepo.GetByMediaItemID(ctx, item2.ID)
	require.NoError(t, err)
	assert.Empty(t, list2)
}

func TestMediaPersonRepo_DeleteByMediaItemIDs_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	mpRepo := postgres.NewMediaPersonRepository(db)
	assert.NoError(t, mpRepo.DeleteByMediaItemIDs(context.Background(), nil))
}

func TestMediaPersonRepo_CastNamesByMediaItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mp-lib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("mp-mi4a", lib.ID)
	item2 := newTestMediaItem("mp-mi4b", lib.ID)
	item3 := newTestMediaItem("mp-mi4c", lib.ID) // no cast at all
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))
	require.NoError(t, itemRepo.Create(ctx, item3))
	pa := &library.Person{ID: "mp-p4a", Name: "Cast Zeta", ProviderIDs: map[string]string{}}
	pb := &library.Person{ID: "mp-p4b", Name: "Cast Alpha", ProviderIDs: map[string]string{}}
	require.NoError(t, personRepo.Create(ctx, pa))
	require.NoError(t, personRepo.Create(ctx, pb))

	// item1: two people; pb also credited twice (actor + director) to prove the
	// per-item dedup. item2: one person. item3: none.
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item1.ID, PersonID: pa.ID, RoleType: "actor", SortOrder: 0}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item1.ID, PersonID: pb.ID, RoleType: "actor", SortOrder: 1}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item1.ID, PersonID: pb.ID, RoleType: "director", SortOrder: 2}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item2.ID, PersonID: pb.ID, RoleType: "actor", SortOrder: 0}))

	byID, err := mpRepo.CastNamesByMediaItems(ctx, []string{string(item1.ID), string(item2.ID), string(item3.ID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"Cast Zeta", "Cast Alpha"}, byID[string(item1.ID)], "sort_order kept, double credit deduped")
	assert.Equal(t, []string{"Cast Alpha"}, byID[string(item2.ID)])
	_, has3 := byID[string(item3.ID)]
	assert.False(t, has3, "an item with no cast has no entry")
}

func TestMediaPersonRepo_CastNamesByMediaItems_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	mpRepo := postgres.NewMediaPersonRepository(db)
	byID, err := mpRepo.CastNamesByMediaItems(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, byID)
}
