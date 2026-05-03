package feed

import (
	"context"

	"github.com/evey-project/evey/internal/domain/feed"
	"github.com/evey-project/evey/internal/domain/notification"
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

	seen := make(map[feed.Type]bool, len(admins))
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
		seen[a.FeedType] = true
		result = append(result, m)
	}
	// Profile overrides without an admin parent (e.g. dynamic feed types
	// like library:<id> or plugin:<id> that are never explicitly created
	// admin-side) must still surface in the merged view — otherwise the
	// CustomizePanel drag/toggle persists then disappears on refetch and
	// the home page can't honour the user's saved order.
	//
	// Defaults for the "missing admin" case: enabled=true, displayOrder=0.
	// Any field set on the override wins as usual.
	for _, o := range overrides {
		if seen[o.FeedType] {
			continue
		}
		m := &feed.MergedConfig{
			FeedType:     o.FeedType,
			Enabled:      true,
			DisplayOrder: 0,
			Params:       o.Params,
		}
		if o.Enabled != nil {
			m.Enabled = *o.Enabled
		}
		if o.DisplayOrder != nil {
			m.DisplayOrder = *o.DisplayOrder
		}
		if o.MaxItems != nil {
			m.MaxItems = *o.MaxItems
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
