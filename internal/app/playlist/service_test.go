package playlist

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/playlist/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

type testEnv struct {
	svc         *Service
	plRepo      *fakes.FakePlaylistRepository
	itemRepo    *fakes.FakePlaylistItemRepository
	querier     *fakes.FakeSmartPlaylistQuerier
	mediaReader *libfakes.FakeMediaItemQueryRepo
}

// seed populates both the smart querier (as a Candidate) and the media reader
// (as the full MediaItem) so GetItems for a smart playlist returns items the
// same way prod would: Spec filters → IDs → hydrate.
func (e *testEnv) seed(items ...*library.MediaItem) {
	for _, item := range items {
		e.mediaReader.Seed(item)
		e.querier.Candidates = append(e.querier.Candidates, toCandidate(item))
	}
}

func toCandidate(item *library.MediaItem) playlist.Candidate {
	c := playlist.Candidate{
		ID:        item.ID,
		Type:      string(item.Type),
		SortTitle: item.SortTitle,
		AddedAt:   item.AddedAt,
	}
	if item.Catalog != nil {
		c.Genres = item.Catalog.Genres
		c.Year = item.Catalog.Year
	}
	if item.Classification != nil {
		c.CommunityRating = item.Classification.CommunityRating
	}
	c.LibraryID = string(item.LibraryID)
	return c
}

func newTestEnv() *testEnv {
	plRepo := fakes.NewFakePlaylistRepository()
	itemRepo := fakes.NewFakePlaylistItemRepository()
	querier := fakes.NewFakeSmartPlaylistQuerier()
	mediaReader := libfakes.NewFakeMediaItemQueryRepo()
	svc := NewService(plRepo, itemRepo, querier, mediaReader)
	return &testEnv{svc: svc, plRepo: plRepo, itemRepo: itemRepo, querier: querier, mediaReader: mediaReader}
}

func newTestService() (*Service, *fakes.FakePlaylistRepository, *fakes.FakePlaylistItemRepository, *fakes.FakeSmartPlaylistQuerier) {
	env := newTestEnv()
	return env.svc, env.plRepo, env.itemRepo, env.querier
}

func TestCreatePlaylist(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{
		Name:      "My Playlist",
		ProfileID: "user-1",
		Type:      playlist.Manual,
	}
	if err := svc.Create(context.Background(), pl); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl.ID == "" {
		t.Fatal("expected ID to be set")
	}
	if pl.SortBy != "sort_order" {
		t.Fatalf("expected default SortBy, got %s", pl.SortBy)
	}
}

func TestGetPlaylistOwnershipCheck(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Test", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	_, err := svc.GetByID(context.Background(), pl.ID, "user-2")
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}

	got, err := svc.GetByID(context.Background(), pl.ID, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Test" {
		t.Fatalf("expected 'Test', got %s", got.Name)
	}
}

func TestListPlaylistsFiltersByUser(t *testing.T) {
	svc, _, _, _ := newTestService()
	svc.Create(context.Background(), &playlist.Playlist{Name: "A", ProfileID: "user-1", Type: playlist.Manual})
	svc.Create(context.Background(), &playlist.Playlist{Name: "B", ProfileID: "user-2", Type: playlist.Manual})
	svc.Create(context.Background(), &playlist.Playlist{Name: "C", ProfileID: "user-1", Type: playlist.Smart})

	list, err := svc.List(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(list))
	}
}

func TestAddAndRemoveItem(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Manual", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	if err := svc.AddItem(context.Background(), pl.ID, "media-1", "user-1"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.AddItem(context.Background(), pl.ID, "media-2", "user-1"); err != nil {
		t.Fatalf("add: %v", err)
	}

	_, items, err := svc.GetItems(context.Background(), pl.ID, "user-1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if removeErr := svc.RemoveItem(context.Background(), pl.ID, "media-1", "user-1"); removeErr != nil {
		t.Fatalf("remove: %v", removeErr)
	}
	_, items, err = svc.GetItems(context.Background(), pl.ID, "user-1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestCannotAddItemsToSmartPlaylist(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{
		Name:      "Smart",
		ProfileID: "user-1",
		Type:      playlist.Smart,
		Rules:     []playlist.Rule{{Field: "genre", Operator: "eq", Value: "Action"}},
	}
	svc.Create(context.Background(), pl)

	err := svc.AddItem(context.Background(), pl.ID, "media-1", "user-1")
	if !errors.Is(err, ErrSmartItems) {
		t.Fatalf("expected ErrSmartItems, got %v", err)
	}
}

