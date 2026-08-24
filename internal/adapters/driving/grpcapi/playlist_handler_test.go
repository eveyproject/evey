package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/playlist"
	playlistfakes "github.com/evey-project/evey/internal/domain/playlist/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Wired like production (server.go registers this handler with libQuery AND
// userAccess): a harness that leaves the access ports nil cannot exercise the
// guards that run on every write, and a guard added later then looks like it
// broke the suite when it is the fake that was lying about the shape.
// Returns the media reader and the access grants so a test can seed an item
// and say who may see it.
func newTestPlaylistHandlerWithAccess() (*playlistHandler, *libfakes.FakeMediaItemQueryRepo, *userfakes.FakeLibraryAccessRepository) {
	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	smartQ := playlistfakes.NewFakeSmartPlaylistQuerier()
	mediaReader := libfakes.NewFakeMediaItemQueryRepo()
	svc := appplaylist.NewService(plRepo, itemRepo, smartQ, mediaReader)
	libQuery := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaReader,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccess),
	)
	return &playlistHandler{svc: svc, libQuery: libQuery, userAccess: userAccess}, mediaReader, libAccess
}

func newTestPlaylistHandler() *playlistHandler {
	h, _, _ := newTestPlaylistHandlerWithAccess()
	return h
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
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound (wrong-owner hides existence), got %v", status.Code(err))
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
	h, mediaReader, libAccess := newTestPlaylistHandlerWithAccess()
	// The add guard now asks whether the caller may see this media: say so.
	mediaReader.Seed(&library.MediaItem{ID: "mi-1", Title: "Visible", LibraryID: "lib-ok"})
	libAccess.Grant(context.Background(), "user-1", "lib-ok")
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
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound (wrong-owner hides existence), got %v", status.Code(err))
	}
}

func TestPlaylistAddItem_Owner(t *testing.T) {
	h, mediaReader, libAccess := newTestPlaylistHandlerWithAccess()
	// The add guard now asks whether the caller may see this media: say so.
	mediaReader.Seed(&library.MediaItem{ID: "mi-1", Title: "Visible", LibraryID: "lib-ok"})
	libAccess.Grant(context.Background(), "user-1", "lib-ok")
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
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound (wrong-owner hides existence), got %v", status.Code(err))
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
	h, mediaReader, libAccess := newTestPlaylistHandlerWithAccess()
	// The add guard now asks whether the caller may see this media: say so.
	mediaReader.Seed(&library.MediaItem{ID: "mi-1", Title: "Visible", LibraryID: "lib-ok"})
	mediaReader.Seed(&library.MediaItem{ID: "mi-2", Title: "Visible Two", LibraryID: "lib-ok"})
	libAccess.Grant(context.Background(), "user-1", "lib-ok")
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

// TestPlaylistGetPlaylistItems_FiltersInaccessibleLibrary proves an item whose
// library the caller cannot access is hidden from GetPlaylistItems (a profile
// may have added it before losing access, or before AddItem gated access), so
// the endpoint cannot be used to read back restricted metadata.
func TestPlaylistGetPlaylistItems_FiltersInaccessibleLibrary(t *testing.T) {
	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	smartQ := playlistfakes.NewFakeSmartPlaylistQuerier()
	mediaReader := libfakes.NewFakeMediaItemQueryRepo()
	svc := appplaylist.NewService(plRepo, itemRepo, smartQ, mediaReader)
	// The handler hydrates manual-playlist item metadata through libQuery.
	libQuery := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaReader,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccess),
	)
	h := &playlistHandler{svc: svc, libQuery: libQuery, userAccess: userAccess}

	// Restrict user-2 to lib-ok; seed one item there and one in a library the
	// user cannot see.
	libAccess.Grant(context.Background(), "user-2", "lib-ok")
	mediaReader.Seed(&library.MediaItem{ID: "mi-ok", Title: "Visible", LibraryID: "lib-ok"})
	mediaReader.Seed(&library.MediaItem{ID: "mi-secret", Title: "Hidden", LibraryID: "lib-secret"})

	ctx := ctxWithClaims("user-2", "profile-2", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "Mine", Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if _, addErr := h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-ok"}); addErr != nil {
		t.Fatalf("AddPlaylistItem mi-ok: %v", addErr)
	}
	// mi-secret goes in through the REPOSITORY, not the handler: the write is
	// now guarded (see TestPlaylistAddPlaylistItem_RefusesInaccessibleMedia),
	// but rows added before that guard existed - or before the caller lost
	// access - are still out there. The read filter is what protects those, and
	// that is exactly what this test must keep proving.
	if addErr := itemRepo.Add(context.Background(), &playlist.PlaylistItem{
		PlaylistID: created.Id, MediaItemID: "mi-secret", SortOrder: 99,
	}); addErr != nil {
		t.Fatalf("seed mi-secret directly: %v", addErr)
	}

	resp, err := h.GetPlaylistItems(ctx, &eveyv1.GetPlaylistItemsRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("GetPlaylistItems: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 accessible item, got %d", len(resp.Items))
	}
	if resp.Items[0].MediaItemId != "mi-ok" || resp.Items[0].Title != "Visible" {
		t.Errorf("expected only mi-ok/Visible, got %s/%s", resp.Items[0].MediaItemId, resp.Items[0].Title)
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
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound (wrong-owner hides existence), got %v", status.Code(err))
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
	h, mediaReader, libAccess := newTestPlaylistHandlerWithAccess()
	// The add guard now asks whether the caller may see this media: say so.
	mediaReader.Seed(&library.MediaItem{ID: "mi-1", Title: "Visible", LibraryID: "lib-ok"})
	libAccess.Grant(context.Background(), "user-1", "lib-ok")
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
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound (wrong-owner hides existence), got %v", status.Code(err))
	}
}

