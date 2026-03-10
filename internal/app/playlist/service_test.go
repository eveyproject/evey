package playlist

import (
	"context"
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
	if err != ErrNotOwner {
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

	if err := svc.RemoveItem(context.Background(), pl.ID, "media-1", "user-1"); err != nil {
		t.Fatalf("remove: %v", err)
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
	if err != ErrSmartItems {
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

	if err := svc.Delete(context.Background(), pl.ID, "user-2"); err != ErrNotOwner {
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

	for i := 0; i < maxItems; i++ {
		if err := svc.AddItem(context.Background(), pl.ID, "media-"+string(rune(i+'A')), "user-1"); err != nil {
			t.Fatalf("add item %d: %v", i, err)
		}
	}

	err := svc.AddItem(context.Background(), pl.ID, "media-overflow", "user-1")
	if err != ErrMaxItems {
		t.Fatalf("expected ErrMaxItems, got %v", err)
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
