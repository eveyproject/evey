package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newPublicHandler builds a libraryHandler wired identically to
// newTestLibraryHandler (library_handler_test.go), with an empty translator
// as used in library_handler_person_test.go and library_handler_scan_test.go.
// accessRepo is exposed so individual tests can call Grant to set up
// per-user library restrictions, and profiles is exposed for parental setup.
func newPublicHandler() (
	*libraryHandler,
	*libfakes.FakeMediaItemQueryRepo,
	*userfakes.FakeLibraryAccessRepository,
	*userfakes.FakeProfileRepository,
) {
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	accessRepo := userfakes.NewFakeLibraryAccessRepository()
	profiles := userfakes.NewFakeProfileRepository()
	eventBus := sharedfakes.NewFakeEventBus()

	querySvc := applibrary.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, searchIdx)

	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		eventBus,
		appuser.WithLibraryAccess(accessRepo),
	)
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(),
		profiles,
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)

	h := &libraryHandler{
		query:      querySvc,
		userAccess: userAccess,
		userQuery:  userQuery,
		translator: eveyi18n.NewEmpty(),
	}
	// The parental gate now fails closed on an unresolvable profile, so the
	// non-parental tests need their claimed profiles to actually exist.
	// Kids-profile tests overwrite these with their own settings.
	_ = profiles.Create(context.Background(), &user.Profile{ID: "profile-1", UserID: "user-1", Name: "Adult"})
	_ = profiles.Create(context.Background(), &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Adult"})
	_ = profiles.Create(context.Background(), &user.Profile{ID: "profile-3", UserID: "user-3", Name: "Adult"})
	return h, mediaQueryRepo, accessRepo, profiles
}

// restrictedUserCtx returns a non-admin context for the given user/profile.
// Uses ctxWithClaims (handler_test_helpers_test.go) for consistency.
func restrictedUserCtx(userID, profileID string) context.Context {
	return ctxWithClaims(userID, profileID, false)
}

// --- GetPublicMediaItem ---

// Happy path: accessible library, non-blocked item → item returned.
func TestGetPublicMediaItem_HappyPath(t *testing.T) {
	h, mediaQuery, accessRepo, _ := newPublicHandler()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-1",
		LibraryID: "lib-1",
		Type:      library.MovieItem,
		Title:     "Open Sesame",
	})
	// Grant "user-1" access only to "lib-1".
	require.NoError(t, accessRepo.Grant(context.Background(), "user-1", "lib-1"))

	ctx := restrictedUserCtx("user-1", "profile-1")
	resp, err := h.GetPublicMediaItem(ctx, &eveyv1.GetPublicMediaItemRequest{Id: "mi-1"})

	require.NoError(t, err)
	assert.Equal(t, "mi-1", resp.Id)
	assert.Equal(t, "Open Sesame", resp.Title)
}

// Access-denied: item lives in lib-2, caller only has access to lib-1 →
// NotFound (no information leak about whether the item even exists).
func TestGetPublicMediaItem_LibraryNotAccessible_NotFound(t *testing.T) {
	h, mediaQuery, accessRepo, _ := newPublicHandler()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-secret",
		LibraryID: "lib-2", // caller has NO grant for this
		Type:      library.MovieItem,
		Title:     "Restricted Movie",
	})
	// Only lib-1 is granted; lib-2 is invisible.
	require.NoError(t, accessRepo.Grant(context.Background(), "user-1", "lib-1"))

	ctx := restrictedUserCtx("user-1", "profile-1")
	_, err := h.GetPublicMediaItem(ctx, &eveyv1.GetPublicMediaItemRequest{Id: "mi-secret"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err), "access-denied must return NotFound, not the real status")
}

// Parental-blocked: a kids profile with DefaultKidsMaxAge blocks items that
// have no resolvable age rating (fail-closed).
func TestGetPublicMediaItem_ParentalBlocked_NotFound(t *testing.T) {
	maxAge := 6
	h, mediaQuery, accessRepo, profiles := newPublicHandler()

	require.NoError(t, profiles.Create(context.Background(), &user.Profile{
		ID:     "prf-kids",
		UserID: "usr-kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			Enabled: true,
			MaxAge:  &maxAge,
		},
	}))

	// Item in lib-1, no classification → unresolvable rating → fail-closed.
	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-rrated",
		LibraryID: "lib-1",
		Type:      library.MovieItem,
		Title:     "Violent Film",
	})
	require.NoError(t, accessRepo.Grant(context.Background(), "usr-kid", "lib-1"))

	ctx := restrictedUserCtx("usr-kid", "prf-kids")
	_, err := h.GetPublicMediaItem(ctx, &eveyv1.GetPublicMediaItemRequest{Id: "mi-rrated"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err), "parental-blocked item must return NotFound")
}

