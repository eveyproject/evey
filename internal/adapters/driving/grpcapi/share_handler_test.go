package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appmedia "github.com/eveyproject/evey/internal/app/media"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type shareTestEnv struct {
	handler       *shareHandler
	shareRepo     *sharedfakes.FakeShareLinkRepo
	mediaItemRepo *libfakes.FakeMediaItemQueryRepo
	libAccessRepo *userfakes.FakeLibraryAccessRepository
}

func newShareTestEnv() *shareTestEnv {
	shareRepo := sharedfakes.NewFakeShareLinkRepo()
	mediaItemRepo := libfakes.NewFakeMediaItemQueryRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libAccessRepo := userfakes.NewFakeLibraryAccessRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := appmedia.NewShareService(shareRepo)
	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItemRepo, streamRepo, searchIdx)
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		bus,
		appuser.WithLibraryAccess(libAccessRepo),
	)

	d2h, _ := shared.NewDurationMs(7200000)
	mediaItemRepo.Seed(&library.MediaItem{
		ID:        "media-1",
		LibraryID: "lib-1",
		Title:     "Test Movie",
		Type:      "movie",
		File:      &library.MediaFile{Duration: d2h},
	})

	h := &shareHandler{
		svc:        svc,
		libQuery:   libQuery,
		userAccess: userAccess,
	}
	return &shareTestEnv{
		handler:       h,
		shareRepo:     shareRepo,
		mediaItemRepo: mediaItemRepo,
		libAccessRepo: libAccessRepo,
	}
}

func TestShareHandler_CreateShareLink_Success(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "media-1", resp.MediaId)
	assert.Contains(t, resp.Url, "/share/")
	assert.NotNil(t, resp.ExpiresAt)
}

func TestShareHandler_CreateShareLink_WithMaxViews(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	maxViews := int32(5)
	resp, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
		MaxViews:      &maxViews,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.MaxViews)
	assert.Equal(t, int32(5), *resp.MaxViews)
}

func TestShareHandler_CreateShareLink_DefaultDuration(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 0,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
}

func TestShareHandler_CreateShareLink_Unauthenticated(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()

	_, err := env.handler.CreateShareLink(context.Background(), &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestShareHandler_CreateShareLink_MediaNotFound(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "nonexistent",
		DurationHours: 24,
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestShareHandler_CreateShareLink_NoLibraryAccess(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	// Grant access to lib-2 only, media-1 is in lib-1
	env.libAccessRepo.Grant(context.Background(), "user-1", "lib-2")
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestShareHandler_CreateShareLink_AdminBypassesLibraryAccess(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	env.libAccessRepo.Grant(context.Background(), "admin-1", "lib-2")
	ctx := ctxWithClaims("admin-1", "profile-1", true)

	resp, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
}

func TestShareHandler_CreateShareLink_RepoError(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	env.shareRepo.Err = assert.AnError
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestShareHandler_ListShareLinks_Success(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-1", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-2", Token: "tok-2", MediaItemID: "media-1",
		CreatedBy: "user-2", ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	resp, err := env.handler.ListShareLinks(ctx, &eveyv1.ListShareLinksRequest{})

	require.NoError(t, err)
	// Non-admin only sees own links
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "link-1", resp.Items[0].Id)
}

func TestShareHandler_ListShareLinks_WithMaxViews(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	maxViews := 10
	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-1", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
		MaxViews: &maxViews, ViewCount: 3,
	})

	resp, err := env.handler.ListShareLinks(ctx, &eveyv1.ListShareLinksRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, int32(3), resp.Items[0].ViewCount)
	require.NotNil(t, resp.Items[0].MaxViews)
	assert.Equal(t, int32(10), *resp.Items[0].MaxViews)
}

func TestShareHandler_ListShareLinks_RepoError(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	env.shareRepo.Err = assert.AnError
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := env.handler.ListShareLinks(ctx, &eveyv1.ListShareLinksRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestShareHandler_RevokeShareLink_Success(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-1", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, err := env.handler.RevokeShareLink(ctx, &eveyv1.RevokeShareLinkRequest{Id: "link-1"})

	require.NoError(t, err)
	// Verify link is gone
	resp, err := env.handler.ListShareLinks(ctx, &eveyv1.ListShareLinksRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestShareHandler_RevokeShareLink_NotFound(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := env.handler.RevokeShareLink(ctx, &eveyv1.RevokeShareLinkRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// A revoke that fails AFTER the existence + ownership checks must surface as
// Internal, not NotFound: the link still exists and is live, so telling the
// admin "not found" would falsely imply a leaked link is gone.
func TestShareHandler_RevokeShareLink_DeleteFailsIsInternal(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-1", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	env.shareRepo.DeleteErr = assert.AnError

	_, err := env.handler.RevokeShareLink(ctx, &eveyv1.RevokeShareLinkRequest{Id: "link-1"})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestShareHandler_RevokeShareLink_OwnershipCheck(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()

	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-1", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// non-admin user-2 cannot revoke user-1's link; the denial is indistinguishable
	// from a non-existent link (NotFound, not PermissionDenied).
	ctxUser2 := ctxWithClaims("user-2", "profile-2", false)
	_, err := env.handler.RevokeShareLink(ctxUser2, &eveyv1.RevokeShareLinkRequest{Id: "link-1"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	// admin can revoke any link
	ctxAdmin := ctxWithClaims("admin-1", "profile-admin", true)
	_, err = env.handler.RevokeShareLink(ctxAdmin, &eveyv1.RevokeShareLinkRequest{Id: "link-1"})
	require.NoError(t, err)
}

func TestShareHandler_ValidateShareLink_Success(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	createResp, err := env.handler.CreateShareLink(ctx, &eveyv1.CreateShareLinkRequest{
		MediaItemId:   "media-1",
		DurationHours: 24,
	})
	require.NoError(t, err)

	valResp, err := env.handler.ValidateShareLink(ctx, &eveyv1.ValidateShareLinkRequest{
		Token: createResp.Token,
	})

	require.NoError(t, err)
	assert.Equal(t, "media-1", valResp.MediaId)
	assert.Equal(t, "Test Movie", valResp.Title)
	assert.Equal(t, "movie", valResp.MediaType)
	assert.Equal(t, int64(7200000), valResp.DurationMs)
	// Share links now resolve to the public download route, not /stream.
	assert.Equal(t, "/api/v1/share/"+createResp.Token+"/download", valResp.StreamUrl)
}

func TestShareHandler_ValidateShareLink_InvalidToken(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := env.handler.ValidateShareLink(ctx, &eveyv1.ValidateShareLinkRequest{
		Token: "invalid-token",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestShareHandler_ValidateShareLink_Expired(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()

	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-expired", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := env.handler.ValidateShareLink(ctx, &eveyv1.ValidateShareLinkRequest{
		Token: "tok-expired",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestShareHandler_ValidateShareLink_ViewLimitReached(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()

	maxViews := 1
	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-limited", MediaItemID: "media-1",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
		MaxViews: &maxViews, ViewCount: 1,
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := env.handler.ValidateShareLink(ctx, &eveyv1.ValidateShareLinkRequest{
		Token: "tok-limited",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestShareHandler_ValidateShareLink_MediaDeleted(t *testing.T) {
	t.Parallel()
	env := newShareTestEnv()

	env.shareRepo.Seed(&shared.ShareLink{
		ID: "link-1", Token: "tok-orphan", MediaItemID: "deleted-media",
		CreatedBy: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := env.handler.ValidateShareLink(ctx, &eveyv1.ValidateShareLinkRequest{
		Token: "tok-orphan",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}
