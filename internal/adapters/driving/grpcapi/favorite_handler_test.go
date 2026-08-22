package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/favorite"
	favfakes "github.com/evey-project/evey/internal/domain/favorite/fakes"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestFavoriteHandler wires a permissive userAccess (see
// newPermissiveTestUserAccess) plus a libQuery seeded with the media item ids
// used across this file's tests. requireMediaAccess now needs both to resolve
// - unlike the old userAccess==nil fail-open guard, it must look the item up
// to run HasLibraryAccess, so a nil libQuery would panic instead of no-op.
func newTestFavoriteHandler() (*favoriteHandler, *favfakes.FakeRepository) {
	repo := favfakes.NewFakeRepository()
	svc := appfavorite.NewService(repo)
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	for _, id := range []string{"m1", "m2", "nonexistent"} {
		mediaQueryRepo.Seed(&library.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib-1", Title: id, Type: "movie"})
	}
	libQuerySvc := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaQueryRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	return &favoriteHandler{svc: svc, libQuery: libQuerySvc, userAccess: newPermissiveTestUserAccess()}, repo
}

func TestFavoriteHandler_ListFavorites_Unauthenticated(t *testing.T) {
	h, _ := newTestFavoriteHandler()

	_, err := h.ListFavorites(context.Background(), &eveyv1.ListFavoritesRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFavoriteHandler_ListFavorites_Empty(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})

	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

// AddFavorite is idempotent: calling it twice keeps the item favorited (it must
// NOT toggle back off - that was a real bug, a double-tap silently un-favorited).
func TestFavoriteHandler_AddFavorite_Idempotent(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.True(t, resp.IsFavorite)

	// Second add must stay favorited, not flip off.
	resp2, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.True(t, resp2.IsFavorite, "AddFavorite must be idempotent, not a toggle")

	// And the item is still in the list exactly once.
	list, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})
	require.NoError(t, err)
	assert.Len(t, list.Items, 1)
}

// TestFavoriteHandler_AddFavorite_UserAccessUnwired_Denies proves the
// hardening: a handler built with userAccess left nil (a wiring regression,
// never true in production since wire.go always constructs it) must deny a
// non-admin caller, not silently allow the write like the old
// userAccess==nil fail-open guard did.
func TestFavoriteHandler_AddFavorite_UserAccessUnwired_Denies(t *testing.T) {
	repo := favfakes.NewFakeRepository()
	svc := appfavorite.NewService(repo)
	h := &favoriteHandler{svc: svc} // userAccess intentionally left nil
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestFavoriteHandler_AddFavorite_Unauthenticated(t *testing.T) {
	h, _ := newTestFavoriteHandler()

	_, err := h.AddFavorite(context.Background(), &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFavoriteHandler_AddFavorite_RepoError(t *testing.T) {
	h, repo := newTestFavoriteHandler()
	repo.Err = errors.New("db error")
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestFavoriteHandler_ListFavorites_AccessErrorFailsClosed(t *testing.T) {
	// A DB error while reading library access must fail CLOSED (return the
	// error), never fall through to "unrestricted" - else a non-admin would
	// see the metadata of favorites in libraries they can't access.
	repo := favfakes.NewFakeRepository()
	svc := appfavorite.NewService(repo)
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	libAccess.Err = errors.New("db down")
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccess),
	)
	h := &favoriteHandler{svc: svc, userAccess: userAccess}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	require.NoError(t, repo.Add(ctx, &favorite.Favorite{ID: "f1", ProfileID: "profile-1", MediaItemID: "m1"}))

	_, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("want Internal (fail-closed) on access-read error, got %v", err)
	}
}

func TestFavoriteHandler_ListFavorites_WithItems(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	_, err = h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m2"})
	require.NoError(t, err)

	resp, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

// A kids profile must not see adult favorites' metadata, and (the wiring fix)
// kid-safe favorites must now resolve their title - the handler used to be
// constructed without libQuery, so titles never appeared at all.
func TestFavoriteHandler_ListFavorites_ParentalFiltersAdult(t *testing.T) {
	repo := favfakes.NewFakeRepository()
	svc := appfavorite.NewService(repo)
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	libQuerySvc := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaQueryRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	profileRepo := userfakes.NewFakeProfileRepository()
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profileRepo,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository())
	h := &favoriteHandler{svc: svc, libQuery: libQuerySvc, userQuery: userQuery}

	ctx := context.Background()
	maxAge := 13
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{
		ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}}))
	require.NoError(t, repo.Add(ctx, &favorite.Favorite{ID: "f-r", ProfileID: "profile-2", MediaItemID: "mi-r"}))
	require.NoError(t, repo.Add(ctx, &favorite.Favorite{ID: "f-pg", ProfileID: "profile-2", MediaItemID: "mi-pg"}))
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}}) // age 17 → blocked
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-pg", Title: "Family", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}}) // age 10 → allowed

	resp, err := h.ListFavorites(ctxWithClaims("user-2", "profile-2", false), &eveyv1.ListFavoritesRequest{})
	require.NoError(t, err)
	titleByID := make(map[string]string, len(resp.Items))
	for _, it := range resp.Items {
		titleByID[it.MediaItemId] = it.Title
	}
	_, hasAdult := titleByID["mi-r"]
	assert.False(t, hasAdult, "adult favorite must be hidden from a kids profile (metadata leak)")
	assert.Equal(t, "Family", titleByID["mi-pg"], "kid-safe favorite kept AND its title now resolves (the wiring fix)")
}