func TestPlaylistRemovePlaylistItem_NoAuth(t *testing.T) {
	h := newTestPlaylistHandler()
	_, err := h.RemovePlaylistItem(context.Background(), &eveyv1.RemovePlaylistItemRequest{Id: "pl-1", MediaItemId: "mi-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestPlaylistProfileIDIgnored(t *testing.T) {
	h := newTestPlaylistHandler()
	// Playlists are user-scoped; req.ProfileId is ignored.
	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ListPlaylists(ctx, &eveyv1.ListPlaylistsRequest{ProfileId: "profile-2"})
	if err != nil {
		t.Fatalf("ListPlaylists: %v", err)
	}
	if len(resp.Playlists) != 0 {
		t.Errorf("expected 0 playlists, got %d", len(resp.Playlists))
	}
}

// Smart playlists round-trip through these two converters and nothing exercised
// them: `PLAYLIST_TYPE_SMART` appeared only in the production file. A bug that
// silently downgrades a smart playlist to manual, or drops a rule field, would
// ship with every playlist test green.
func TestPlaylistTypeFromProto(t *testing.T) {
	assert.Equal(t, playlist.Smart, playlistTypeFromProto(eveyv1.PlaylistType_PLAYLIST_TYPE_SMART))
	assert.Equal(t, playlist.Manual, playlistTypeFromProto(eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL))
	// Unknown / unspecified falls back to manual: a playlist whose type we
	// cannot read must not become a rule-driven one by accident.
	assert.Equal(t, playlist.Manual, playlistTypeFromProto(eveyv1.PlaylistType_PLAYLIST_TYPE_UNSPECIFIED))
	assert.Equal(t, playlist.Manual, playlistTypeFromProto(eveyv1.PlaylistType(9999)))
}

func TestPlaylistRulesFromProto(t *testing.T) {
	out := playlistRulesFromProto([]*eveyv1.PlaylistRule{
		{Field: "genre", Operator: "eq", Value: "Drame"},
		{Field: "year", Operator: "gte", Value: "2010"},
	})
	require.Len(t, out, 2)
	assert.Equal(t, playlist.Rule{Field: "genre", Operator: "eq", Value: "Drame"}, out[0])
	assert.Equal(t, playlist.Rule{Field: "year", Operator: "gte", Value: "2010"}, out[1])
}

func TestPlaylistRulesFromProto_EmptyIsNonNil(t *testing.T) {
	// Non-nil so a smart playlist with no rules yet persists as an empty list
	// rather than a null the domain would have to special-case.
	out := playlistRulesFromProto(nil)
	assert.NotNil(t, out)
	assert.Empty(t, out)
}

// A playlist is a read-back surface: the titles it holds are returned as
// metadata (title, year, type). Library access was gated here - the code even
// says so - but the parental filter was not, while every sibling listing
// (collections, feeds, favourites, resume/next-up) applies it.
//
// So a restricted profile could not PLAY a blocked title, and could not open
// its media page, but could still read its name back out of a playlist: one
// built before the restriction, or by another profile on the same account.
// Found 2026-08-08 by sweeping each guard for the path that lacks its twin -
// the motif that has now produced eight defects in this repository.
func TestPlaylistGetItems_HidesParentallyBlockedTitles(t *testing.T) {
	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	smartQ := playlistfakes.NewFakeSmartPlaylistQuerier()
	mediaReader := libfakes.NewFakeMediaItemQueryRepo()
	svc := appplaylist.NewService(plRepo, itemRepo, smartQ, mediaReader)
	libQuery := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaReader,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())

	maxAge := 7
	kidProfile := &user.Profile{
		ID:     "profile-kid",
		UserID: "user-kid",
		Name:   "Kid",
		Parental: &user.ParentalSettings{
			Enabled:     true,
			MaxAge:      &maxAge,
			BlockedTags: []string{"violence"},
		},
	}
	profileRepo := userfakes.NewFakeProfileRepository()
	if err := profileRepo.Create(context.Background(), kidProfile); err != nil {
		t.Fatalf("seed kid profile: %v", err)
	}
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profileRepo,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)
	h := &playlistHandler{
		svc: svc, libQuery: libQuery, userQuery: userQuery,
		userAccess: newPermissiveTestUserAccess(),
	}

	// Both titles carry "US: G" so they pass the age check; only the blocked
	// tag separates them. An item with no resolvable rating is fail-closed for
	// a kid profile, so the visible one needs a rating.
	mediaReader.Seed(&library.MediaItem{
		ID: "mi-safe", LibraryID: "lib-1", Title: "Some Movie", Type: "movie",
		Catalog:        &library.MediaCatalog{Tags: []string{}},
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}},
	})
	mediaReader.Seed(&library.MediaItem{
		ID: "mi-blocked", LibraryID: "lib-1", Title: "Some Blocked Movie", Type: "movie",
		Catalog:        &library.MediaCatalog{Tags: []string{"violence"}},
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}},
	})

	ctx := ctxWithClaims("user-kid", "profile-kid", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "Mine", Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("create playlist: %v", err)
	}
	for _, id := range []string{"mi-safe", "mi-blocked"} {
		_, addErr := h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{
			Id: created.Id, MediaItemId: id,
		})
		if addErr != nil {
			t.Fatalf("add %s: %v", id, addErr)
		}
	}

	resp, err := h.GetPlaylistItems(ctx, &eveyv1.GetPlaylistItemsRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("get playlist items: %v", err)
	}
	for _, it := range resp.Items {
		if it.MediaItemId == "mi-blocked" || it.Title == "Some Blocked Movie" {
			t.Errorf("a parentally blocked title was read back from the playlist: id=%q title=%q", it.MediaItemId, it.Title)
		}
	}
	// The permitted title must still be there - over-filtering would be its own bug.
	var sawSafe bool
	for _, it := range resp.Items {
		if it.MediaItemId == "mi-safe" {
			sawSafe = true
		}
	}
	if !sawSafe {
		t.Error("the permitted title must still be listed")
	}
}

