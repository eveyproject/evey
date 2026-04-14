package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	playlistfakes "github.com/evey-project/evey/internal/domain/playlist/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestPlaylistHandler() *playlistHandler {
	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	smartQ := playlistfakes.NewFakeSmartPlaylistQuerier()
	svc := appplaylist.NewService(plRepo, itemRepo, smartQ)
	return &playlistHandler{svc: svc}
}

func TestPlaylistListPlaylists_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.ListPlaylists(context.Background(), &eveyv1.ListPlaylistsRequest{})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistListPlaylists_Authenticated(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ListPlaylists(ctx, &eveyv1.ListPlaylistsRequest{})
	if err != nil {
		t.Fatalf("ListPlaylists: %v", err)
	}
	if len(resp.Playlists) != 0 {
		t.Errorf("expected 0 playlists, got %d", len(resp.Playlists))
	}
}

func TestPlaylistCreatePlaylist_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.CreatePlaylist(context.Background(), &eveyv1.CreatePlaylistRequest{Name: "My List"})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistCreatePlaylist_Authenticated(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name:        "Favorites Mix",
		Description: "best tracks",
		Type:        eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if resp.Id == "" {
		t.Error("expected non-empty playlist id")
	}
	if resp.Name != "Favorites Mix" {
		t.Errorf("expected name 'Favorites Mix', got %q", resp.Name)
	}
}

