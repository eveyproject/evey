package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/favorite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFavoriteRepo_Add(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	favRepo := postgres.NewFavoriteRepository(db)
	ctx := context.Background()

	u := newTestUser("favu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("favp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("favlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi := newTestMediaItem("favmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi))

	fav := &favorite.Favorite{
		ID:          "fav1",
		ProfileID:   p.ID,
		MediaItemID: mi.ID,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, favRepo.Add(ctx, fav))

	isFav, err := favRepo.IsFavorite(ctx, string(p.ID), string(mi.ID))
	require.NoError(t, err)
	assert.True(t, isFav)

	// duplicate add should not error (ON CONFLICT DO NOTHING)
	fav2 := &favorite.Favorite{
		ID:          "fav1dup",
		ProfileID:   p.ID,
		MediaItemID: mi.ID,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, favRepo.Add(ctx, fav2))
}

func TestFavoriteRepo_Remove(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	favRepo := postgres.NewFavoriteRepository(db)
	ctx := context.Background()

	u := newTestUser("favu2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("favp2", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("favlib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi := newTestMediaItem("favmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi))

	fav := &favorite.Favorite{
		ID: "fav2", ProfileID: p.ID, MediaItemID: mi.ID,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, favRepo.Add(ctx, fav))

	require.NoError(t, favRepo.Remove(ctx, string(p.ID), string(mi.ID)))

	isFav, err := favRepo.IsFavorite(ctx, string(p.ID), string(mi.ID))
	require.NoError(t, err)
	assert.False(t, isFav)

	// removing a non-existent favorite should not error
	require.NoError(t, favRepo.Remove(ctx, string(p.ID), string(mi.ID)))
}

func TestFavoriteRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	favRepo := postgres.NewFavoriteRepository(db)
	ctx := context.Background()

	u := newTestUser("favu3")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("favp3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("favlib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("favmi3a", lib.ID)
	mi2 := newTestMediaItem("favmi3b", lib.ID)
	mi3 := newTestMediaItem("favmi3c", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))
	require.NoError(t, itemRepo.Create(ctx, mi3))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, favRepo.Add(ctx, &favorite.Favorite{ID: "fav3a", ProfileID: p.ID, MediaItemID: mi1.ID, CreatedAt: now}))
	require.NoError(t, favRepo.Add(ctx, &favorite.Favorite{ID: "fav3b", ProfileID: p.ID, MediaItemID: mi2.ID, CreatedAt: now}))
	require.NoError(t, favRepo.Add(ctx, &favorite.Favorite{ID: "fav3c", ProfileID: p.ID, MediaItemID: mi3.ID, CreatedAt: now}))

	list, err := favRepo.List(ctx, string(p.ID), 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// pagination
	page1, err := favRepo.List(ctx, string(p.ID), 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := favRepo.List(ctx, string(p.ID), 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 1)
}

func TestFavoriteRepo_IsFavorite(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	favRepo := postgres.NewFavoriteRepository(db)
	ctx := context.Background()

	u := newTestUser("favu4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("favp4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("favlib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("favmi4a", lib.ID)
	mi2 := newTestMediaItem("favmi4b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, favRepo.Add(ctx, &favorite.Favorite{ID: "fav4", ProfileID: p.ID, MediaItemID: mi1.ID, CreatedAt: now}))

	isFav, err := favRepo.IsFavorite(ctx, string(p.ID), string(mi1.ID))
	require.NoError(t, err)
	assert.True(t, isFav)

	isNotFav, err := favRepo.IsFavorite(ctx, string(p.ID), string(mi2.ID))
	require.NoError(t, err)
	assert.False(t, isNotFav)
}

func TestFavoriteRepo_IsFavoriteMany(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	favRepo := postgres.NewFavoriteRepository(db)
	ctx := context.Background()

	u := newTestUser("favu5")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("favp5", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	other := newTestProfile("favp5b", u.ID)
	require.NoError(t, profileRepo.Create(ctx, other))
	lib := newTestLibrary("favlib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("favmi5a", lib.ID)
	mi2 := newTestMediaItem("favmi5b", lib.ID)
	mi3 := newTestMediaItem("favmi5c", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))
	require.NoError(t, itemRepo.Create(ctx, mi3))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, favRepo.Add(ctx, &favorite.Favorite{ID: "fav5a", ProfileID: p.ID, MediaItemID: mi1.ID, CreatedAt: now}))
	// Another profile's favorite on mi3 must never leak into p's answer.
	require.NoError(t, favRepo.Add(ctx, &favorite.Favorite{ID: "fav5c", ProfileID: other.ID, MediaItemID: mi3.ID, CreatedAt: now}))

	got, err := favRepo.IsFavoriteMany(ctx, string(p.ID), []string{string(mi1.ID), string(mi2.ID), string(mi3.ID)})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{string(mi1.ID): true}, got)

	// An empty list must not run `= ANY(NULL)`, which is NULL and never true.
	empty, err := favRepo.IsFavoriteMany(ctx, string(p.ID), nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
