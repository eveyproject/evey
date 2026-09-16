package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appfeed "github.com/evey-project/evey/internal/app/feed"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/feed"
	feedfakes "github.com/evey-project/evey/internal/domain/feed/fakes"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func feedDurMs(ms int64) shared.Duration {
	d, _ := shared.NewDurationMs(ms)
	return d
}

func newFeedHandler() (*feedHandler, *feedfakes.FakeConfigCommandRepo, *feedfakes.FakeConfigQueryRepo, *feedfakes.FakeFeedQuerier) {
	cmdRepo := feedfakes.NewFakeConfigCommandRepo()
	queryRepo := feedfakes.NewFakeConfigQueryRepo()
	querier := feedfakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	commands := appfeed.NewCommandService(cmdRepo, queryRepo)
	queries := appfeed.NewQueryService(queryRepo, querier, analyzer)

	h := &feedHandler{commands: commands, queries: queries, userAccess: newPermissiveTestUserAccess()}
	return h, cmdRepo, queryRepo, querier
}

func TestFeedHandler_GetFeedConfig_Success(t *testing.T) {
	h, _, queryRepo, _ := newFeedHandler()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeContinueWatching, Enabled: true, DisplayOrder: 1, MaxItems: 10},
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 2, MaxItems: 20},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetFeedConfig(ctx, &eveyv1.GetFeedConfigRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Configs, 2)
	assert.Equal(t, string(feed.TypeContinueWatching), resp.Configs[0].FeedType)
	assert.True(t, resp.Configs[0].Enabled)
	assert.Equal(t, int32(10), resp.Configs[0].MaxItems)
}

