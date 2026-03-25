package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	feedfakes "github.com/gravity-zero/evey/internal/domain/feed/fakes"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newFeedHandler() (*feedHandler, *feedfakes.FakeConfigCommandRepo, *feedfakes.FakeConfigQueryRepo, *feedfakes.FakeFeedQuerier) {
	cmdRepo := feedfakes.NewFakeConfigCommandRepo()
	queryRepo := feedfakes.NewFakeConfigQueryRepo()
	querier := feedfakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	commands := appfeed.NewCommandService(cmdRepo, queryRepo)
	queries := appfeed.NewQueryService(queryRepo, querier, analyzer)

	h := &feedHandler{commands: commands, queries: queries}
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
		{MediaItemID: "m1", Title: "Movie 1", Type: "movie", PositionMs: 5000},
		{MediaItemID: "m2", Title: "Movie 2", Type: "movie", PositionMs: 12000},
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