func TestSmartPlaylistGetItems(t *testing.T) {
	env := newTestEnv()
	env.seed(
		&library.MediaItem{ID: "m-1", Title: "Action Movie", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		&library.MediaItem{ID: "m-2", Title: "Drama Movie", Catalog: &library.MediaCatalog{Genres: []string{"Drama"}}},
		&library.MediaItem{ID: "m-3", Title: "Action Drama", Catalog: &library.MediaCatalog{Genres: []string{"Action", "Drama"}}},
	)

	pl := &playlist.Playlist{
		Name:      "Action",
		ProfileID: "user-1",
		Type:      playlist.Smart,
		Rules:     []playlist.Rule{{Field: "genre", Operator: "eq", Value: "Action"}},
	}
	env.svc.Create(context.Background(), pl)

	mediaItems, _, err := env.svc.GetItems(context.Background(), pl.ID, "user-1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(mediaItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(mediaItems))
	}
}

func TestUpdatePlaylist(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Old", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	updated := &playlist.Playlist{ID: pl.ID, Name: "New", Description: "updated"}
	if err := svc.Update(context.Background(), updated, "user-1"); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := svc.GetByID(context.Background(), pl.ID, "user-1")
	if got.Name != "New" {
		t.Fatalf("expected 'New', got %s", got.Name)
	}
	if got.Type != playlist.Manual {
		t.Fatal("type should not change")
	}
}

func TestDeletePlaylistOwnership(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Mine", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	if err := svc.Delete(context.Background(), pl.ID, "user-2"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
	if err := svc.Delete(context.Background(), pl.ID, "user-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := svc.GetByID(context.Background(), pl.ID, "user-1")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestMaxItemsLimit(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Big", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	for i := range maxItems {
		if err := svc.AddItem(context.Background(), pl.ID, "media-"+string(rune(i+'A')), "user-1"); err != nil {
			t.Fatalf("add item %d: %v", i, err)
		}
	}

	err := svc.AddItem(context.Background(), pl.ID, "media-overflow", "user-1")
	if !errors.Is(err, ErrMaxItems) {
		t.Fatalf("expected ErrMaxItems, got %v", err)
	}
}

type fakeMediaChecker struct {
	items map[string]*library.MediaItem
}

func (f *fakeMediaChecker) GetByID(_ context.Context, id string) (*library.MediaItem, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, shared.ErrNotFound
}

type errorMediaChecker struct {
	err error
}

func (f *errorMediaChecker) GetByID(_ context.Context, _ string) (*library.MediaItem, error) {
	return nil, f.err
}

func TestAddItem_InvalidMediaID(t *testing.T) {
	svc, _, _, _ := newTestService()
	checker := &fakeMediaChecker{items: map[string]*library.MediaItem{}}
	svc.WithMediaChecker(checker)

	pl := &playlist.Playlist{Name: "Manual", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	err := svc.AddItem(context.Background(), pl.ID, "nonexistent-id", "user-1")
	if !errors.Is(err, ErrMediaItemNotFound) {
		t.Fatalf("expected ErrMediaItemNotFound, got %v", err)
	}
}

func TestAddItem_ValidMediaID(t *testing.T) {
	svc, _, _, _ := newTestService()
	checker := &fakeMediaChecker{items: map[string]*library.MediaItem{
		"media-1": {ID: "media-1", Title: "Test Movie"},
	}}
	svc.WithMediaChecker(checker)

	pl := &playlist.Playlist{Name: "Manual", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	if err := svc.AddItem(context.Background(), pl.ID, "media-1", "user-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, items, err := svc.GetItems(context.Background(), pl.ID, "user-1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

type fakeLibraryAccess struct {
	allow map[string]bool // keyed by libraryID
	err   error
}

func (f *fakeLibraryAccess) HasAccessForProfile(_ context.Context, _ shared.ProfileID, libraryID shared.LibraryID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.allow[string(libraryID)], nil
}

func TestAddItem_LibraryAccessDenied(t *testing.T) {
	svc, _, _, _ := newTestService()
	svc.WithMediaChecker(&fakeMediaChecker{items: map[string]*library.MediaItem{
		"media-1": {ID: "media-1", Title: "Test Movie", LibraryID: "lib-private"},
	}})
	svc.WithLibraryAccessChecker(&fakeLibraryAccess{allow: map[string]bool{"lib-public": true}})

	pl := &playlist.Playlist{Name: "Manual", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	// Media exists but lives in a library the profile cannot access: must look
	// exactly like a missing item (no existence leak) and must not be added.
	err := svc.AddItem(context.Background(), pl.ID, "media-1", "user-1")
	if !errors.Is(err, ErrMediaItemNotFound) {
		t.Fatalf("expected ErrMediaItemNotFound on inaccessible library, got %v", err)
	}
	_, items, _ := svc.GetItems(context.Background(), pl.ID, "user-1")
	if len(items) != 0 {
		t.Fatalf("inaccessible item must not be added, got %d items", len(items))
	}
}

func TestAddItem_LibraryAccessGranted(t *testing.T) {
	svc, _, _, _ := newTestService()
	svc.WithMediaChecker(&fakeMediaChecker{items: map[string]*library.MediaItem{
		"media-1": {ID: "media-1", Title: "Test Movie", LibraryID: "lib-public"},
	}})
	svc.WithLibraryAccessChecker(&fakeLibraryAccess{allow: map[string]bool{"lib-public": true}})

	pl := &playlist.Playlist{Name: "Manual", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	if err := svc.AddItem(context.Background(), pl.ID, "media-1", "user-1"); err != nil {
		t.Fatalf("unexpected error adding accessible item: %v", err)
	}
	_, items, _ := svc.GetItems(context.Background(), pl.ID, "user-1")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

type fakeMusicChecker struct {
	ids map[string]bool
	err error
}

func (f *fakeMusicChecker) Exists(_ context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.ids[id], nil
}

func TestAddItem_MusicType_ValidTrack(t *testing.T) {
	svc, _, _, _ := newTestService()
	svc.WithMusicTrackChecker(&fakeMusicChecker{ids: map[string]bool{"track-1": true}})

	pl := &playlist.Playlist{Name: "Mix", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	if err := svc.AddItemWithType(context.Background(), pl.ID, "track-1", "u1", "music"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, items, err := svc.GetItems(context.Background(), pl.ID, "u1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].MediaType != "music" {
		t.Fatalf("MediaType = %q, want music", items[0].MediaType)
	}
}

func TestAddItem_MusicType_UnknownTrack(t *testing.T) {
	svc, _, _, _ := newTestService()
	svc.WithMusicTrackChecker(&fakeMusicChecker{ids: map[string]bool{}})

	pl := &playlist.Playlist{Name: "Mix", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	err := svc.AddItemWithType(context.Background(), pl.ID, "missing", "u1", "music")
	if !errors.Is(err, ErrMediaItemNotFound) {
		t.Fatalf("expected ErrMediaItemNotFound, got %v", err)
	}
}

func TestAddItem_DefaultMediaType_IsVideo(t *testing.T) {
	svc, _, itemRepo, _ := newTestService()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	if err := svc.AddItem(context.Background(), pl.ID, "m1", "u1"); err != nil {
		t.Fatalf("add: %v", err)
	}
	items, _ := itemRepo.GetByPlaylistID(context.Background(), pl.ID)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].MediaType != "video" {
		t.Fatalf("MediaType = %q, want video", items[0].MediaType)
	}
}

func TestReorderItems(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Ordered", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)

	svc.AddItem(context.Background(), pl.ID, "a", "user-1")
	svc.AddItem(context.Background(), pl.ID, "b", "user-1")
	svc.AddItem(context.Background(), pl.ID, "c", "user-1")

	if err := svc.ReorderItems(context.Background(), pl.ID, "user-1", []string{"c", "a", "b"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	_, items, _ := svc.GetItems(context.Background(), pl.ID, "user-1")
	if len(items) != 3 {
		t.Fatalf("expected 3, got %d", len(items))
	}
	if items[0].MediaItemID != "c" {
		t.Fatalf("expected c first, got %s", items[0].MediaItemID)
	}
}

// Same contract as collections (decided 2026-08-05): a reorder naming ids the
// playlist does not hold is refused with ErrForeignItems, never silently
// half-applied - the repo UPDATE matches on membership, so a stranger id would
// otherwise change nothing behind a success response.
func TestReorderItems_ForeignIDRejected(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Ordered", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)
	svc.AddItem(context.Background(), pl.ID, "a", "user-1")
	svc.AddItem(context.Background(), pl.ID, "b", "user-1")

	err := svc.ReorderItems(context.Background(), pl.ID, "user-1", []string{"b", "a", "stranger"})
	if !errors.Is(err, ErrForeignItems) {
		t.Fatalf("expected ErrForeignItems, got %v", err)
	}

	_, items, _ := svc.GetItems(context.Background(), pl.ID, "user-1")
	if len(items) != 2 || items[0].MediaItemID != "a" {
		t.Fatalf("order must be untouched after a refused reorder, got %v", items)
	}
}

func TestReorderItems_WrongOwner(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Mine", ProfileID: "user-1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)
	svc.AddItem(context.Background(), pl.ID, "a", "user-1")

	err := svc.ReorderItems(context.Background(), pl.ID, "user-2", []string{"a"})
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
}

func TestDelete_RemovesFromList(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()

	pl := &playlist.Playlist{Name: "Bye", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	if err := svc.Delete(ctx, pl.ID, "u1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err := svc.List(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 playlists after delete, got %d", len(list))
	}
}

func TestUpdate_NameAndDescription(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()

	pl := &playlist.Playlist{Name: "Before", Description: "old", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	updated := &playlist.Playlist{ID: pl.ID, Name: "After", Description: "new"}
	if err := svc.Update(ctx, updated, "u1"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := svc.GetByID(ctx, pl.ID, "u1")
	if got.Name != "After" || got.Description != "new" {
		t.Fatalf("expected 'After'/'new', got %q/%q", got.Name, got.Description)
	}
}

func TestUpdate_WrongOwner(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()

	pl := &playlist.Playlist{Name: "Mine", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	err := svc.Update(ctx, &playlist.Playlist{ID: pl.ID, Name: "Stolen"}, "u2")
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
}

func TestList_MultipleUsers(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()

	svc.Create(ctx, &playlist.Playlist{Name: "A1", ProfileID: "u1", Type: playlist.Manual})
	svc.Create(ctx, &playlist.Playlist{Name: "A2", ProfileID: "u1", Type: playlist.Smart})
	svc.Create(ctx, &playlist.Playlist{Name: "B1", ProfileID: "u2", Type: playlist.Manual})

	u1list, _ := svc.List(ctx, "u1")
	u2list, _ := svc.List(ctx, "u2")
	u3list, _ := svc.List(ctx, "u3")

	if len(u1list) != 2 {
		t.Fatalf("u1: expected 2, got %d", len(u1list))
	}
	if len(u2list) != 1 {
		t.Fatalf("u2: expected 1, got %d", len(u2list))
	}
	if len(u3list) != 0 {
		t.Fatalf("u3: expected 0, got %d", len(u3list))
	}
}

func TestCreate_EmptyNameRejected(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "", ProfileID: "u1", Type: playlist.Manual}
	if err := svc.Create(context.Background(), pl); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreate_InvalidTypeRejected(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.PlaylistType("bogus")}
	if err := svc.Create(context.Background(), pl); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestCreate_DefaultSortOrder(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(context.Background(), pl)
	if pl.SortOrder != "asc" {
		t.Fatalf("SortOrder = %q, want asc", pl.SortOrder)
	}
}

func TestCreate_PreservesExplicitSortFields(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", SortBy: "title", SortOrder: "desc"}
	svc.Create(context.Background(), pl)
	if pl.SortBy != "title" {
		t.Fatalf("SortBy = %q, want title", pl.SortBy)
	}
	if pl.SortOrder != "desc" {
		t.Fatalf("SortOrder = %q, want desc", pl.SortOrder)
	}
}

func TestCreate_SetsTimestamps(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1"}
	svc.Create(context.Background(), pl)
	if pl.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}
	if pl.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt not set")
	}
}

func TestUpdate_PreservesProfileIDAndCreatedAt(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "Orig", ProfileID: "u1", Type: playlist.Smart}
	svc.Create(ctx, pl)
	origCreatedAt := pl.CreatedAt

	upd := &playlist.Playlist{ID: pl.ID, Name: "New", ProfileID: "attacker", Type: playlist.Manual}
	svc.Update(ctx, upd, "u1")

	if upd.ProfileID != "u1" {
		t.Fatalf("ProfileID = %q, want u1 (immutable)", upd.ProfileID)
	}
	if upd.Type != playlist.Smart {
		t.Fatalf("Type = %q, want smart (immutable)", upd.Type)
	}
	if upd.CreatedAt != origCreatedAt {
		t.Fatal("CreatedAt was mutated")
	}
	if upd.UpdatedAt.IsZero() || !upd.UpdatedAt.After(origCreatedAt) {
		t.Fatal("UpdatedAt not refreshed")
	}
}

func TestAddItem_SortOrderIncrements(t *testing.T) {
	svc, _, itemRepo, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	svc.AddItem(ctx, pl.ID, "m1", "u1")
	svc.AddItem(ctx, pl.ID, "m2", "u1")
	svc.AddItem(ctx, pl.ID, "m3", "u1")

	items, _ := itemRepo.GetByPlaylistID(ctx, pl.ID)
	if len(items) != 3 {
		t.Fatalf("len=%d, want 3", len(items))
	}
	for i, it := range items {
		if it.SortOrder != i {
			t.Fatalf("item %d SortOrder=%d, want %d", i, it.SortOrder, i)
		}
	}
}

func TestAddItem_SortOrderAfterGap(t *testing.T) {
	svc, _, itemRepo, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	svc.AddItem(ctx, pl.ID, "m1", "u1")
	svc.AddItem(ctx, pl.ID, "m2", "u1")
	svc.RemoveItem(ctx, pl.ID, "m1", "u1")
	svc.AddItem(ctx, pl.ID, "m3", "u1")

	items, _ := itemRepo.GetByPlaylistID(ctx, pl.ID)
	if len(items) != 2 {
		t.Fatalf("len=%d, want 2", len(items))
	}
	// m2 has SortOrder=1, m3 should get SortOrder=2 (max+1)
	if items[1].SortOrder != 2 {
		t.Fatalf("m3 SortOrder=%d, want 2", items[1].SortOrder)
	}
}

func TestAddItem_GetItemsRepoError(t *testing.T) {
	svc, _, itemRepo, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	itemRepo.Err = errors.New("storage fail")
	err := svc.AddItem(ctx, pl.ID, "m1", "u1")
	if err == nil {
		t.Fatal("expected error from item repo")
	}
}

func TestAddItem_MediaCheckerNonNotFoundError(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	checker := &errorMediaChecker{err: errors.New("db timeout")}
	svc.WithMediaChecker(checker)

	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	err := svc.AddItem(ctx, pl.ID, "m1", "u1")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrMediaItemNotFound) {
		t.Fatal("should not be ErrMediaItemNotFound for non-NotFound errors")
	}
}

func TestRemoveItem_SmartPlaylistRejected(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Smart}
	svc.Create(ctx, pl)

	err := svc.RemoveItem(ctx, pl.ID, "m1", "u1")
	if !errors.Is(err, ErrSmartItems) {
		t.Fatalf("got %v, want ErrSmartItems", err)
	}
}

func TestGetItems_ManualReturnsNilMediaItems(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)
	svc.AddItem(ctx, pl.ID, "m1", "u1")

	mediaItems, plItems, err := svc.GetItems(ctx, pl.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if mediaItems != nil {
		t.Fatal("manual playlist should return nil mediaItems")
	}
	if len(plItems) != 1 {
		t.Fatalf("expected 1 plItem, got %d", len(plItems))
	}
}

func TestGetItems_SmartReturnsNilPlaylistItems(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	env.seed(&library.MediaItem{ID: "m1", Title: "A"})
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Smart}
	env.svc.Create(ctx, pl)

	mediaItems, plItems, err := env.svc.GetItems(ctx, pl.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if plItems != nil {
		t.Fatal("smart playlist should return nil plItems")
	}
	if len(mediaItems) != 1 {
		t.Fatalf("expected 1 mediaItem, got %d", len(mediaItems))
	}
}

func TestGetItems_ItemRepoError(t *testing.T) {
	svc, _, itemRepo, _ := newTestService()
	ctx := context.Background()
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	itemRepo.Err = errors.New("storage fail")
	_, _, err := svc.GetItems(ctx, pl.ID, "u1")
	if err == nil {
		t.Fatal("expected error from item repo")
	}
}

func TestGetItems_SmartQuerierError(t *testing.T) {
	svc, _, _, querier := newTestService()
	ctx := context.Background()
	querier.Err = errors.New("query fail")
	pl := &playlist.Playlist{Name: "x", ProfileID: "u1", Type: playlist.Smart}
	svc.Create(ctx, pl)

	_, _, err := svc.GetItems(ctx, pl.ID, "u1")
	if err == nil {
		t.Fatal("expected error from smart querier")
	}
}

func TestGetItems_SmartPlaylist_QuerierReturnsItems(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()

	env.seed(
		&library.MediaItem{ID: "m1", Title: "Movie A", Type: "movie"},
		&library.MediaItem{ID: "m2", Title: "Movie B", Type: "movie"},
		&library.MediaItem{ID: "m3", Title: "Show C", Type: "show"},
	)

	pl := &playlist.Playlist{
		Name:      "Movies Only",
		ProfileID: "u1",
		Type:      playlist.Smart,
		Rules:     []playlist.Rule{{Field: "type", Operator: "eq", Value: "movie"}},
	}
	env.svc.Create(ctx, pl)

	mediaItems, plItems, err := env.svc.GetItems(ctx, pl.ID, "u1")
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if plItems != nil {
		t.Fatal("smart playlist: expected nil playlist items")
	}
	if len(mediaItems) != 2 {
		t.Fatalf("expected 2 media items, got %d", len(mediaItems))
	}
}
