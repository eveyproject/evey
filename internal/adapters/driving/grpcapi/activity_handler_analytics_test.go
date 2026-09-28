package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/activity"
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

// GetAnalytics requires admin claims.
func TestActivityHandler_GetAnalytics_NonAdmin_PermissionDenied(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.GetAnalytics(ctx, &eveyv1.GetAnalyticsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// GetAnalytics without claims returns PermissionDenied (no claims ≠ admin).
func TestActivityHandler_GetAnalytics_NoClaims_PermissionDenied(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	_, err := h.GetAnalytics(context.Background(), &eveyv1.GetAnalyticsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// GetAnalytics with admin returns the wire payload; the fake service yields
// zero-valued totals (no history) plus empty slices.
func TestActivityHandler_GetAnalytics_Admin_Empty(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("admin-1", "profile-1", true)
	resp, err := h.GetAnalytics(ctx, &eveyv1.GetAnalyticsRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.TotalPlaysToday)
	assert.Equal(t, int32(0), resp.TotalPlaysThisWeek)
	assert.Equal(t, int32(0), resp.ActiveUsersToday)
	assert.Empty(t, resp.TopMedia)
	assert.Empty(t, resp.RecentActivity)
}

// GetPopularMedia requires auth.
func TestActivityHandler_GetPopularMedia_Unauthenticated(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	_, err := h.GetPopularMedia(context.Background(), &eveyv1.GetPopularMediaRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// GetPopularMedia with no data returns empty list (no lib query wired).
func TestActivityHandler_GetPopularMedia_EmptyResults(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetPopularMedia(ctx, &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

// GetPopularMedia with limit <= 0 defaults to 20 internally. The default IS
// observable: ask for more rows than the default and check the response stops
// at it. The previous version of this test only checked the call succeeded,
// which is the one thing its name did not claim.
func TestActivityHandler_GetPopularMedia_DefaultLimit(t *testing.T) {
	t.Parallel()
	h, _, stats, _, _ := newActivityHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.GetPopularMedia(ctx, &eveyv1.GetPopularMediaRequest{Limit: 0})
	require.NoError(t, err)
	assert.Equal(t, 20, stats.LastMostPopularLimit, "limit 0 must become the default, not reach the reader as 0")

	// A negative limit is the same case, and must not reach the reader either -
	// a repository asked for -1 rows is a query nobody wrote a plan for.
	_, err = h.GetPopularMedia(ctx, &eveyv1.GetPopularMediaRequest{Limit: -5})
	require.NoError(t, err)
	assert.Equal(t, 20, stats.LastMostPopularLimit)

	// And an explicit limit is passed through untouched.
	_, err = h.GetPopularMedia(ctx, &eveyv1.GetPopularMediaRequest{Limit: 7})
	require.NoError(t, err)
	assert.Equal(t, 7, stats.LastMostPopularLimit)
}

// The popular row is scoped like every other row: a title in a library the
// member was not granted, or one that no longer exists, is dropped; an admin
// sees all of them; a failing access lookup fails closed.
// The popular row of a kids profile drops what its parental rules hide, like
// every other home row. Neutralising that filter left the package green
// (sabotage sweep, 2026-09-28).
func TestActivityHandler_GetPopularMedia_ParentalFiltered(t *testing.T) {
	h, _, stats, _, _ := newActivityHandler()
	stats.MostPopular = []activity.MediaPlayCount{
		{MediaItemID: "mi-clean", Title: "Safe", PlayCount: 9},
		{MediaItemID: "mi-violent", Title: "Action", PlayCount: 5},
	}
	g := map[string]string{"US": "G"}
	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaRepo.Seed(&library.MediaItem{ID: "mi-clean", LibraryID: "lib-1", Type: library.MovieItem, Title: "Safe",
		Catalog: &library.MediaCatalog{}, Classification: &library.MediaClassification{CertificationsByCountry: g}})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-violent", LibraryID: "lib-1", Type: library.MovieItem, Title: "Action",
		Catalog: &library.MediaCatalog{Tags: []string{"violence"}}, Classification: &library.MediaClassification{CertificationsByCountry: g}})
	h.libQuery = applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	maxAge := 7
	profiles := userfakes.NewFakeProfileRepository()
	require.NoError(t, profiles.Create(context.Background(), &user.Profile{
		ID: "profile-1", UserID: "user-1", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge, BlockedTags: []string{"violence"}},
	}))
	h.userQuery = appuser.NewQueryService(userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository())

	resp, err := h.GetPopularMedia(ctxWithClaims("user-1", "profile-1", false), &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	var got []string
	for _, it := range resp.Items {
		got = append(got, it.MediaItemId)
	}
	assert.Equal(t, []string{"mi-clean"}, got, "the hidden title must not be offered to the kids profile")
}

func TestActivityHandler_GetPopularMedia_LibraryScoped(t *testing.T) {
	h, _, stats, _, _ := newActivityHandler()
	stats.MostPopular = []activity.MediaPlayCount{
		{MediaItemID: "mi-mine", Title: "Mine", PlayCount: 9},
		{MediaItemID: "mi-other", Title: "Other", PlayCount: 5},
		{MediaItemID: "mi-gone", Title: "Deleted", PlayCount: 3},
	}
	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaRepo.Seed(&library.MediaItem{ID: "mi-mine", LibraryID: "lib-mine", Type: library.MovieItem, Title: "Mine"})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-other", LibraryID: "lib-other", Type: library.MovieItem, Title: "Other"})
	h.libQuery = applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	h.userAccess = appuser.NewAccessService(userfakes.NewFakeUserRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(), appuser.WithLibraryAccess(libAccess))
	ids := func(resp *eveyv1.GetPopularMediaResponse) []string {
		out := make([]string, 0, len(resp.Items))
		for _, it := range resp.Items {
			out = append(out, it.MediaItemId)
		}
		return out
	}

	require.NoError(t, libAccess.Grant(context.Background(), "user-1", "lib-mine"))
	resp, err := h.GetPopularMedia(ctxWithClaims("user-1", "profile-1", false), &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"mi-mine"}, ids(resp))
	assert.Equal(t, "movie", resp.Items[0].Type)
	assert.EqualValues(t, 9, resp.Items[0].PlayCount)

	resp, err = h.GetPopularMedia(ctxWithClaims("adm", "profile-adm", true), &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"mi-mine", "mi-other"}, ids(resp), "an admin sees every existing title; the deleted one is still dropped")

	libAccess.Err = errors.New("db down")
	_, err = h.GetPopularMedia(ctxWithClaims("user-1", "profile-1", false), &eveyv1.GetPopularMediaRequest{Limit: 10})
	assert.Equal(t, codes.Internal, status.Code(err), "an access lookup failure never reads as unrestricted")
}
