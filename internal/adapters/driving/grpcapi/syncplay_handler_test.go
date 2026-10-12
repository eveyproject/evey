package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appplaylist "github.com/eveyproject/evey/internal/app/playlist"
	appsyncplay "github.com/eveyproject/evey/internal/app/syncplay"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/playlist"
	plfakes "github.com/eveyproject/evey/internal/domain/playlist/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	syncfakes "github.com/eveyproject/evey/internal/domain/syncplay/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type syncplayTestEnv struct {
	handler       *syncplayHandler
	roomRepo      *syncfakes.FakeRoomRepository
	mediaItemRepo *libfakes.FakeMediaItemQueryRepo
	libAccessRepo *userfakes.FakeLibraryAccessRepository
	playlistRepo  *plfakes.FakePlaylistRepository
	playlistItems *plfakes.FakePlaylistItemRepository
	profiles      *userfakes.FakeProfileRepository
}

func newSyncplayTestEnv() *syncplayTestEnv {
	roomRepo := syncfakes.NewFakeRoomRepository()
	hub := appsyncplay.NewHub(roomRepo)

	mediaItemRepo := libfakes.NewFakeMediaItemQueryRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItemRepo, streamRepo, searchIdx)

	libAccessRepo := userfakes.NewFakeLibraryAccessRepository()
	bus := sharedfakes.NewFakeEventBus()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		bus,
		appuser.WithLibraryAccess(libAccessRepo),
	)

	playlistRepo := plfakes.NewFakePlaylistRepository()
	playlistItems := plfakes.NewFakePlaylistItemRepository()
	smart := plfakes.NewFakeSmartPlaylistQuerier()
	playlistSvc := appplaylist.NewService(playlistRepo, playlistItems, smart, mediaItemRepo)

	profiles := userfakes.NewFakeProfileRepository()
	// Seed the plain profiles the tests authenticate as. Real claims always name
	// a profile that exists; the parental resolver fails CLOSED on one it can't
	// load, so an unseeded repo would block every room for the wrong reason.
	for _, p := range []struct{ id, user string }{{"p-1", "u-1"}, {"p-2", "u-2"}, {"p-3", "u-3"}} {
		_ = profiles.Create(context.Background(), &user.Profile{ID: shared.ProfileID(p.id), UserID: shared.UserID(p.user), Name: p.id})
	}
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)

	h := &syncplayHandler{
		hub:         hub,
		libQuery:    libQuery,
		userAccess:  userAccess,
		userQuery:   userQuery,
		playlistSvc: playlistSvc,
	}
	return &syncplayTestEnv{
		handler:       h,
		roomRepo:      roomRepo,
		mediaItemRepo: mediaItemRepo,
		libAccessRepo: libAccessRepo,
		playlistRepo:  playlistRepo,
		playlistItems: playlistItems,
		profiles:      profiles,
	}
}

func (e *syncplayTestEnv) seedPlaylist(plID, profileID string, trackIDs []string) {
	pl := &playlist.Playlist{
		ID:        plID,
		Name:      "My PL",
		ProfileID: shared.ProfileID(profileID),
		Type:      playlist.Manual,
	}
	_ = e.playlistRepo.Create(context.Background(), pl)
	for i, tid := range trackIDs {
		_ = e.playlistItems.Add(context.Background(), &playlist.PlaylistItem{
			PlaylistID:  plID,
			MediaItemID: shared.MediaItemID(tid),
			SortOrder:   i,
			AddedAt:     time.Now(),
		})
	}
}

func (e *syncplayTestEnv) seedMedia(id, libraryID string) {
	e.mediaItemRepo.Seed(&library.MediaItem{ID: shared.MediaItemID(id), LibraryID: shared.LibraryID(libraryID), Title: "Test"})
}

func (e *syncplayTestEnv) grantAccess(userID, libraryID string) {
	_ = e.libAccessRepo.Grant(context.Background(), shared.UserID(userID), shared.LibraryID(libraryID))
}

// --- CreateRoom ---

func TestCreateRoom_Success(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		MediaId: "media-1",
		Name:    "My Room",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.NotEmpty(t, resp.Code)
	assert.Equal(t, "My Room", resp.Name)
	assert.Equal(t, "media-1", resp.MediaId)
	assert.Equal(t, "u-1", resp.HostId)
}

