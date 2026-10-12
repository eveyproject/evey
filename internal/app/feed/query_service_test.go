package feed_test

import (
	"context"
	"errors"
	"testing"

	appfeed "github.com/eveyproject/evey/internal/app/feed"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/feed/fakes"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func feedDurMs(ms int64) shared.Duration {
	d, _ := shared.NewDurationMs(ms)
	return d
}

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

func TestQueryService_GetMergedConfig_ProfileCanEnableDisabledFeed(t *testing.T) {
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
	assert.True(t, configs[0].Enabled)
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
		{MediaItemID: "m1", Title: "Movie 1", Type: "movie", Position: feedDurMs(5000)},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.ContinueWatching(context.Background(), "p1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m1"), items[0].MediaItemID)
}

func TestQueryService_RecentlyAdded(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.RecentlyAddedItems = []*feed.FeedItem{
		{MediaItemID: "m2", Title: "New Film", Type: "movie"},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.RecentlyAdded(context.Background(), "p1", 30, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m2"), items[0].MediaItemID)
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
	assert.Equal(t, shared.MediaItemID("m3"), items[0].MediaItemID)
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
	assert.Equal(t, shared.MediaItemID("m4"), items[0].MediaItemID)
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
	items, err := svc.RecentlyAdded(context.Background(), "p1", 7, 10)
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

// Profile-only entries (no admin parent) must surface in the merged
// view. Otherwise dynamic feed types like library:<id> or plugin:<id>
// - which are never registered admin-side - vanish on every refetch
// after the user toggles or reorders them in CustomizePanel.
func TestQueryService_GetMergedConfig_OrphanedProfileOverrides_Surface(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 0, MaxItems: 20},
	}
	enabled := true
	displayOrder := 5
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.PluginType("plg-weather"), Enabled: &enabled, DisplayOrder: &displayOrder},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 2)

	var pluginCfg *feed.MergedConfig
	for _, c := range configs {
		if c.FeedType == feed.PluginType("plg-weather") {
			pluginCfg = c
		}
	}
	require.NotNil(t, pluginCfg, "orphaned plugin override must surface")
	assert.True(t, pluginCfg.Enabled)
	assert.Equal(t, 5, pluginCfg.DisplayOrder)
}

// Profile-only with Enabled unset defaults to true (don't drop the
// row just because the override blob omitted the flag).
func TestQueryService_GetMergedConfig_OrphanedOverride_DefaultEnabled(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.LibraryType("lib-1")},
	}
	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.True(t, configs[0].Enabled)
	assert.Equal(t, 0, configs[0].DisplayOrder)
}

// The merged config is the home layout every client renders. It must come back
// in one total order - by display order, ties broken by feed type - or two
// devices sorting the same rows can place tied rows differently.
func TestQueryService_GetMergedConfig_TotalOrder(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeResumePlayback, Enabled: true, DisplayOrder: 0},
		{FeedType: feed.TypeRecommended, Enabled: true, DisplayOrder: 1},
		{FeedType: feed.TypeMostPopular, Enabled: true, DisplayOrder: 2},
	}
	zero, one := 0, 1
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.TypeMostPopular, DisplayOrder: &zero},
		{ProfileID: "p1", FeedType: feed.PluginType("plg-weather"), DisplayOrder: &one},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	got := make([]feed.Type, len(configs))
	for i, c := range configs {
		got[i] = c.FeedType
	}
	assert.Equal(t, []feed.Type{
		feed.TypeMostPopular, feed.TypeResumePlayback,
		feed.PluginType("plg-weather"), feed.TypeRecommended,
	}, got)
}

func TestQueryService_ResumePlayback(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.ResumePlaybackItems = []*feed.FeedItem{
		{MediaItemID: "m10", Title: "Paused Movie", Type: "movie", Position: feedDurMs(30000)},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.ResumePlayback(context.Background(), "p1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m10"), items[0].MediaItemID)
}

func TestQueryService_NextEpisode(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	querier.NextEpisodeItems = []*feed.FeedItem{
		{MediaItemID: "ep5", Title: "Episode 5", Type: "episode"},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	items, err := svc.NextEpisode(context.Background(), "p1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("ep5"), items[0].MediaItemID)
}

func TestQueryService_Recommended_AnalyzerError(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	analyzer.Err = errors.New("db down")

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	_, err := svc.Recommended(context.Background(), "p1", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestQueryService_GetMergedConfig_ProfileOverridesParams(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 1, MaxItems: 20, Params: map[string]any{"key": "admin"}},
	}
	queryRepo.ProfileConfigs["p1"] = []*feed.ProfileConfig{
		{ProfileID: "p1", FeedType: feed.TypeRecentlyAdded, Params: map[string]any{"key": "user"}},
	}

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	configs, err := svc.GetMergedConfig(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, map[string]any{"key": "user"}, configs[0].Params)
}

func TestQueryService_GetMergedConfig_AdminQueryError(t *testing.T) {
	queryRepo := fakes.NewFakeConfigQueryRepo()
	querier := fakes.NewFakeFeedQuerier()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	queryRepo.Err = errors.New("db error")

	svc := appfeed.NewQueryService(queryRepo, querier, analyzer)
	_, err := svc.GetMergedConfig(context.Background(), "p1")
	require.Error(t, err)
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
