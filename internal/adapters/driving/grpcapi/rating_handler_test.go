package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domainrating "github.com/evey-project/evey/internal/domain/rating"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeStarRatingRepo implements domainrating.Repository in-memory
type fakeStarRatingRepo struct {
	ratings map[string]*domainrating.Rating // key: mediaItemID+":"+profileID
}

func newFakeStarRatingRepo() *fakeStarRatingRepo {
	return &fakeStarRatingRepo{ratings: make(map[string]*domainrating.Rating)}
}

func (r *fakeStarRatingRepo) Upsert(_ context.Context, rt domainrating.Rating) error {
	r.ratings[string(rt.MediaItemID)+":"+string(rt.ProfileID)] = &rt
	return nil
}

func (r *fakeStarRatingRepo) Get(_ context.Context, mediaItemID, profileID string) (*domainrating.Rating, error) {
	rt, ok := r.ratings[mediaItemID+":"+profileID]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return rt, nil
}

func (r *fakeStarRatingRepo) Delete(_ context.Context, mediaItemID, profileID string) error {
	delete(r.ratings, mediaItemID+":"+profileID)
	return nil
}

func (r *fakeStarRatingRepo) GetAggregates(_ context.Context, mediaItemIDs []string) (map[string]domainrating.Aggregate, error) {
	result := make(map[string]domainrating.Aggregate)
	for _, id := range mediaItemIDs {
		var sum, count int
		for key, rt := range r.ratings {
			if len(key) > len(id) && key[:len(id)+1] == id+":" {
				sum += rt.Value.Int()
				count++
			}
		}
		if count > 0 {
			result[id] = domainrating.Aggregate{Average: float64(sum) / float64(count), Count: count}
		} else {
			result[id] = domainrating.Aggregate{}
		}
	}
	return result, nil
}

type ratingFixture struct {
	handler       *ratingHandler
	thumbRepo     *libfakes.FakeUserRatingRepository
	starRepo      *fakeStarRatingRepo
	mediaItems    *libfakes.FakeMediaItemQueryRepo
	libraryAccess *userfakes.FakeLibraryAccessRepository
}

func newRatingFixture() *ratingFixture {
	thumbRepo := libfakes.NewFakeUserRatingRepository()
	starRepo := newFakeStarRatingRepo()
	bus := sharedfakes.NewFakeEventBus()
	mediaItems := libfakes.NewFakeMediaItemQueryRepo()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()

	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItems, nil, searchIdx)
	starSvc := apprating.NewService(starRepo, bus, apprating.WithThumbRating(thumbRepo))
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
		starRepo:      starRepo,
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

func TestPostStarRating_MissingMediaWinsOverInvalidValue(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	ctx := authedCtx("u1", "p1", true)

	_, err := f.handler.PostStarRating(ctx, &eveyv1.PostStarRatingRequest{MediaItemId: "missing", Value: 0})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

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

// --- PostStarRating ---

func TestPostStarRating_Success(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	resp, err := f.handler.PostStarRating(ctx, &eveyv1.PostStarRatingRequest{MediaItemId: "m1", Value: 7})
	require.NoError(t, err)
	assert.Equal(t, int32(7), resp.Value)
}

// --- GetStarRating ---

func TestGetStarRating_Success(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	_, _ = f.handler.PostStarRating(ctx, &eveyv1.PostStarRatingRequest{MediaItemId: "m1", Value: 8})

	resp, err := f.handler.GetStarRating(ctx, &eveyv1.GetStarRatingRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.Equal(t, int32(8), resp.Value)
}

func TestGetStarRating_NotFound(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib1")
	ctx := authedCtx("u1", "p1", true)

	resp, err := f.handler.GetStarRating(ctx, &eveyv1.GetStarRatingRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Value)
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

func TestPostStarRating_IDOR_NoAccess(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib-private")
	_ = f.libraryAccess.Grant(context.Background(), "u1", "lib-other")
	ctx := authedCtx("u1", "p1", false)

	_, err := f.handler.PostStarRating(ctx, &eveyv1.PostStarRatingRequest{MediaItemId: "m1", Value: 5})
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
