package notification

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type QueryService struct {
	repo     notification.Repository
	settings notification.SettingsRepository
}

func NewQueryService(repo notification.Repository, settings notification.SettingsRepository) *QueryService {
	return &QueryService{repo: repo, settings: settings}
}

func (s *QueryService) List(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	return s.repo.ListByProfile(ctx, profileID, unreadOnly, limit, offset)
}

func (s *QueryService) UnreadCount(ctx context.Context, profileID string) (int, error) {
	return s.repo.UnreadCount(ctx, profileID)
}

func (s *QueryService) GetSettings(ctx context.Context, profileID string) (*notification.Settings, error) {
	return s.settings.GetByProfile(ctx, profileID)
}

func (s *QueryService) GetGlobalSettings(ctx context.Context) (*notification.Settings, error) {
	return s.settings.GetGlobalDefaults(ctx)
}