// The creator, signed in on a profile, must be admitted to their own room's
// websocket (room.IsMember(claims.UserID)) and hold its host authority (the hub
// compares the sender's user id): hosting as the profile failed both.
func TestCreateRoom_CreatorOnAProfileHostsTheRoom(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")

	resp, err := env.handler.CreateRoom(ctxWithClaims("u-1", "p-1", false), &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	room, err := env.handler.hub.GetRoom(context.Background(), resp.Id)
	require.NoError(t, err)
	assert.True(t, room.IsMember("u-1"), "the websocket admits the creator's user id")
	assert.True(t, room.IsHost("u-1"), "the hub's host authority is the creator's user id")
}

func TestCreateRoom_DefaultName(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		MediaId: "media-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "Watch Together", resp.Name)
}

func TestCreateRoom_Unauthenticated(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	resp, err := env.handler.CreateRoom(context.Background(), &eveyv1.CreateSyncPlayRoomRequest{
		MediaId: "media-1",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCreateRoom_MissingMediaID(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateRoom_NoMediaAccess(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	// grant access to a different library so the user has restrictions
	env.grantAccess("u-1", "lib-other")

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		MediaId: "media-1",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCreateRoom_AdminBypassesAccess(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")

	ctx := ctxWithClaims("u-1", "p-1", true)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		MediaId: "media-1",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

// --- JoinRoom ---

func TestJoinRoom_Success(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.grantAccess("u-2", "lib-1")

	ctx1 := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx1, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	ctx2 := ctxWithClaims("u-2", "p-2", false)
	resp, err := env.handler.JoinRoom(ctx2, &eveyv1.JoinSyncPlayRoomRequest{RoomId: created.Id})

	require.NoError(t, err)
	assert.Equal(t, created.Id, resp.RoomId)
	assert.Contains(t, resp.WsUrl, created.Id)
}

func TestJoinRoom_Unauthenticated(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	resp, err := env.handler.JoinRoom(context.Background(), &eveyv1.JoinSyncPlayRoomRequest{RoomId: "room-1"})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestJoinRoom_NotFound(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.JoinRoom(ctx, &eveyv1.JoinSyncPlayRoomRequest{RoomId: "nonexistent"})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- JoinByCode ---

func TestJoinByCode_Success(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.grantAccess("u-2", "lib-1")

	ctx1 := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx1, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	ctx2 := ctxWithClaims("u-2", "p-2", false)
	resp, err := env.handler.JoinByCode(ctx2, &eveyv1.JoinSyncPlayByCodeRequest{Code: created.Code})

	require.NoError(t, err)
	assert.Equal(t, created.Id, resp.RoomId)
	assert.Equal(t, "media-1", resp.MediaId)
	assert.Contains(t, resp.WsUrl, created.Id)
}

func TestJoinByCode_EmptyCode(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.JoinByCode(ctx, &eveyv1.JoinSyncPlayByCodeRequest{})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestJoinByCode_InvalidCode(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	ctx := ctxWithClaims("u-1", "p-1", false)
	resp, err := env.handler.JoinByCode(ctx, &eveyv1.JoinSyncPlayByCodeRequest{Code: "99999"})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- GetRoom ---

func TestGetRoom_AsHost(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")

	ctx := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1", Name: "Room1"})
	require.NoError(t, err)

	resp, err := env.handler.GetRoom(ctx, &eveyv1.GetSyncPlayRoomRequest{RoomId: created.Id})

	require.NoError(t, err)
	assert.Equal(t, created.Id, resp.Id)
	assert.Equal(t, "Room1", resp.Name)
	assert.NotEmpty(t, resp.Code) // host sees code
}

func TestGetRoom_AsMember(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.grantAccess("u-2", "lib-1")

	ctx1 := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx1, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	ctx2 := ctxWithClaims("u-2", "p-2", false)
	_, err = env.handler.JoinRoom(ctx2, &eveyv1.JoinSyncPlayRoomRequest{RoomId: created.Id})
	require.NoError(t, err)

	resp, err := env.handler.GetRoom(ctx2, &eveyv1.GetSyncPlayRoomRequest{RoomId: created.Id})

	require.NoError(t, err)
	assert.Equal(t, created.Id, resp.Id)
	assert.Empty(t, resp.Code) // non-host doesn't see code
}

func TestGetRoom_NonMemberGetsNotFound(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.grantAccess("u-3", "lib-1")

	ctx1 := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx1, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	// u-3 has access to the room's media but is not a member: the room must look
	// non-existent (NotFound), not "forbidden" (which would confirm it exists).
	ctx3 := ctxWithClaims("u-3", "p-3", false)
	resp, err := env.handler.GetRoom(ctx3, &eveyv1.GetSyncPlayRoomRequest{RoomId: created.Id})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- LeaveRoom ---

func TestLeaveRoom_Success(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.grantAccess("u-2", "lib-1")

	ctx1 := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx1, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	ctx2 := ctxWithClaims("u-2", "p-2", false)
	_, err = env.handler.JoinRoom(ctx2, &eveyv1.JoinSyncPlayRoomRequest{RoomId: created.Id})
	require.NoError(t, err)

	resp, err := env.handler.LeaveRoom(ctx2, &eveyv1.LeaveSyncPlayRoomRequest{RoomId: created.Id})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestLeaveRoom_Unauthenticated(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()

	resp, err := env.handler.LeaveRoom(context.Background(), &eveyv1.LeaveSyncPlayRoomRequest{RoomId: "room-1"})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- CreateRoom from playlist ---

func TestCreateRoom_FromPlaylist_Success(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("t1", "lib-1")
	env.seedMedia("t2", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.seedPlaylist("pl-1", "p-1", []string{"t1", "t2"})
	// Use a music track service stub: requireMediaAccess hits musicSvc when media_type=music
	// musicSvc is nil here - so set up. requireMediaAccess for music with nil musicSvc returns NotFound.
	// Workaround: admin bypass.

	ctx := ctxWithClaims("u-1", "p-1", true)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		PlaylistId: "pl-1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, "music", resp.MediaType)
	assert.Equal(t, "t1", resp.MediaId)
	assert.Equal(t, "My PL", resp.Name)

	// verify queue persisted
	getResp, err := env.handler.GetRoom(ctx, &eveyv1.GetSyncPlayRoomRequest{RoomId: resp.Id})
	require.NoError(t, err)
	assert.Equal(t, []string{"t1", "t2"}, getResp.Queue)
	assert.Equal(t, int32(0), getResp.TrackIndex)
	assert.Equal(t, "pl-1", getResp.PlaylistId)
}

func TestCreateRoom_FromPlaylist_NotFound(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	ctx := ctxWithClaims("u-1", "p-1", true)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		PlaylistId: "pl-missing",
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCreateRoom_FromPlaylist_Empty(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedPlaylist("pl-1", "p-1", nil)
	ctx := ctxWithClaims("u-1", "p-1", true)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		PlaylistId: "pl-1",
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestCreateRoom_FromQueue_Success(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("t1", "lib-1")
	env.seedMedia("t2", "lib-1")
	env.seedMedia("t3", "lib-1")
	ctx := ctxWithClaims("u-1", "p-1", true) // admin bypass for music access (nil musicSvc)

	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		Queue: []string{"t1", "t2", "t3"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, "music", resp.MediaType)
	assert.Equal(t, "t1", resp.MediaId)

	getResp, err := env.handler.GetRoom(ctx, &eveyv1.GetSyncPlayRoomRequest{RoomId: resp.Id})
	require.NoError(t, err)
	assert.Equal(t, []string{"t1", "t2", "t3"}, getResp.Queue)
	assert.Equal(t, int32(0), getResp.TrackIndex)
	assert.Empty(t, getResp.PlaylistId, "a custom queue is not bound to a playlist")
}

func TestCreateRoom_FromQueue_DedupsAndRejectsEmpty(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("t1", "lib-1")
	env.seedMedia("t2", "lib-1")
	ctx := ctxWithClaims("u-1", "p-1", true)

	// Duplicates and blanks collapse; first-seen order is preserved.
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		Queue: []string{"t1", "", "t2", "t1"},
	})
	require.NoError(t, err)
	getResp, err := env.handler.GetRoom(ctx, &eveyv1.GetSyncPlayRoomRequest{RoomId: resp.Id})
	require.NoError(t, err)
	assert.Equal(t, []string{"t1", "t2"}, getResp.Queue)

	// A queue with no usable IDs is rejected.
	_, err = env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		Queue: []string{"", ""},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateRoom_FromPlaylist_NotOwner(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("t1", "lib-1")
	env.seedPlaylist("pl-1", "p-other", []string{"t1"})
	ctx := ctxWithClaims("u-1", "p-1", true)
	resp, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{
		PlaylistId: "pl-1",
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// A kids profile must not be able to open a SyncPlay room on media its own
// parental settings forbid. Without this gate the room becomes a laundering
// step: the public join-code route mints a 24 h guest token that carries NO
// profile, and a guest token with an empty profile makes the parental gate
// no-op on every stream route - so the restricted media becomes watchable, by
// the child and by anyone they hand the code to.
func TestCreateRoom_ParentalBlocked(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.mediaItemRepo.Seed(&library.MediaItem{
		ID: "media-x", LibraryID: "lib-1", Title: "Scary",
		Catalog: &library.MediaCatalog{Tags: []string{"horror"}},
	})
	env.grantAccess("u-1", "lib-1")
	_ = env.profiles.Create(context.Background(), &user.Profile{
		ID: "p-kid", UserID: "u-1", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{Enabled: true, BlockedTags: []string{"horror"}},
	})

	ctx := ctxWithClaims("u-1", "p-kid", false)
	_, err := env.handler.CreateRoom(ctx, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-x"})

	assert.Equal(t, codes.NotFound, status.Code(err),
		"a kids profile must not open a room on media its parental settings block")
}
