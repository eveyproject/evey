package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	domainplaylist "github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlaylist(id, profileID string) *domainplaylist.Playlist {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &domainplaylist.Playlist{
		ID:          id,
		Name:        "Playlist " + id,
		Description: "Desc for " + id,
		ProfileID:   shared.ProfileID(profileID),
		Type:        domainplaylist.Manual,
		Rules:       []domainplaylist.Rule{},
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
	profileRepo := postgres.NewProfileRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	ctx := context.Background()

	u := newTestUser("plu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("plp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	pl := newTestPlaylist("pl1", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))

	got, err := plRepo.GetByID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Equal(t, pl.Name, got.Name)
	assert.Equal(t, pl.Description, got.Description)
	assert.Equal(t, pl.ProfileID, got.ProfileID)
	assert.Equal(t, domainplaylist.Manual, got.Type)
}

func TestPlaylistRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	ctx := context.Background()

	u1 := newTestUser("plu2")
	require.NoError(t, userRepo.Create(ctx, u1))
	u2 := newTestUser("plu3")
	require.NoError(t, userRepo.Create(ctx, u2))
	p1 := newTestProfile("plp2a", u1.ID)
	require.NoError(t, profileRepo.Create(ctx, p1))
	p2 := newTestProfile("plp2b", u2.ID)
	require.NoError(t, profileRepo.Create(ctx, p2))

	require.NoError(t, plRepo.Create(ctx, newTestPlaylist("pl2a", string(p1.ID))))
	require.NoError(t, plRepo.Create(ctx, newTestPlaylist("pl2b", string(p1.ID))))
	require.NoError(t, plRepo.Create(ctx, newTestPlaylist("pl2c", string(p2.ID))))

	list, err := plRepo.List(ctx, string(p1.ID))
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, pl := range list {
		assert.Equal(t, string(p1.ID), string(pl.ProfileID))
	}

	list2, err := plRepo.List(ctx, string(p2.ID))
	require.NoError(t, err)
	assert.Len(t, list2, 1)
}

func TestPlaylistRepo_AddItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("plp4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pllib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("plmi1", lib.ID)
	mi2 := newTestMediaItem("plmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))

	pl := newTestPlaylist("pl3", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{
		PlaylistID: pl.ID, MediaItemID: mi1.ID, SortOrder: 0, AddedAt: now,
	}))
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{
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

func TestPlaylistItemRepo_AddNextOrder(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("ano-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("ano-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("ano-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	mids := []string{"ano-m1", "ano-m2", "ano-m3"}
	for _, id := range mids {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, lib.ID)))
	}
	pl := newTestPlaylist("ano-pl", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, id := range mids {
		require.NoError(t, plItemRepo.AddNextOrder(ctx, &domainplaylist.PlaylistItem{
			PlaylistID: pl.ID, MediaItemID: shared.MediaItemID(id), AddedAt: now, MediaType: domainplaylist.MediaTypeVideo,
		}, 0))
	}
	// First insert over an empty playlist must land at order 0, then 1, 2.
	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	require.Len(t, items, 3)
	for i, it := range items { // GetByPlaylistID returns sorted by sort_order
		assert.Equal(t, i, it.SortOrder, "sequential adds get incrementing sort_order")
	}

	// A duplicate media item is a no-op (ON CONFLICT DO NOTHING).
	require.NoError(t, plItemRepo.AddNextOrder(ctx, &domainplaylist.PlaylistItem{
		PlaylistID: pl.ID, MediaItemID: shared.MediaItemID("ano-m1"), AddedAt: now, MediaType: domainplaylist.MediaTypeVideo,
	}, 0))
	items, err = plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Len(t, items, 3, "duplicate add must be a no-op")
}

func TestPlaylistRepo_ReorderItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu5")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("plp5", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pllib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("plmi3", lib.ID)
	mi2 := newTestMediaItem("plmi4", lib.ID)
	mi3 := newTestMediaItem("plmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))
	require.NoError(t, itemRepo.Create(ctx, mi3))

	pl := newTestPlaylist("pl4", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi1.ID, SortOrder: 0, AddedAt: now}))
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi2.ID, SortOrder: 1, AddedAt: now}))
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi3.ID, SortOrder: 2, AddedAt: now}))

	require.NoError(t, plItemRepo.Reorder(ctx, pl.ID, []string{string(mi3.ID), string(mi1.ID), string(mi2.ID)}))

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