func TestFavoriteHandler_RemoveFavorite_NotFavorited(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.RemoveFavorite(ctx, &eveyv1.RemoveFavoriteRequest{MediaItemId: "nonexistent"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestFavoriteHandler_RemoveFavorite_Success(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	_, err = h.RemoveFavorite(ctx, &eveyv1.RemoveFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	list, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})
	require.NoError(t, err)
	assert.Empty(t, list.Items)
}

func TestFavoriteHandler_IsFavorite_False(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.IsFavorite(ctx, &eveyv1.IsFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.False(t, resp.IsFavorite)
}

func TestFavoriteHandler_IsFavorite_True(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	resp, err := h.IsFavorite(ctx, &eveyv1.IsFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.True(t, resp.IsFavorite)
}

func TestFavoriteHandler_IsFavorite_Unauthenticated(t *testing.T) {
	h, _ := newTestFavoriteHandler()

	_, err := h.IsFavorite(context.Background(), &eveyv1.IsFavoriteRequest{MediaItemId: "m1"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// IDOR regression guard - non-admin must not operate on another profile.
func TestFavoriteHandler_ListFavorites_IDORAnotherProfile(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestFavoriteHandler_AddFavorite_IDORAnotherProfile(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1", ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestFavoriteHandler_RemoveFavorite_IDORAnotherProfile(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.RemoveFavorite(ctx, &eveyv1.RemoveFavoriteRequest{MediaItemId: "m1", ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestFavoriteHandler_AdminDeniedOnAnotherProfile(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	// Admins are no longer special-cased on profile-scoped RPCs: a foreign
	// profile_id on a normal request is denied even for an admin. Cross-profile
	// admin access goes through the explicit, audited ImpersonateUser RPC, which
	// mints a token whose ProfileID IS the target (so resolveProfileID then sees
	// reqProfileID == claims.ProfileID and allows it).
	ctx := ctxWithClaims("admin-1", "profile-admin", true)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1", ProfileId: "profile-target"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestFavoriteHandler_IsFavorite_IDORAnotherProfile(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.IsFavorite(ctx, &eveyv1.IsFavoriteRequest{MediaItemId: "m1", ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// capturingFavRepo wraps the real fake and records the pagination params last
// passed to List. Used to verify that the handler clamps hostile input before
// it reaches the repository layer.
type capturingFavRepo struct {
	*favfakes.FakeRepository
	lastLimit  int
	lastOffset int
}

func (c *capturingFavRepo) List(ctx context.Context, profileID string, limit, offset int) ([]*favorite.Favorite, error) {
	c.lastLimit = limit
	c.lastOffset = offset
	return c.FakeRepository.List(ctx, profileID, limit, offset)
}

// TestFavoriteHandler_ListFavorites_NegativeOffsetClamped proves that
// req.Offset=-1 never reaches the repo as -1: the repo fake would panic on
// out[-1:], so a passing test means the handler clamped it to 0.
func TestFavoriteHandler_ListFavorites_NegativeOffsetClamped(t *testing.T) {
	inner := favfakes.NewFakeRepository()
	cap := &capturingFavRepo{FakeRepository: inner}
	svc := appfavorite.NewService(cap)
	h := &favoriteHandler{svc: svc}
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{Offset: -1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 0, cap.lastOffset, "negative offset must be clamped to 0 before reaching the repo")
}

// TestFavoriteHandler_ListFavorites_HugeLimitClamped proves that a limit far
// above maxPageSize is replaced with a sane default, not forwarded verbatim.
func TestFavoriteHandler_ListFavorites_HugeLimitClamped(t *testing.T) {
	inner := favfakes.NewFakeRepository()
	cap := &capturingFavRepo{FakeRepository: inner}
	svc := appfavorite.NewService(cap)
	h := &favoriteHandler{svc: svc}
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{Limit: 1_000_000})
	require.NoError(t, err)
	assert.LessOrEqual(t, cap.lastLimit, maxPageSize, "huge limit must be clamped to at most maxPageSize")
}

// countingFavRepo counts repository round trips so a test can prove the batch
// RPC does ONE lookup for N ids instead of N.
type countingFavRepo struct {
	*favfakes.FakeRepository
	manyCalls   int
	singleCalls int
	lastIDs     []string
}

func (c *countingFavRepo) IsFavorite(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	c.singleCalls++
	return c.FakeRepository.IsFavorite(ctx, profileID, mediaItemID)
}

func (c *countingFavRepo) IsFavoriteMany(ctx context.Context, profileID string, ids []string) (map[string]bool, error) {
	c.manyCalls++
	c.lastIDs = ids
	return c.FakeRepository.IsFavoriteMany(ctx, profileID, ids)
}

// newBatchTestHandler wires an EXPLICIT library grant list. A user with zero
// grants is unrestricted everywhere in this codebase, which would make the
// leak test below pass without any guard at all.
func newBatchTestHandler(t *testing.T) (*favoriteHandler, *countingFavRepo) {
	t.Helper()
	repo := &countingFavRepo{FakeRepository: favfakes.NewFakeRepository()}
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	for _, id := range []string{"m1", "m2", "m3"} {
		mediaQueryRepo.Seed(&library.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib-1", Title: id, Type: "movie"})
	}
	mediaQueryRepo.Seed(&library.MediaItem{ID: "m-secret", LibraryID: "lib-private", Title: "Some Movie", Type: "movie"})
	libQuerySvc := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaQueryRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	require.NoError(t, libAccess.Grant(context.Background(), shared.UserID("user-1"), shared.LibraryID("lib-1")))
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccess),
	)
	h := &favoriteHandler{svc: appfavorite.NewService(repo), libQuery: libQuerySvc, userAccess: userAccess}
	return h, repo
}

func TestFavoriteHandler_BatchIsFavorite_OneRoundTripForManyIDs(t *testing.T) {
	h, repo := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m2"})
	require.NoError(t, err)
	repo.singleCalls = 0

	resp, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1", "m2", "m3"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"m2"}, resp.FavoriteMediaItemIds)
	assert.Equal(t, 1, repo.manyCalls, "N ids must cost ONE repository lookup")
	assert.Zero(t, repo.singleCalls, "the batch path must not fan out into per-id lookups")
}

func TestFavoriteHandler_BatchIsFavorite_DeduplicatesAndDropsEmpty(t *testing.T) {
	h, repo := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1", "m1", "", "m2"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2"}, repo.lastIDs)
}

// Over the ceiling the call is refused outright. Truncating instead would hand
// the client "not a favorite" for the dropped ids - an empty heart on a real
// favorite, silently.
func TestFavoriteHandler_BatchIsFavorite_OverLimitRefused(t *testing.T) {
	h, repo := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)
	ids := make([]string, maxBatchIsFavoriteIDs+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("m-%d", i)
	}

	_, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: ids})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Zero(t, repo.manyCalls, "an over-sized batch must be refused before it reaches the repo")
}

// IDOR: a batch endpoint is the classic place to leak by id. An item in a
// library the caller was not granted must come back exactly like a non-favorite
// - absent - even when the favorites row genuinely exists (access revoked after
// the fact). Anything else turns this RPC into an existence oracle.
func TestFavoriteHandler_BatchIsFavorite_HidesMediaOutsideLibraryAccess(t *testing.T) {
	h, repo := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)
	require.NoError(t, repo.Add(ctx, &favorite.Favorite{ID: "f-secret", ProfileID: "profile-1", MediaItemID: "m-secret"}))
	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	resp, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1", "m-secret"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1"}, resp.FavoriteMediaItemIds,
		"an item in a library the caller cannot access must be indistinguishable from a non-favorite")
}

