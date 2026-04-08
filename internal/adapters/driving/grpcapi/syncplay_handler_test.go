package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	syncfakes "github.com/gravity-zero/evey/internal/domain/syncplay/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
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

	h := &syncplayHandler{
		hub:        hub,
		libQuery:   libQuery,
		userAccess: userAccess,
	}
	return &syncplayTestEnv{
		handler:       h,
		roomRepo:      roomRepo,
		mediaItemRepo: mediaItemRepo,
		libAccessRepo: libAccessRepo,
	}
}

func (e *syncplayTestEnv) seedMedia(id, libraryID string) {
	e.mediaItemRepo.Seed(&library.MediaItem{ID: id, LibraryID: libraryID, Title: "Test"})
}

func (e *syncplayTestEnv) grantAccess(userID, libraryID string) {
	_ = e.libAccessRepo.Grant(context.Background(), userID, libraryID)
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
	assert.Equal(t, "p-1", resp.HostId)
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

func TestGetRoom_Forbidden(t *testing.T) {
	t.Parallel()
	env := newSyncplayTestEnv()
	env.seedMedia("media-1", "lib-1")
	env.grantAccess("u-1", "lib-1")
	env.grantAccess("u-3", "lib-1")

	ctx1 := ctxWithClaims("u-1", "p-1", false)
	created, err := env.handler.CreateRoom(ctx1, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "media-1"})
	require.NoError(t, err)

	ctx3 := ctxWithClaims("u-3", "p-3", false)
	resp, err := env.handler.GetRoom(ctx3, &eveyv1.GetSyncPlayRoomRequest{RoomId: created.Id})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
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
