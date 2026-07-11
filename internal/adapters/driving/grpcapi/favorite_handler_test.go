package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/favorite"
	favfakes "github.com/evey-project/evey/internal/domain/favorite/fakes"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestFavoriteHandler() (*favoriteHandler, *favfakes.FakeRepository) {
	repo := favfakes.NewFakeRepository()
	svc := appfavorite.NewService(repo)
	return &favoriteHandler{svc: svc}, repo
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