// GetByPlaylistIDs is the batch form of GetByPlaylistID (fixes the playlist
// generator's N+1 de-duplication scan) - a single ANY($1) query must group
// rows correctly per playlist, and a playlist with no items must simply be
// absent from the result rather than an empty-slice entry.
func TestPlaylistItemRepo_GetByPlaylistIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu6")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("plp6", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pllib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi1 := newTestMediaItem("plmi6", lib.ID)
	mi2 := newTestMediaItem("plmi7", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))

	plA := newTestPlaylist("pl6a", string(p.ID))
	plB := newTestPlaylist("pl6b", string(p.ID))
	plEmpty := newTestPlaylist("pl6empty", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, plA))
	require.NoError(t, plRepo.Create(ctx, plB))
	require.NoError(t, plRepo.Create(ctx, plEmpty))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: plA.ID, MediaItemID: mi1.ID, SortOrder: 0, AddedAt: now}))
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: plB.ID, MediaItemID: mi2.ID, SortOrder: 0, AddedAt: now}))

	got, err := plItemRepo.GetByPlaylistIDs(ctx, []string{plA.ID, plB.ID, plEmpty.ID})
	require.NoError(t, err)
	require.Len(t, got[plA.ID], 1)
	assert.Equal(t, mi1.ID, got[plA.ID][0].MediaItemID)
	require.Len(t, got[plB.ID], 1)
	assert.Equal(t, mi2.ID, got[plB.ID][0].MediaItemID)
	_, hasEmpty := got[plEmpty.ID]
	assert.False(t, hasEmpty, "a playlist with no items must be absent from the map")

	empty, err := plItemRepo.GetByPlaylistIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestSmartPlaylistQuerier_Query_NullParentID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	querier := postgres.NewSmartPlaylistQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("sqllib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	mi1 := newTestMediaItem("sqmi1", lib.ID)
	mi1.Catalog.Genres = []string{"Action"}
	mi2 := newTestMediaItem("sqmi2", lib.ID)
	mi2.Catalog.Genres = []string{"Drama"}
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))

	spec, err := domainplaylist.CompileRules(nil)
	require.NoError(t, err)
	ids, err := querier.Query(ctx, spec, "title", "asc", 100)
	require.NoError(t, err, "Query must handle NULL parent_id without scan error")
	assert.Len(t, ids, 2)
}

func TestSmartPlaylistQuerier_Query_GenreFilter(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	querier := postgres.NewSmartPlaylistQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("sqllib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	action := newTestMediaItem("sqmi3", lib.ID)
	action.Catalog.Genres = []string{"Action"}
	drama := newTestMediaItem("sqmi4", lib.ID)
	drama.Catalog.Genres = []string{"Drama"}
	require.NoError(t, itemRepo.Create(ctx, action))
	require.NoError(t, itemRepo.Create(ctx, drama))

	spec, err := domainplaylist.CompileRules([]domainplaylist.Rule{{Field: "genre", Operator: "eq", Value: "Action"}})
	require.NoError(t, err)
	ids, err := querier.Query(ctx, spec, "title", "asc", 100)
	require.NoError(t, err)
	assert.Len(t, ids, 1)
	assert.Equal(t, string(action.ID), string(ids[0]))
}

func TestPlaylistRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaylistRepository(db)
	err := repo.Delete(context.Background(), "playlist-missing")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPlaylistRepo_Update_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaylistRepository(db)
	pl := newTestPlaylist("playlist-missing", "u")
	err := repo.Update(context.Background(), pl)
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPlaylistRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("plu6")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("plp6", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pllib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	mi := newTestMediaItem("plmi6", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi))

	pl := newTestPlaylist("pl5", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{
		PlaylistID: pl.ID, MediaItemID: mi.ID, SortOrder: 0, AddedAt: now,
	}))

	require.NoError(t, plRepo.Delete(ctx, pl.ID))

	_, err := plRepo.GetByID(ctx, pl.ID)
	require.ErrorIs(t, err, shared.ErrNotFound)

	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// A double-tapped Create used to land two identical playlists: the unique