func TestFeedHandler_GetFeedConfig_Unauthenticated(t *testing.T) {
	h, _, _, _ := newFeedHandler()

	_, err := h.GetFeedConfig(context.Background(), &eveyv1.GetFeedConfigRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFeedHandler_UpdateAdminFeedConfig_Success(t *testing.T) {
	h, cmdRepo, _, _ := newFeedHandler()

	resp, err := h.UpdateAdminFeedConfig(context.Background(), &eveyv1.UpdateAdminFeedConfigRequest{
		FeedType:     string(feed.TypeRecentlyAdded),
		Enabled:      true,
		DisplayOrder: 5,
		MaxItems:     25,
	})

	require.NoError(t, err)
	assert.Equal(t, string(feed.TypeRecentlyAdded), resp.FeedType)
	assert.True(t, resp.Enabled)
	assert.Equal(t, int32(5), resp.DisplayOrder)
	assert.Equal(t, int32(25), resp.MaxItems)

	stored := cmdRepo.AdminConfigs[feed.TypeRecentlyAdded]
	require.NotNil(t, stored)
	assert.Equal(t, 25, stored.MaxItems)
}

func TestFeedHandler_ContinueWatching_Success(t *testing.T) {
	h, _, _, querier := newFeedHandler()

	querier.ContinueWatchingItems = []*feed.FeedItem{
		{MediaItemID: "m1", Title: "Movie 1", Type: "movie", Position: feedDurMs(5000)},
		{MediaItemID: "m2", Title: "Movie 2", Type: "movie", Position: feedDurMs(12000)},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ContinueWatching(ctx, &eveyv1.ContinueWatchingRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "m1", resp.Items[0].MediaItemId)
	assert.Equal(t, int64(5000), resp.Items[0].PositionMs)
}

func TestFeedHandler_RecentlyAdded_Success(t *testing.T) {
	h, _, _, querier := newFeedHandler()

	querier.RecentlyAddedItems = []*feed.FeedItem{
		{MediaItemID: "m3", Title: "New Film", Type: "movie", AddedAt: "2026-03-10"},
	}

	resp, err := h.RecentlyAdded(context.Background(), &eveyv1.RecentlyAddedRequest{DaysWindow: 7, Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "m3", resp.Items[0].MediaItemId)
}

func TestFeedHandler_UpdateProfileFeedConfig_Success(t *testing.T) {
	h, _, _, _ := newFeedHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	enabled := true
	displayOrder := int32(3)
	maxItems := int32(15)
	resp, err := h.UpdateProfileFeedConfig(ctx, &eveyv1.UpdateProfileFeedConfigRequest{
		FeedType:     string(feed.TypeContinueWatching),
		Enabled:      &enabled,
		DisplayOrder: &displayOrder,
		MaxItems:     &maxItems,
	})

	require.NoError(t, err)
	assert.Equal(t, "profile-1", resp.ProfileId)
	assert.Equal(t, string(feed.TypeContinueWatching), resp.FeedType)
	require.NotNil(t, resp.Enabled)
	assert.True(t, *resp.Enabled)
	assert.Equal(t, int32(3), *resp.DisplayOrder)
	assert.Equal(t, int32(15), *resp.MaxItems)
}

func TestFeedHandler_ResetProfileFeedConfig_Success(t *testing.T) {
	h, _, _, _ := newFeedHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ResetProfileFeedConfig(ctx, &eveyv1.ResetProfileFeedConfigRequest{
		FeedType: string(feed.TypeContinueWatching),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestFeedHandler_GetAdminFeedConfigs_Success(t *testing.T) {
	h, _, queryRepo, _ := newFeedHandler()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecommended, Enabled: true, DisplayOrder: 3, MaxItems: 12},
	}

	resp, err := h.GetAdminFeedConfigs(context.Background(), &eveyv1.GetAdminFeedConfigsRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Configs, 1)
	assert.Equal(t, string(feed.TypeRecommended), resp.Configs[0].FeedType)
	assert.Equal(t, int32(12), resp.Configs[0].MaxItems)
}

func TestFeedHandler_Recommended_Success(t *testing.T) {
	h, _, _, querier := newFeedHandler()

	querier.RecommendedItems = []*feed.FeedItem{
		{MediaItemID: "m5", Title: "Rec Movie", Type: "movie", Score: 90},
		{MediaItemID: "m6", Title: "Rec Series", Type: "series", Score: 85},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.Recommended(ctx, &eveyv1.RecommendedRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "m5", resp.Items[0].MediaItemId)
	assert.Equal(t, int32(90), resp.Items[0].Score)
}

func TestFeedHandler_Recommended_Unauthenticated(t *testing.T) {
	h, _, _, _ := newFeedHandler()
	_, err := h.Recommended(context.Background(), &eveyv1.RecommendedRequest{Limit: 10})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFeedHandler_SimilarItems_Success(t *testing.T) {
	h, _, _, querier := newFeedHandler()

	querier.SimilarItemsResult = []*feed.FeedItem{
		{MediaItemID: "m7", Title: "Similar A", Type: "movie"},
		{MediaItemID: "m8", Title: "Similar B", Type: "movie"},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.SimilarItems(ctx, &eveyv1.SimilarItemsRequest{MediaItemId: "m1", Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "m7", resp.Items[0].MediaItemId)
}

func TestFeedHandler_SimilarItems_Unauthenticated(t *testing.T) {
	h, _, _, _ := newFeedHandler()

	_, err := h.SimilarItems(context.Background(), &eveyv1.SimilarItemsRequest{MediaItemId: "m1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFeedHandler_SimilarItems_EmptyMediaID_InvalidArgument(t *testing.T) {
	h, _, _, _ := newFeedHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.SimilarItems(ctx, &eveyv1.SimilarItemsRequest{MediaItemId: "", Limit: 10})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// feedHandlerWithAccessResult groups the objects returned by
// newFeedHandlerWithAccess so callers can seed both media items and access
// grants without relying on positional return values.
type feedHandlerWithAccessResult struct {
	handler        *feedHandler
	querier        *feedfakes.FakeFeedQuerier
	mediaQueryRepo *libfakes.FakeMediaItemQueryRepo
	libraryAccess  *userfakes.FakeLibraryAccessRepository
}

// newFeedHandlerWithAccess builds a feedHandler with a real userAccess service
// and a real libQuery service wired in. This enables filterByLibraryAccess to
// exercise the actual grant check instead of short-circuiting via the
// "h.userAccess == nil" guard in the production code.
//
// The existing newFeedHandler helper is intentionally left unmodified so that
// the tests that use it continue to compile and pass.
func newFeedHandlerWithAccess() feedHandlerWithAccessResult {
	cmdRepo := feedfakes.NewFakeConfigCommandRepo()
	queryRepo := feedfakes.NewFakeConfigQueryRepo()
	querier := feedfakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	commands := appfeed.NewCommandService(cmdRepo, queryRepo)
	queries := appfeed.NewQueryService(queryRepo, querier, analyzer)

	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libQuerySvc := applibrary.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, searchIdx)

	eventBus := sharedfakes.NewFakeEventBus()
	userRepo := userfakes.NewFakeUserRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userRepo, apiKeyRepo, inviteRepo, qcRepo,
		userfakes.NewFakePasswordHasher(), eventBus,
		appuser.WithLibraryAccess(libraryAccess),
	)

	h := &feedHandler{
		commands:   commands,
		queries:    queries,
		userAccess: userAccess,
		libQuery:   libQuerySvc,
	}
	return feedHandlerWithAccessResult{
		handler:        h,
		querier:        querier,
		mediaQueryRepo: mediaQueryRepo,
		libraryAccess:  libraryAccess,
	}
}

// TestFeedHandler_RecentlyAdded_LibraryAccessFilter verifies that
// filterByLibraryAccess drops items whose underlying media item belongs to a
// library that the non-admin user is NOT granted access to.
//
// Without this test, removing the userAccess/libQuery nil-check short-circuit
// (or making the grant check always return true) would not break any existing
// test - this test closes that gap.
func TestFeedHandler_RecentlyAdded_LibraryAccessFilter(t *testing.T) {
	res := newFeedHandlerWithAccess()

	// Grant user-3 access to lib-allowed only; lib-hidden is not in the list.
	res.libraryAccess.Grant(context.Background(), "user-3", "lib-allowed")

	// Seed the media items so libQuery can resolve MediaItemID → LibraryID.
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-visible",
		LibraryID: "lib-allowed",
		Title:     "Visible Movie",
		Type:      "movie",
	})
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-hidden",
		LibraryID: "lib-hidden",
		Title:     "Hidden Movie",
		Type:      "movie",
	})

	// Both items are returned by the querier (simulating a DB query with no
	// access filter - the handler layer is responsible for filtering).
	res.querier.RecentlyAddedItems = []*feed.FeedItem{
		{MediaItemID: "mi-visible", Title: "Visible Movie", Type: "movie"},
		{MediaItemID: "mi-hidden", Title: "Hidden Movie", Type: "movie"},
	}

	ctx := ctxWithClaims("user-3", "profile-3", false)
	resp, err := res.handler.RecentlyAdded(ctx, &eveyv1.RecentlyAddedRequest{DaysWindow: 7, Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1, "only items from granted library must pass the filter")
	assert.Equal(t, "mi-visible", resp.Items[0].MediaItemId)
}

// TestFeedHandler_ContinueWatching_LibraryAccessFilter verifies that a
// profile's own continue-watching history is filtered by current library
// access: an item from a library the user is no longer granted must not
// resurface (ContinueWatching shares the access+parental filter now).
func TestFeedHandler_ContinueWatching_LibraryAccessFilter(t *testing.T) {
	res := newFeedHandlerWithAccess()

	res.libraryAccess.Grant(context.Background(), "user-3", "lib-allowed")
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID: "mi-visible", LibraryID: "lib-allowed", Title: "Visible", Type: "movie",
	})
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID: "mi-hidden", LibraryID: "lib-hidden", Title: "Hidden", Type: "movie",
	})
	res.querier.ContinueWatchingItems = []*feed.FeedItem{
		{MediaItemID: "mi-visible", Title: "Visible", Type: "movie", Position: feedDurMs(5000)},
		{MediaItemID: "mi-hidden", Title: "Hidden", Type: "movie", Position: feedDurMs(5000)},
	}

	ctx := ctxWithClaims("user-3", "profile-3", false)
	resp, err := res.handler.ContinueWatching(ctx, &eveyv1.ContinueWatchingRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1, "only items from a granted library may resurface")
	assert.Equal(t, "mi-visible", resp.Items[0].MediaItemId)
}

// TestFeedHandler_RecentlyAdded_LibraryAccessFilter_AdminBypass verifies
// the positive/admin path: admins receive all items regardless of library
// grants (filterByLibraryAccess short-circuits for IsAdmin). This ensures the
// filter test above discriminates rather than always passing.
func TestFeedHandler_RecentlyAdded_LibraryAccessFilter_AdminBypass(t *testing.T) {
	res := newFeedHandlerWithAccess()

	// Seed items in two different libraries.
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-a",
		LibraryID: "lib-a",
		Title:     "Film A",
		Type:      "movie",
	})
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-b",
		LibraryID: "lib-b",
		Title:     "Film B",
		Type:      "movie",
	})

	res.querier.RecentlyAddedItems = []*feed.FeedItem{
		{MediaItemID: "mi-a", Title: "Film A", Type: "movie"},
		{MediaItemID: "mi-b", Title: "Film B", Type: "movie"},
	}

	// Admin context: no grants needed.
	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	resp, err := res.handler.RecentlyAdded(ctx, &eveyv1.RecentlyAddedRequest{DaysWindow: 7, Limit: 10})

	require.NoError(t, err)
	assert.Len(t, resp.Items, 2, "admin must see all items across all libraries")
}

