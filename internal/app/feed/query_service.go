package feed

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/domain/notification"
)

type QueryService struct {
	configs  feed.ConfigQueryRepo
	querier  feed.FeedQuerier
	analyzer notification.WatchHistoryAnalyzer
}

func NewQueryService(
	configs feed.ConfigQueryRepo,
	querier feed.FeedQuerier,
	analyzer notification.WatchHistoryAnalyzer,
) *QueryService {
	return &QueryService{configs: configs, querier: querier, analyzer: analyzer}
}

func (s *QueryService) GetMergedConfig(ctx context.Context, profileID string) ([]*feed.MergedConfig, error) {
	admins, err := s.configs.GetAdminConfig(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := s.configs.GetProfileConfig(ctx, profileID)
	if err != nil {
		return nil, err
	}
	overrideMap := make(map[feed.Type]*feed.ProfileConfig, len(overrides))
	for _, o := range overrides {
		overrideMap[o.FeedType] = o
	}

	var result []*feed.MergedConfig
	for _, a := range admins {
		m := &feed.MergedConfig{
			FeedType:     a.FeedType,
			Enabled:      a.Enabled,
			DisplayOrder: a.DisplayOrder,
			MaxItems:     a.MaxItems,
			Params:       a.Params,
		}
		if o, ok := overrideMap[a.FeedType]; ok {
			if o.Enabled != nil {
				m.Enabled = *o.Enabled
			}
			if o.DisplayOrder != nil {
				m.DisplayOrder = *o.DisplayOrder
			}
			if o.MaxItems != nil {
				m.MaxItems = *o.MaxItems
			}
			if o.Params != nil {
				m.Params = o.Params
			}
		}
		result = append(result, m)
	}
	return result, nil
}

func (s *QueryService) GetAdminConfigs(ctx context.Context) ([]*feed.AdminConfig, error) {
	return s.configs.GetAdminConfig(ctx)
}

func (s *QueryService) ContinueWatching(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	return s.querier.ContinueWatching(ctx, profileID, limit)
}

func (s *QueryService) ResumePlayback(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	return s.querier.ResumePlayback(ctx, profileID, limit)
}

func (s *QueryService) NextEpisode(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	return s.querier.NextEpisode(ctx, profileID, limit)
}

func (s *QueryService) RecentlyAdded(ctx context.Context, daysWindow, limit int) ([]*feed.FeedItem, error) {
	return s.querier.RecentlyAdded(ctx, daysWindow, limit)
}

func (s *QueryService) Recommended(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	genres, err := s.analyzer.GetFavoriteGenres(ctx, profileID, 3)
	if err != nil {
		return nil, err
	}
	return s.querier.Recommended(ctx, profileID, genres, limit)
}

func (s *QueryService) SimilarItems(ctx context.Context, mediaItemID, profileID string, limit int) ([]*feed.FeedItem, error) {
	return s.querier.SimilarItems(ctx, mediaItemID, profileID, limit)
}
