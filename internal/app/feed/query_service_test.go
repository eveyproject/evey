package feed_test

import (
	"context"
	"testing"

	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/domain/feed/fakes"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryService_GetMergedConfig_NoOverrides(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeContinueWatching, Enabled: true, DisplayOrder: 0, MaxItems: 10},
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 1, MaxItems: 20},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 2)
	assert.True(t, configs[0].Enabled)
	assert.Equal(t, 10, configs[0].MaxItems)
}

func TestQueryService_GetMergedConfig_ProfileOverridesMaxItems(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeContinueWatching, Enabled: true, DisplayOrder: 0, MaxItems: 10},
	}
	maxItems := 5
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.TypeContinueWatching, MaxItems: &maxItems},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, 5, configs[0].MaxItems)
	assert.True(t, configs[0].Enabled)
}

func TestQueryService_GetMergedConfig_ProfileDisablesFeed(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 1, MaxItems: 20},
	}
	disabled := false
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.TypeRecentlyAdded, Enabled: &disabled},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.False(t, configs[0].Enabled)
}

func TestQueryService_GetMergedConfig_ProfileCannotEnableDisabledFeed(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecommended, Enabled: false, DisplayOrder: 2, MaxItems: 15},
	}
	enabled := true
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.TypeRecommended, Enabled: &enabled},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.False(t, configs[0].Enabled)
}

func TestQueryService_GetAdminConfigs(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	expected := []*feed.AdminConfig{
		{FeedType: feed.TypeContinueWatching, Enabled: true},
	}
	queryRepo.AdminConfigs = expected

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	got, err := svc.GetAdminConfigs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestQueryService_ContinueWatching(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.ContinueWatchingItems = []*feed.FeedItem{
		{MediaItemID: "m1", Title: "Movie 1", Type: "movie", PositionMs: 5000},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.ContinueWatching(context.Background(), "p1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "m1", items[0].MediaItemID)
}

func TestQueryService_RecentlyAdded(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.RecentlyAddedItems = []*feed.FeedItem{
		{MediaItemID: "m2", Title: "New Film", Type: "movie"},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.RecentlyAdded(context.Background(), 30, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "m2", items[0].MediaItemID)
}

func TestQueryService_Recommended(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Genres = []string{"action", "sci-fi"}

	querier.RecommendedItems = []*feed.FeedItem{
		{MediaItemID: "m3", Title: "Action Movie", Type: "movie", Score: 2},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.Recommended(context.Background(), "p1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "m3", items[0].MediaItemID)
}

func TestQueryService_SimilarItems(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.SimilarItemsResult = []*feed.FeedItem{
		{MediaItemID: "m4", Title: "Similar Movie", Type: "movie", Score: 3},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.SimilarItems(context.Background(), "m1", "p1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "m4", items[0].MediaItemID)
}

func TestQueryService_ContinueWatching_Empty(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.ContinueWatching(context.Background(), "p1", 10)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestQueryService_RecentlyAdded_Empty(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.RecentlyAdded(context.Background(), 7, 10)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestQueryService_Recommended_NoGenres(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.RecommendedItems = []*feed.FeedItem{
		{MediaItemID: "m5", Title: "Popular Film", Type: "movie"},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.Recommended(context.Background(), "p-new", 5)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestQueryService_GetMergedConfig_Empty(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestQueryService_GetMergedConfig_ProfileOverridesDisplayOrder(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeContinueWatching, Enabled: true, DisplayOrder: 0, MaxItems: 10},
	}
	order := 5
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.TypeContinueWatching, DisplayOrder: &order},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, 5, configs[0].DisplayOrder)
	assert.True(t, configs[0].Enabled)
}