// TestFeedHandler_Recommended_LibraryAccessFilter mirrors the
// RecentlyAdded test for the Recommended RPC which also calls
// filterByLibraryAccess.
func TestFeedHandler_Recommended_LibraryAccessFilter(t *testing.T) {
	res := newFeedHandlerWithAccess()

	res.libraryAccess.Grant(context.Background(), "user-4", "lib-allowed")

	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-rec-ok",
		LibraryID: "lib-allowed",
		Title:     "Good Rec",
		Type:      "movie",
	})
	res.mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-rec-hidden",
		LibraryID: "lib-hidden",
		Title:     "Forbidden Rec",
		Type:      "movie",
	})

	res.querier.RecommendedItems = []*feed.FeedItem{
		{MediaItemID: "mi-rec-ok", Title: "Good Rec", Type: "movie", Score: 95},
		{MediaItemID: "mi-rec-hidden", Title: "Forbidden Rec", Type: "movie", Score: 90},
	}

	ctx := ctxWithClaims("user-4", "profile-4", false)
	resp, err := res.handler.Recommended(ctx, &eveyv1.RecommendedRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1, "only items from granted library must pass the filter")
	assert.Equal(t, "mi-rec-ok", resp.Items[0].MediaItemId)
}

// "Similar to this title" is gated on the seed title: a member who cannot
// see the seed's library (or a seed that no longer exists) gets NotFound
// - never the neighbours of a title they were never shown - while an admin
// is not subject to the grant check.
func TestFeedHandler_SimilarItems_SeedAccessGate(t *testing.T) {
	env := newFeedHandlerWithAccess()
	env.mediaQueryRepo.Seed(&library.MediaItem{ID: "seed-private", LibraryID: "lib-private", Title: "Private"})
	env.mediaQueryRepo.Seed(&library.MediaItem{ID: "seed-open", LibraryID: "lib-open", Title: "Open"})
	require.NoError(t, env.libraryAccess.Grant(context.Background(), "user-1", "lib-open"))

	_, err := env.handler.SimilarItems(ctxWithClaims("user-1", "profile-1", false), &eveyv1.SimilarItemsRequest{MediaItemId: "seed-private"})
	assert.Equal(t, codes.NotFound, status.Code(err), "a library the member was not granted")
	_, err = env.handler.SimilarItems(ctxWithClaims("user-1", "profile-1", false), &eveyv1.SimilarItemsRequest{MediaItemId: "seed-gone"})
	assert.Equal(t, codes.NotFound, status.Code(err), "a seed that does not exist")
	_, err = env.handler.SimilarItems(ctxWithClaims("user-1", "profile-1", false), &eveyv1.SimilarItemsRequest{MediaItemId: "seed-open"})
	require.NoError(t, err)
	_, err = env.handler.SimilarItems(ctxWithClaims("adm-1", "profile-adm", true), &eveyv1.SimilarItemsRequest{MediaItemId: "seed-private"})
	require.NoError(t, err, "an admin bypasses the grant")

	env.handler.userAccess = nil
	_, err = env.handler.SimilarItems(ctxWithClaims("user-1", "profile-1", false), &eveyv1.SimilarItemsRequest{MediaItemId: "seed-open"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "an unwired access port refuses rather than fails open")
}