// (profile_id, name) index (migration 000148) now refuses the second one and
// the repo surfaces the domain sentinel, not a raw pq error.
func TestPlaylistRepo_DuplicateNameRefused(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	ctx := context.Background()

	u := newTestUser("dup-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("dup-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	first := newTestPlaylist("dup-pl-1", string(p.ID))
	first.Name = "Soirée"
	require.NoError(t, plRepo.Create(ctx, first))

	second := newTestPlaylist("dup-pl-2", string(p.ID))
	second.Name = "Soirée"
	err := plRepo.Create(ctx, second)
	require.ErrorIs(t, err, domainplaylist.ErrDuplicateName)

	// A DIFFERENT profile may reuse the name.
	p2 := newTestProfile("dup-p2", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p2))
	third := newTestPlaylist("dup-pl-3", string(p2.ID))
	third.Name = "Soirée"
	require.NoError(t, plRepo.Create(ctx, third))

	// Renaming onto a taken name is the same violation.
	second.Name = "Autre"
	require.NoError(t, plRepo.Create(ctx, second))
	second.Name = "Soirée"
	require.ErrorIs(t, plRepo.Update(ctx, second), domainplaylist.ErrDuplicateName)
}

// The 500-item cap used to be a read-then-insert in Go: two concurrent adds at
// 499 both passed. The cap now rides INSIDE the insert statement; the repo
// reports the cap distinctly from the silent duplicate no-op.
func TestPlaylistItemRepo_AddNextOrder_CapInsideInsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	ctx := context.Background()

	u := newTestUser("cap-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("cap-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("cap-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	for _, id := range []string{"cap-m1", "cap-m2", "cap-m3"} {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, lib.ID)))
	}
	pl := newTestPlaylist("cap-pl", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))

	now := time.Now().UTC()
	add := func(mid string, max int) error {
		return plItemRepo.AddNextOrder(ctx, &domainplaylist.PlaylistItem{
			PlaylistID: pl.ID, MediaItemID: shared.MediaItemID(mid), AddedAt: now, MediaType: domainplaylist.MediaTypeVideo,
		}, max)
	}
	require.NoError(t, add("cap-m1", 2))
	require.NoError(t, add("cap-m2", 2))
	// Cap reached: the third insert must be refused ATOMICALLY by the statement.
	require.ErrorIs(t, add("cap-m3", 2), domainplaylist.ErrMaxItemsReached)
	// Re-adding an existing item stays the silent no-op it always was (the
	// conflict path), NOT a cap error.
	require.NoError(t, add("cap-m1", 3))
	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

// Removing a title from a playlist touches that one row and says so when
// there was nothing to remove.
func TestPlaylistItemRepo_Remove(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("plu-rm")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("plp-rm", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("pllib-rm")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	mi1, mi2 := newTestMediaItem("plmi-rm1", lib.ID), newTestMediaItem("plmi-rm2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, mi1))
	require.NoError(t, itemRepo.Create(ctx, mi2))
	plRepo := postgres.NewPlaylistRepository(db)
	plItemRepo := postgres.NewPlaylistItemRepository(db)
	pl := newTestPlaylist("pl-rm", string(p.ID))
	require.NoError(t, plRepo.Create(ctx, pl))
	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi1.ID, SortOrder: 0, AddedAt: now}))
	require.NoError(t, plItemRepo.Add(ctx, &domainplaylist.PlaylistItem{PlaylistID: pl.ID, MediaItemID: mi2.ID, SortOrder: 1, AddedAt: now}))

	require.NoError(t, plItemRepo.Remove(ctx, pl.ID, string(mi1.ID)))
	items, err := plItemRepo.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, mi2.ID, items[0].MediaItemID)
	require.ErrorIs(t, plItemRepo.Remove(ctx, pl.ID, string(mi1.ID)), shared.ErrNotFound, "already gone")
	require.ErrorIs(t, plItemRepo.Remove(ctx, "pl-other", string(mi2.ID)), shared.ErrNotFound, "another playlist's row is not touched")
}

