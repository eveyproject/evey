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

func TestMediaItemQueryRepoAdapter_GetCastByMediaItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("cq-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	thumb := "/thumbs/actor1.jpg"
	p1 := &library.Person{ID: "cq-p1", Name: "Alice Doe", ProviderIDs: map[string]string{}, ThumbPath: &thumb}
	p2 := &library.Person{ID: "cq-p2", Name: "Bob Smith", ProviderIDs: map[string]string{}, ThumbPath: nil}
	require.NoError(t, personRepo.Create(ctx, p1))
	require.NoError(t, personRepo.Create(ctx, p2))

	char := "Hero"
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{
		MediaItemID:   item.ID,
		PersonID:      p1.ID,
		RoleType:      "actor",
		CharacterName: &char,
		SortOrder:     0,
	}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{
		MediaItemID:   item.ID,
		PersonID:      p2.ID,
		RoleType:      "director",
		CharacterName: nil,
		SortOrder:     1,
	}))

	cast, err := queryRepo.GetCastByMediaItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, cast, 2)

	assert.Equal(t, p1.ID, cast[0].PersonID)
	assert.Equal(t, "Alice Doe", cast[0].PersonName)
	assert.Equal(t, "actor", cast[0].RoleType)
	require.NotNil(t, cast[0].CharacterName)
	assert.Equal(t, "Hero", *cast[0].CharacterName)
	assert.Equal(t, 0, cast[0].SortOrder)
	require.NotNil(t, cast[0].ThumbPath)
	assert.Equal(t, thumb, *cast[0].ThumbPath)

	assert.Equal(t, p2.ID, cast[1].PersonID)
	assert.Equal(t, "Bob Smith", cast[1].PersonName)
	assert.Equal(t, "director", cast[1].RoleType)
	assert.Nil(t, cast[1].CharacterName)
	assert.Equal(t, 1, cast[1].SortOrder)
	assert.Nil(t, cast[1].ThumbPath)
}

func TestMediaItemQueryRepoAdapter_GetCastByMediaItem_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("cq-mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	cast, err := queryRepo.GetCastByMediaItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, cast)
}

func TestMediaItemQueryRepoAdapter_GetCastForItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-blib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item1 := newTestMediaItem("cq-bmi1", lib.ID)
	item2 := newTestMediaItem("cq-bmi2", lib.ID)
	item3 := newTestMediaItem("cq-bmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))
	require.NoError(t, itemRepo.Create(ctx, item3))

	thumb := "/thumbs/p.jpg"
	p1 := &library.Person{ID: "cq-bp1", Name: "Actor One", ProviderIDs: map[string]string{}, ThumbPath: &thumb}
	p2 := &library.Person{ID: "cq-bp2", Name: "Actor Two", ProviderIDs: map[string]string{}, ThumbPath: nil}
	require.NoError(t, personRepo.Create(ctx, p1))
	require.NoError(t, personRepo.Create(ctx, p2))

	char := "Hero"
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item1.ID, PersonID: p1.ID, RoleType: "actor", CharacterName: &char, SortOrder: 0}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item2.ID, PersonID: p2.ID, RoleType: "director", CharacterName: nil, SortOrder: 0}))
	// item3 has no cast

	result, err := queryRepo.GetCastForItems(ctx, []shared.MediaItemID{item1.ID, item2.ID, item3.ID})
	require.NoError(t, err)

	require.Len(t, result[item1.ID], 1)
	assert.Equal(t, p1.ID, result[item1.ID][0].PersonID)
	assert.Equal(t, "actor", result[item1.ID][0].RoleType)

	require.Len(t, result[item2.ID], 1)
	assert.Equal(t, p2.ID, result[item2.ID][0].PersonID)
	assert.Equal(t, "director", result[item2.ID][0].RoleType)

	assert.Empty(t, result[item3.ID])
}

func TestMediaItemQueryRepoAdapter_GetCastForItems_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)
	result, err := queryRepo.GetCastForItems(context.Background(), []shared.MediaItemID{})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestMediaItemQueryRepoAdapter_List_Count(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cq-mi3a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cq-mi3b", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cq-mi3c", lib.ID)))

	libID := lib.ID
	filter := library.ListMediaItemsFilter{LibraryID: &libID, Limit: 10}

	list, err := queryRepo.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	count, err := queryRepo.Count(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestMediaItemRepo_List_AllowedRatings(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)

	lib := newTestLibrary("cq-lib4")
	require.NoError(t, libRepo.Create(ctx, lib))

	pg := newTestMediaItem("cq-mi-pg", lib.ID)
	pg.OfficialRating = "PG"
	require.NoError(t, itemRepo.Create(ctx, pg))

	r := newTestMediaItem("cq-mi-r", lib.ID)
	r.OfficialRating = "R"
	require.NoError(t, itemRepo.Create(ctx, r))

	noRating := newTestMediaItem("cq-mi-norating", lib.ID)
	noRating.OfficialRating = ""
	require.NoError(t, itemRepo.Create(ctx, noRating))

	libID := lib.ID
	filter := library.ListMediaItemsFilter{
		LibraryID:      &libID,
		AllowedRatings: []string{"G", "TV-Y", "TV-Y7", "TV-G", "PG", "TV-PG", "PG-13", "TV-14"},
		Limit:          10,
	}

	list, err := itemRepo.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, list, 2, "PG and empty-rating items should be returned; R should be excluded")

	ids := make(map[string]bool)
	for _, item := range list {
		ids[string(item.ID)] = true
	}
	assert.True(t, ids["cq-mi-pg"])
	assert.True(t, ids["cq-mi-norating"])
	assert.False(t, ids["cq-mi-r"])
}
