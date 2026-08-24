package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ratingFixture struct {
	handler       *ratingHandler
	thumbRepo     *libfakes.FakeUserRatingRepository
	mediaItems    *libfakes.FakeMediaItemQueryRepo
	libraryAccess *userfakes.FakeLibraryAccessRepository
}

func newRatingFixture() *ratingFixture {
	thumbRepo := libfakes.NewFakeUserRatingRepository()
	bus := sharedfakes.NewFakeEventBus()
	mediaItems := libfakes.NewFakeMediaItemQueryRepo()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()

	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItems, nil, searchIdx)
	starSvc := apprating.NewService(thumbRepo)
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		bus,
		appuser.WithLibraryAccess(libraryAccess),
	)

	h := &ratingHandler{
		starSvc:    starSvc,
		libQuery:   libQuery,
		userAccess: userAccess,
	}
	return &ratingFixture{
		handler:       h,
		thumbRepo:     thumbRepo,
		mediaItems:    mediaItems,
		libraryAccess: libraryAccess,
	}
}

func authedCtx(userID, profileID string, isAdmin bool) context.Context {
	return interceptors.ContextWithClaims(context.Background(), &domainuser.Claims{
		UserID:    userID,
		ProfileID: profileID,
		IsAdmin:   isAdmin,
	})
}

func (f *ratingFixture) seedMedia(id, libraryID string) {
	f.mediaItems.Seed(&library.MediaItem{ID: shared.MediaItemID(id), LibraryID: shared.LibraryID(libraryID)})
}

// --- PutThumbRating ---

func TestPutThumbRating_Success(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	resp, err := f.handler.PutThumbRating(ctx, &eveyv1.PutThumbRatingRequest{MediaItemId: "m1", Value: 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Value)
}

func TestPutThumbRating_Unauthenticated(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()

	_, err := f.handler.PutThumbRating(context.Background(), &eveyv1.PutThumbRatingRequest{MediaItemId: "m1", Value: 1})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestPutThumbRating_InvalidValue(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	_, err := f.handler.PutThumbRating(ctx, &eveyv1.PutThumbRatingRequest{MediaItemId: "m1", Value: 5})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestPutThumbRating_MissingMediaWinsOverInvalidValue(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	ctx := authedCtx("u1", "p1", true)

	_, err := f.handler.PutThumbRating(ctx, &eveyv1.PutThumbRatingRequest{MediaItemId: "missing", Value: 5})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

// (PostStarRating/GetStarRating RPCs were removed when the star UI was
// ripped. Tests deleted; only thumbs up/down remain - covered above.)

// --- GetMyThumbRating ---

func TestGetMyThumbRating_Success(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	_, _ = f.handler.PutThumbRating(ctx, &eveyv1.PutThumbRatingRequest{MediaItemId: "m1", Value: -1})

	resp, err := f.handler.GetMyThumbRating(ctx, &eveyv1.GetMyThumbRatingRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.Equal(t, int32(-1), resp.Value)
}

func TestGetMyThumbRating_NotFound(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	resp, err := f.handler.GetMyThumbRating(ctx, &eveyv1.GetMyThumbRatingRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Value)
}

// The repository already separates the two answers - shared.ErrNotFound for
// "this profile has not rated it", anything else for "the read failed" - and
// the handler collapsed them into Value: 0. A neutral thumb is a STATEMENT
// about what the viewer chose; on a failed read the server does not know what
// they chose, and a client that then writes a rating overwrites an opinion it
// never saw. Logged at Debug on top, which is invisible in production.
func TestGetMyThumbRating_ReadErrorIsNotANeutralThumb(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	f.thumbRepo.Err = errors.New("db down")
	ctx := authedCtx("u1", "p1", true)

	resp, err := f.handler.GetMyThumbRating(ctx, &eveyv1.GetMyThumbRatingRequest{MediaItemId: "m1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("want Internal when the rating cannot be read, got err=%v resp=%v", err, resp)
	}
}

// --- IDOR: non-admin without library access ---

func TestPutThumbRating_IDOR_NoAccess(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib-private")
	// Grant user access to a DIFFERENT library
	_ = f.libraryAccess.Grant(context.Background(), "u1", "lib-other")
	ctx := authedCtx("u1", "p1", false)

	_, err := f.handler.PutThumbRating(ctx, &eveyv1.PutThumbRatingRequest{MediaItemId: "m1", Value: 1})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestGetMyThumbRating_IDOR_NoAccess(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib-private")
	_ = f.libraryAccess.Grant(context.Background(), "u1", "lib-other")
	ctx := authedCtx("u1", "p1", false)

	_, err := f.handler.GetMyThumbRating(ctx, &eveyv1.GetMyThumbRatingRequest{MediaItemId: "m1"})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

// --- Admin bypasses library access ---

func TestPutThumbRating_AdminBypassesAccessCheck(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib-private")
	_ = f.libraryAccess.Grant(context.Background(), "admin1", "lib-other")
	ctx := authedCtx("admin1", "p1", true)

	resp, err := f.handler.PutThumbRating(ctx, &eveyv1.PutThumbRatingRequest{MediaItemId: "m1", Value: 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Value)
}