// playlist_items has no FK (a row points at a media item OR a music track), so
// a removed media used to leave its row behind, still counted by the cap.
func TestPlaylistItemRepo_PurgeOrphans(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	u := newTestUser("plorph-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("plorph-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("plorph-lib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("plorph-kept", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("plorph-gone", lib.ID)))
	trackID, err := postgres.NewMusicTrackRepository(db).Upsert(ctx, newTestTrack("plorph-track", lib.ID, "T", "A", "Alb"))
	require.NoError(t, err)

	pl := newTestPlaylist("plorph-pl", string(p.ID))
	require.NoError(t, postgres.NewPlaylistRepository(db).Create(ctx, pl))
	items := postgres.NewPlaylistItemRepository(db)
	now := time.Now().UTC()
	for i, it := range []struct{ id, mediaType string }{
		{"plorph-kept", domainplaylist.MediaTypeVideo},
		{"plorph-gone", domainplaylist.MediaTypeVideo},
		{trackID, domainplaylist.MediaTypeMusic},
		{"plorph-no-such-track", domainplaylist.MediaTypeMusic},
	} {
		require.NoError(t, items.Add(ctx, &domainplaylist.PlaylistItem{
			PlaylistID: pl.ID, MediaItemID: shared.MediaItemID(it.id), SortOrder: i, AddedAt: now, MediaType: it.mediaType,
		}))
	}
	require.NoError(t, itemRepo.Delete(ctx, "plorph-gone"))

	n, err := items.PurgeOrphans(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	got, err := items.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	var ids []string
	for _, it := range got {
		ids = append(ids, string(it.MediaItemID))
	}
	assert.Equal(t, []string{"plorph-kept", trackID}, ids)
}

// PurgeRemoved drops the rows of one removed media item or music track, by
// kind, and leaves an id that still exists alone.
func TestPlaylistItemRepo_PurgeRemoved(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	u := newTestUser("plrm-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("plrm-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("plrm-lib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("plrm-kept", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("plrm-gone", lib.ID)))
	tracks := postgres.NewMusicTrackRepository(db)
	trackID, err := tracks.Upsert(ctx, newTestTrack("plrm-track", lib.ID, "T", "A", "Alb"))
	require.NoError(t, err)

	pl := newTestPlaylist("plrm-pl", string(p.ID))
	require.NoError(t, postgres.NewPlaylistRepository(db).Create(ctx, pl))
	items := postgres.NewPlaylistItemRepository(db)
	now := time.Now().UTC()
	for i, it := range []struct{ id, mediaType string }{
		{"plrm-kept", domainplaylist.MediaTypeVideo},
		{"plrm-gone", domainplaylist.MediaTypeVideo},
		{trackID, domainplaylist.MediaTypeMusic},
		{"plrm-gone-track", domainplaylist.MediaTypeMusic},
	} {
		require.NoError(t, items.Add(ctx, &domainplaylist.PlaylistItem{
			PlaylistID: pl.ID, MediaItemID: shared.MediaItemID(it.id), SortOrder: i, AddedAt: now, MediaType: it.mediaType,
		}))
	}
	require.NoError(t, itemRepo.Delete(ctx, "plrm-gone"))

	n, err := items.PurgeRemoved(ctx, "plrm-kept", false)
	require.NoError(t, err)
	assert.Zero(t, n, "an id that still exists keeps its rows")
	n, err = items.PurgeRemoved(ctx, trackID, true)
	require.NoError(t, err)
	assert.Zero(t, n, "a track that still exists keeps its rows")
	n, err = items.PurgeRemoved(ctx, "plrm-gone", false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	n, err = items.PurgeRemoved(ctx, "plrm-gone-track", true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	got, err := items.GetByPlaylistID(ctx, pl.ID)
	require.NoError(t, err)
	var ids []string
	for _, it := range got {
		ids = append(ids, string(it.MediaItemID))
	}
	assert.Equal(t, []string{"plrm-kept", trackID}, ids)
}
