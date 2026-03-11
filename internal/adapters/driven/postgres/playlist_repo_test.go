package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/playlist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlaylist(id, userID string) *playlist.Playlist {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &playlist.Playlist{
		ID:          id,
		Name:        "Playlist " + id,
		Description: "Desc for " + id,
		UserID:      userID,
		Type:        playlist.Manual,
		Rules:       []playlist.Rule{},
		SortBy:      "sort_order",
		SortOrder:   "asc",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestPlaylistRepo_Create(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	ctx := context.Background()

	u := newTestUser("plu1")
	require.NoError(t, userRepo.Create(ctx, u))

	pl := newTestPlaylist("pl1", u.ID)
	require.NoError(t, plRepo.Create(ctx, pl))

	got, err := plRepo.GetByID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Equal(t, pl.Name, got.Name)
	assert.Equal(t, pl.Description, got.Description)
	assert.Equal(t, pl.UserID, got.UserID)
	assert.Equal(t, playlist.Manual, got.Type)
}

func TestPlaylistRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	ctx := context.Background()

	u1 := newTestUser("plu2")
	require.NoError(t, userRepo.Create(ctx, u1))
	u2 := newTestUser("plu3")
	require.NoError(t, userRepo.Create(ctx, u2))

	require.NoError(t, plRepo.Create(ctx, newTestPlaylist("pl2a", u1.ID)))
	require.NoError(t, plRepo.Create(ctx, newTestPlaylist("pl2b", u1.ID)))
	require.NoError(t, plRepo.Create(ctx, newTestPlaylist("pl2c", u2.ID)))

	list, err := plRepo.List(ctx, u1.ID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, pl := range list {
		assert.Equal(t, u1.ID, pl.UserID)
	}

	list2, err := plRepo.List(ctx, u2.ID)
	require.NoError(t, err)
	assert.Len(t, list2, 1)
}

func TestPlaylistRepo_AddItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu4")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("pllib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("plmi1", lib.ID)
	mi2 := newTestMediaItem("plmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))

	pl := newTestPlaylist("pl3", u.ID)
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &playlist.PlaylistItem{
		PlaylistID: pl.ID, MediaItemID: mi1.ID, SortOrder: 0, AddedAt: now,
	}))
	require.NoError(t, plItemRepo.Add(ctx, &playlist.PlaylistItem{
		PlaylistID: pl.ID, MediaItemID: mi2.ID, SortOrder: 1, AddedAt: now,
	}))

	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, mi1.ID, items[0].MediaItemID)
	assert.Equal(t, 0, items[0].SortOrder)
	assert.Equal(t, mi2.ID, items[1].MediaItemID)
	assert.Equal(t, 1, items[1].SortOrder)
}

func TestPlaylistRepo_ReorderItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu5")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("pllib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("plmi3", lib.ID)
	mi2 := newTestMediaItem("plmi4", lib.ID)
	mi3 := newTestMediaItem("plmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))
	require.NoError(t, itemRepo.Create(ctx, mi3))

	pl := newTestPlaylist("pl4", u.ID)
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &playlist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi1.ID, SortOrder: 0, AddedAt: now}))
	require.NoError(t, plItemRepo.Add(ctx, &playlist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi2.ID, SortOrder: 1, AddedAt: now}))
	require.NoError(t, plItemRepo.Add(ctx, &playlist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi3.ID, SortOrder: 2, AddedAt: now}))

	require.NoError(t, plItemRepo.Reorder(ctx, pl.ID, []string{mi3.ID, mi1.ID, mi2.ID}))

	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Len(t, items, 3)
	assert.Equal(t, mi3.ID, items[0].MediaItemID)
	assert.Equal(t, 0, items[0].SortOrder)
	assert.Equal(t, mi1.ID, items[1].MediaItemID)
	assert.Equal(t, 1, items[1].SortOrder)
	assert.Equal(t, mi2.ID, items[2].MediaItemID)
	assert.Equal(t, 2, items[2].SortOrder)
}

func TestPlaylistRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu6")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("pllib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi := newTestMediaItem("plmi6", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi))

	pl := newTestPlaylist("pl5", u.ID)
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &playlist.PlaylistItem{
		PlaylistID: pl.ID, MediaItemID: mi.ID, SortOrder: 0, AddedAt: now,
	}))

	require.NoError(t, plRepo.Delete(ctx, pl.ID))

	_, err := plRepo.GetByID(ctx, pl.ID)
	assert.ErrorIs(t, err, playlist.ErrNotFound)

	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Empty(t, items)
}