// Query error (e.g. DB unavailable) → NotFound, no panic.
func TestGetPublicMediaItem_QueryError_NotFound(t *testing.T) {
	h, mediaQuery, _, _ := newPublicHandler()
	mediaQuery.Err = errors.New("db unavailable")

	ctx := restrictedUserCtx("user-1", "profile-1")
	_, err := h.GetPublicMediaItem(ctx, &eveyv1.GetPublicMediaItemRequest{Id: "any"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- ListPublicMediaItems ---

// A kids/parental-restricted caller must have AllowedRatings applied to the
// query. We assert behaviourally via the fake query repo, which filters items
// whose OfficialRating is non-empty and not in allowedSet. The handler passes
// filter.AllowedRatings = AllowedRatingsForAge(maxAge), which excludes "R"
// (MPAA, min-age 17) for a max-age-6 profile.
func TestListPublicMediaItems_ParentalFilter_Applied(t *testing.T) {
	maxAge := 6
	h, mediaQuery, accessRepo, profiles := newPublicHandler()

	require.NoError(t, profiles.Create(context.Background(), &user.Profile{
		ID:     "prf-kids2",
		UserID: "usr-kid2",
		IsKids: true,
		Parental: &user.ParentalSettings{
			Enabled: true,
			MaxAge:  &maxAge,
		},
	}))

	require.NoError(t, accessRepo.Grant(context.Background(), "usr-kid2", "lib-10"))

	// G-rated (age 0) - must be returned to the kids caller.
	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-g",
		LibraryID: "lib-10",
		Type:      library.MovieItem,
		Title:     "Kids Film",
		Classification: &library.MediaClassification{
			OfficialRating: "G",
		},
	})
	// R-rated (age 17) - must be excluded by the parental filter.
	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-r",
		LibraryID: "lib-10",
		Type:      library.MovieItem,
		Title:     "Adult Film",
		Classification: &library.MediaClassification{
			OfficialRating: "R",
		},
	})

	ctx := restrictedUserCtx("usr-kid2", "prf-kids2")
	resp, err := h.ListPublicMediaItems(ctx, &eveyv1.ListPublicMediaItemsRequest{
		LibraryId: "lib-10",
	})

	require.NoError(t, err)
	// The parental AllowedRatings filter must have been applied:
	// G passes, R is excluded.
	assert.NotEmpty(t, resp.Items, "kids should see the G-rated item")
	ids := make([]string, 0, len(resp.Items))
	for _, it := range resp.Items {
		ids = append(ids, it.Id)
	}
	assert.NotContains(t, ids, "mi-r", "R-rated item must not leak past parental filter")
}

// Caller requests a LibraryId they cannot access → empty response, no items leaked.
func TestListPublicMediaItems_InaccessibleLibrary_EmptyResponse(t *testing.T) {
	h, mediaQuery, accessRepo, _ := newPublicHandler()

	// Seed item in lib-forbidden.
	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-forbidden",
		LibraryID: "lib-forbidden",
		Type:      library.MovieItem,
		Title:     "Secret Item",
	})
	// Grant caller access only to lib-allowed, NOT lib-forbidden.
	require.NoError(t, accessRepo.Grant(context.Background(), "user-2", "lib-allowed"))

	ctx := restrictedUserCtx("user-2", "profile-2")
	resp, err := h.ListPublicMediaItems(ctx, &eveyv1.ListPublicMediaItemsRequest{
		LibraryId: "lib-forbidden",
	})

	require.NoError(t, err)
	assert.Empty(t, resp.Items, "items from an inaccessible library must not be returned")
	assert.Zero(t, resp.Total)
}

// Happy path: accessible library with items → items and total returned.
func TestListPublicMediaItems_HappyPath(t *testing.T) {
	h, mediaQuery, accessRepo, _ := newPublicHandler()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-a",
		LibraryID: "lib-movies",
		Type:      library.MovieItem,
		Title:     "Film A",
	})
	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-b",
		LibraryID: "lib-movies",
		Type:      library.MovieItem,
		Title:     "Film B",
	})
	require.NoError(t, accessRepo.Grant(context.Background(), "user-3", "lib-movies"))

	ctx := restrictedUserCtx("user-3", "profile-3")
	resp, err := h.ListPublicMediaItems(ctx, &eveyv1.ListPublicMediaItemsRequest{
		LibraryId: "lib-movies",
	})

	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.Positive(t, resp.Total)
}
