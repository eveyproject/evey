package feed

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/domain/notification"
)

type Service struct {
	adminCfg    feed.AdminConfigRepository
	profileCfg  feed.ProfileConfigRepository
	querier     feed.FeedQuerier
	analyzer    notification.WatchHistoryAnalyzer
}

func NewService(
	adminCfg feed.AdminConfigRepository,
	profileCfg feed.ProfileConfigRepository,
	querier feed.FeedQuerier,
	analyzer notification.WatchHistoryAnalyzer,
) *Service {
	return &Service{adminCfg: adminCfg, profileCfg: profileCfg, querier: querier, analyzer: analyzer}
}

// GetMergedConfig returns the resolved feed configs for a profile, merging admin presets with profile overrides.
func (s *Service) GetMergedConfig(ctx context.Context, profileID string) ([]*feed.MergedConfig, error) {
	admins, err := s.adminCfg.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := s.profileCfg.GetByProfile(ctx, profileID)
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
			// profile can only disable, not enable a disabled feed
			if o.Enabled != nil && (!a.Enabled || !*o.Enabled) {
				m.Enabled = false
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

func (s *Service) ContinueWatching(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	return s.querier.ContinueWatching(ctx, profileID, limit)
}

func (s *Service) RecentlyAdded(ctx context.Context, daysWindow, limit int) ([]*feed.FeedItem, error) {
	return s.querier.RecentlyAdded(ctx, daysWindow, limit)
}

func (s *Service) Recommended(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	genres, err := s.analyzer.GetFavoriteGenres(ctx, profileID, 3)
	if err != nil {
		return nil, err
	}
	return s.querier.Recommended(ctx, profileID, genres, limit)
}

func (s *Service) SimilarItems(ctx context.Context, mediaItemID, profileID string, limit int) ([]*feed.FeedItem, error) {
	return s.querier.SimilarItems(ctx, mediaItemID, profileID, limit)
}

func (s *Service) UpdateAdminConfig(ctx context.Context, cfg *feed.AdminConfig) error {
	return s.adminCfg.Upsert(ctx, cfg)
}

func (s *Service) GetAdminConfigs(ctx context.Context) ([]*feed.AdminConfig, error) {
	return s.adminCfg.GetAll(ctx)
}

func (s *Service) UpdateProfileConfig(ctx context.Context, cfg *feed.ProfileConfig) error {
	return s.profileCfg.Upsert(ctx, cfg)
}

func (s *Service) ResetProfileConfig(ctx context.Context, profileID string, feedType feed.Type) error {
	return s.profileCfg.Delete(ctx, profileID, feedType)
}