func TestPlaylistGetPlaylist_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.GetPlaylist(context.Background(), &eveyv1.GetPlaylistRequest{Id: "pl-1"})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistGetPlaylist_WrongOwner(t *testing.T) {
	h := newTestPlaylistHandler()
	// Create playlist as user-1/profile-1
	ctxOwner := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctxOwner, &eveyv1.CreatePlaylistRequest{Name: "Mine"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	// Access as user-2/profile-2
	ctxOther := ctxWithClaims("user-2", "profile-2", false)
	_, err = h.GetPlaylist(ctxOther, &eveyv1.GetPlaylistRequest{Id: created.Id})
	if err == nil {
		t.Fatal("expected error for wrong owner")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", status.Code(err))
	}
}

func TestPlaylistGetPlaylist_Owner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{Name: "Mine"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	resp, err := h.GetPlaylist(ctx, &eveyv1.GetPlaylistRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("GetPlaylist: %v", err)
	}
	if resp.Id != created.Id {
		t.Errorf("expected id %q, got %q", created.Id, resp.Id)
	}
}

func TestPlaylistDeletePlaylist_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.DeletePlaylist(context.Background(), &eveyv1.DeletePlaylistRequest{Id: "pl-1"})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistDeletePlaylist_WrongOwner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctxOwner := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctxOwner, &eveyv1.CreatePlaylistRequest{Name: "Mine"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	ctxOther := ctxWithClaims("user-2", "profile-2", false)
	_, err = h.DeletePlaylist(ctxOther, &eveyv1.DeletePlaylistRequest{Id: created.Id})
	if err == nil {
		t.Fatal("expected error for wrong owner")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestPlaylistDeletePlaylist_Owner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{Name: "Mine"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	_, err = h.DeletePlaylist(ctx, &eveyv1.DeletePlaylistRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}
}

func TestPlaylistAddItem_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.AddPlaylistItem(context.Background(), &eveyv1.AddPlaylistItemRequest{
		Id:          "pl-1",
		MediaItemId: "mi-1",
	})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistAddItem_WrongOwner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctxOwner := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctxOwner, &eveyv1.CreatePlaylistRequest{
		Name: "Mine",
		Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	ctxOther := ctxWithClaims("user-2", "profile-2", false)
	_, err = h.AddPlaylistItem(ctxOther, &eveyv1.AddPlaylistItemRequest{
		Id:          created.Id,
		MediaItemId: "mi-1",
	})
	if err == nil {
		t.Fatal("expected error for wrong owner")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestPlaylistAddItem_Owner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "Mine",
		Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	_, err = h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{
		Id:          created.Id,
		MediaItemId: "mi-1",
	})
	if err != nil {
		t.Fatalf("AddPlaylistItem: %v", err)
	}
}

func TestPlaylistOtherProfileID_NonAdminDenied(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	// profile-2 != profile-1, non-admin → PermissionDenied
	_, err := h.ListPlaylists(ctx, &eveyv1.ListPlaylistsRequest{ProfileId: "profile-2"})
	if err == nil {
		t.Fatal("expected error accessing other profile")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestPlaylistUpdatePlaylist_Success(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "Original",
		Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	resp, err := h.UpdatePlaylist(ctx, &eveyv1.UpdatePlaylistRequest{
		Id:   created.Id,
		Name: "Renamed",
	})
	if err != nil {
		t.Fatalf("UpdatePlaylist: %v", err)
	}
	if resp.Name != "Renamed" {
		t.Errorf("expected name 'Renamed', got %q", resp.Name)
	}
}

func TestPlaylistUpdatePlaylist_WrongOwner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctxOwner := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctxOwner, &eveyv1.CreatePlaylistRequest{Name: "Mine"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	ctxOther := ctxWithClaims("user-2", "profile-2", false)
	_, err = h.UpdatePlaylist(ctxOther, &eveyv1.UpdatePlaylistRequest{
		Id:   created.Id,
		Name: "Hacked",
	})
	if err == nil {
		t.Fatal("expected error for wrong owner")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestPlaylistUpdatePlaylist_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.UpdatePlaylist(context.Background(), &eveyv1.UpdatePlaylistRequest{Id: "pl-1", Name: "X"})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistGetPlaylistItems_Success(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "My List",
		Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	_, _ = h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-1"})
	_, _ = h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-2"})

	resp, err := h.GetPlaylistItems(ctx, &eveyv1.GetPlaylistItemsRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("GetPlaylistItems: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestPlaylistGetPlaylistItems_WrongOwner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctxOwner := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctxOwner, &eveyv1.CreatePlaylistRequest{
		Name: "Mine",
		Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	ctxOther := ctxWithClaims("user-2", "profile-2", false)
	_, err = h.GetPlaylistItems(ctxOther, &eveyv1.GetPlaylistItemsRequest{Id: created.Id})
	if err == nil {
		t.Fatal("expected error for wrong owner")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestPlaylistGetPlaylistItems_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.GetPlaylistItems(context.Background(), &eveyv1.GetPlaylistItemsRequest{Id: "pl-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistRemovePlaylistItem_Success(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "My List",
		Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	_, _ = h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-1"})
	_, err = h.RemovePlaylistItem(ctx, &eveyv1.RemovePlaylistItemRequest{Id: created.Id, MediaItemId: "mi-1"})
	if err != nil {
		t.Fatalf("RemovePlaylistItem: %v", err)
	}

	items, _ := h.GetPlaylistItems(ctx, &eveyv1.GetPlaylistItemsRequest{Id: created.Id})
	if len(items.Items) != 0 {
		t.Errorf("expected 0 items after remove, got %d", len(items.Items))
	}
}

func TestPlaylistRemovePlaylistItem_WrongOwner(t *testing.T) {
	h := newTestPlaylistHandler()
	ctxOwner := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctxOwner, &eveyv1.CreatePlaylistRequest{
		Name: "Mine", Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	_, _ = h.AddPlaylistItem(ctxOwner, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-1"})

	ctxOther := ctxWithClaims("user-2", "profile-2", false)
	_, err = h.RemovePlaylistItem(ctxOther, &eveyv1.RemovePlaylistItemRequest{Id: created.Id, MediaItemId: "mi-1"})
	if err == nil {
		t.Fatal("expected error for wrong owner")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestPlaylistRemovePlaylistItem_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.RemovePlaylistItem(context.Background(), &eveyv1.RemovePlaylistItemRequest{Id: "pl-1", MediaItemId: "mi-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistOtherProfileID_AdminAllowed(t *testing.T) {
	h := newTestPlaylistHandler()
	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	// admin can list another profile's playlists
	resp, err := h.ListPlaylists(ctx, &eveyv1.ListPlaylistsRequest{ProfileId: "profile-other"})
	if err != nil {
		t.Fatalf("admin ListPlaylists for other profile: %v", err)
	}
	if len(resp.Playlists) != 0 {
		t.Errorf("expected 0 playlists, got %d", len(resp.Playlists))
	}
}