// AddPlaylistItem never asked whether the caller may see the media it is
// adding, while its sibling AddFavorite did (favorite_handler.go, via
// requireMediaAccess). The read side saved face - GetPlaylistItems filters
// inaccessible rows back out - but the write went through, so the row landed
// in the table and the FK on media_item_id turned "add it and see" into an
// existence oracle for any id on the server.
//
// The guard belongs on the WRITE, exactly like favourites: same question,
// same answer, and NotFound rather than PermissionDenied so an inaccessible
// id stays indistinguishable from a missing one.
func TestPlaylistAddPlaylistItem_RefusesInaccessibleMedia(t *testing.T) {
	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	smartQ := playlistfakes.NewFakeSmartPlaylistQuerier()
	mediaReader := libfakes.NewFakeMediaItemQueryRepo()
	svc := appplaylist.NewService(plRepo, itemRepo, smartQ, mediaReader)
	libQuery := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaReader,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccess),
	)
	h := &playlistHandler{svc: svc, libQuery: libQuery, userAccess: userAccess}

	libAccess.Grant(context.Background(), "user-2", "lib-ok")
	mediaReader.Seed(&library.MediaItem{ID: "mi-ok", Title: "Visible", LibraryID: "lib-ok"})
	mediaReader.Seed(&library.MediaItem{ID: "mi-secret", Title: "Hidden", LibraryID: "lib-secret"})

	ctx := ctxWithClaims("user-2", "profile-2", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{
		Name: "Mine", Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL,
	})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if _, addErr := h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-ok"}); addErr != nil {
		t.Fatalf("an accessible item must still be addable: %v", addErr)
	}
	_, addErr := h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-secret"})
	assertCode(t, addErr, codes.NotFound)

	// A completely unknown id must answer the SAME way as an inaccessible one,
	// or the difference is the oracle.
	_, unknownErr := h.AddPlaylistItem(ctx, &eveyv1.AddPlaylistItemRequest{Id: created.Id, MediaItemId: "mi-nope"})
	assertCode(t, unknownErr, codes.NotFound)
}
