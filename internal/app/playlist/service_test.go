package playlist

import (
	"context"
	"errors"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/playlist"
	"github.com/gravity-zero/evey/internal/domain/playlist/fakes"
)

func newTestService() (*Service, *fakes.FakePlaylistRepository, *fakes.FakePlaylistItemRepository, *fakes.FakeSmartPlaylistQuerier) {
	plRepo := fakes.NewFakePlaylistRepository()
	itemRepo := fakes.NewFakePlaylistItemRepository()
	querier := fakes.NewFakeSmartPlaylistQuerier()
	svc := NewService(plRepo, itemRepo, querier)
	return svc, plRepo, itemRepo, querier
}

func TestCreatePlaylist(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{
		Name:   "My Playlist",
		UserID: "user-1",
		Type:   playlist.Manual,
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
	pl := &playlist.Playlist{Name: "Test", UserID: "user-1", Type: playlist.Manual}
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
	svc.Create(context.Background(), &playlist.Playlist{Name: "A", UserID: "user-1", Type: playlist.Manual})
	svc.Create(context.Background(), &playlist.Playlist{Name: "B", UserID: "user-2", Type: playlist.Manual})
	svc.Create(context.Background(), &playlist.Playlist{Name: "C", UserID: "user-1", Type: playlist.Smart})

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
	pl := &playlist.Playlist{Name: "Manual", UserID: "user-1", Type: playlist.Manual}
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
		Name:   "Smart",
		UserID: "user-1",
		Type:   playlist.Smart,
		Rules:  []playlist.Rule{{Field: "genre", Operator: "eq", Value: "Action"}},
	}
	svc.Create(context.Background(), pl)

	err := svc.AddItem(context.Background(), pl.ID, "media-1", "user-1")
	if !errors.Is(err, ErrSmartItems) {
		t.Fatalf("expected ErrSmartItems, got %v", err)
	}
}

func TestSmartPlaylistGetItems(t *testing.T) {
	svc, _, _, querier := newTestService()
	querier.Items = []*library.MediaItem{
		{ID: "m-1", Title: "Action Movie", Genres: []string{"Action"}},
		{ID: "m-2", Title: "Drama Movie", Genres: []string{"Drama"}},
		{ID: "m-3", Title: "Action Drama", Genres: []string{"Action", "Drama"}},
	}

	pl := &playlist.Playlist{
		Name:   "Action",
		UserID: "user-1",
		Type:   playlist.Smart,
		Rules:  []playlist.Rule{{Field: "genre", Operator: "eq", Value: "Action"}},
	}
	svc.Create(context.Background(), pl)

	mediaItems, _, err := svc.GetItems(context.Background(), pl.ID, "user-1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(mediaItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(mediaItems))
	}
}

func TestUpdatePlaylist(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Old", UserID: "user-1", Type: playlist.Manual}
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
	pl := &playlist.Playlist{Name: "Mine", UserID: "user-1", Type: playlist.Manual}
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
	pl := &playlist.Playlist{Name: "Big", UserID: "user-1", Type: playlist.Manual}
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
	return nil, library.ErrNotFound
}

func TestAddItem_InvalidMediaID(t *testing.T) {
	svc, _, _, _ := newTestService()
	checker := &fakeMediaChecker{items: map[string]*library.MediaItem{}}
	svc.WithMediaChecker(checker)

	pl := &playlist.Playlist{Name: "Manual", UserID: "user-1", Type: playlist.Manual}
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

	pl := &playlist.Playlist{Name: "Manual", UserID: "user-1", Type: playlist.Manual}
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

func TestReorderItems(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Ordered", UserID: "user-1", Type: playlist.Manual}
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

func TestReorderItems_WrongOwner(t *testing.T) {
	svc, _, _, _ := newTestService()
	pl := &playlist.Playlist{Name: "Mine", UserID: "user-1", Type: playlist.Manual}
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

	pl := &playlist.Playlist{Name: "Bye", UserID: "u1", Type: playlist.Manual}
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

	pl := &playlist.Playlist{Name: "Before", Description: "old", UserID: "u1", Type: playlist.Manual}
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

	pl := &playlist.Playlist{Name: "Mine", UserID: "u1", Type: playlist.Manual}
	svc.Create(ctx, pl)

	err := svc.Update(ctx, &playlist.Playlist{ID: pl.ID, Name: "Stolen"}, "u2")
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
}

func TestList_MultipleUsers(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()

	svc.Create(ctx, &playlist.Playlist{Name: "A1", UserID: "u1", Type: playlist.Manual})
	svc.Create(ctx, &playlist.Playlist{Name: "A2", UserID: "u1", Type: playlist.Smart})
	svc.Create(ctx, &playlist.Playlist{Name: "B1", UserID: "u2", Type: playlist.Manual})

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

func TestGetItems_SmartPlaylist_QuerierReturnsItems(t *testing.T) {
	svc, _, _, querier := newTestService()
	ctx := context.Background()

	querier.Items = []*library.MediaItem{
		{ID: "m1", Title: "Movie A", Type: "movie"},
		{ID: "m2", Title: "Movie B", Type: "movie"},
		{ID: "m3", Title: "Show C", Type: "show"},
	}

	pl := &playlist.Playlist{
		Name:   "Movies Only",
		UserID: "u1",
		Type:   playlist.Smart,
		Rules:  []playlist.Rule{{Field: "type", Operator: "eq", Value: "movie"}},
	}
	svc.Create(ctx, pl)

	mediaItems, plItems, err := svc.GetItems(ctx, pl.ID, "u1")
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