// An id that matches no media item must also be silent - same shape as above,
// so probing random ids reveals nothing.
func TestFavoriteHandler_BatchIsFavorite_UnknownIDsAreSilent(t *testing.T) {
	h, _ := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m-does-not-exist"}})
	require.NoError(t, err)
	assert.Empty(t, resp.FavoriteMediaItemIds)
}

// A DB error while reading library access must fail CLOSED, like ListFavorites.
func TestFavoriteHandler_BatchIsFavorite_AccessErrorFailsClosed(t *testing.T) {
	h, _ := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	broken := userfakes.NewFakeLibraryAccessRepository()
	broken.Err = errors.New("db down")
	h.userAccess = appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(broken),
	)

	_, err = h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1"}})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// A wiring regression (userAccess left nil) must deny a non-admin, never
// fall through to an unguarded lookup.
func TestFavoriteHandler_BatchIsFavorite_UserAccessUnwiredDenies(t *testing.T) {
	h := &favoriteHandler{svc: appfavorite.NewService(favfakes.NewFakeRepository())}
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1"}})
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

// A kids profile must not learn that an adult title is favorited.
func TestFavoriteHandler_BatchIsFavorite_ParentalBlockedHidden(t *testing.T) {
	repo := favfakes.NewFakeRepository()
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-pg", Title: "Family", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}})
	libQuerySvc := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(), mediaQueryRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	profileRepo := userfakes.NewFakeProfileRepository()
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profileRepo,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository())
	h := &favoriteHandler{svc: appfavorite.NewService(repo), libQuery: libQuerySvc,
		userAccess: newPermissiveTestUserAccess(), userQuery: userQuery}

	ctx := context.Background()
	maxAge := 13
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{
		ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}}))
	require.NoError(t, repo.Add(ctx, &favorite.Favorite{ID: "f-r", ProfileID: "profile-2", MediaItemID: "mi-r"}))
	require.NoError(t, repo.Add(ctx, &favorite.Favorite{ID: "f-pg", ProfileID: "profile-2", MediaItemID: "mi-pg"}))

	resp, err := h.BatchIsFavorite(ctxWithClaims("user-2", "profile-2", false),
		&eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"mi-r", "mi-pg"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"mi-pg"}, resp.FavoriteMediaItemIds,
		"a parentally blocked favorite must be hidden, not reported")
}

func TestFavoriteHandler_BatchIsFavorite_Unauthenticated(t *testing.T) {
	h, _ := newBatchTestHandler(t)

	_, err := h.BatchIsFavorite(context.Background(), &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1"}})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFavoriteHandler_BatchIsFavorite_IDORAnotherProfile(t *testing.T) {
	h, _ := newBatchTestHandler(t)
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.BatchIsFavorite(ctx, &eveyv1.BatchIsFavoriteRequest{MediaItemIds: []string{"m1"}, ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
