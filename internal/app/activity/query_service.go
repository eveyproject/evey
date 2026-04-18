package activity

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
)

type QueryService struct {
	loginHistory    activity.LoginHistoryRepository
	metrics         activity.ServerMetricRepository
	auditLog        activity.AuditLogRepository
	statsReader     activity.PlaySessionStatsReader
	playHistory     activity.PlayHistoryRepository
	analyticsReader activity.AnalyticsReader
}

type Option func(*QueryService)

func WithAnalyticsReader(r activity.AnalyticsReader) Option {
	return func(s *QueryService) { s.analyticsReader = r }
}

func NewQueryService(
	loginHistory activity.LoginHistoryRepository,
	metrics activity.ServerMetricRepository,
	auditLog activity.AuditLogRepository,
	statsReader activity.PlaySessionStatsReader,
	playHistory activity.PlayHistoryRepository,
	opts ...Option,
) *QueryService {
	s := &QueryService{
		loginHistory: loginHistory,
		metrics:      metrics,
		auditLog:     auditLog,
		statsReader:  statsReader,
		playHistory:  playHistory,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *QueryService) GetLoginHistory(ctx context.Context, userID string, limit, offset int) ([]*activity.LoginEntry, error) {
	return s.loginHistory.GetByUserID(ctx, shared.UserID(userID), limit, offset)
}

func (s *QueryService) GetLoginHistoryCursor(ctx context.Context, userID string, successOnly bool, from, to *time.Time, cursor *activity.LoginHistoryCursor, limit int) ([]*activity.LoginEntry, error) {
	return s.loginHistory.ListByUserIDCursor(ctx, shared.UserID(userID), successOnly, from, to, cursor, limit)
}

func (s *QueryService) GetServerMetrics(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	return s.metrics.GetByTimeRange(ctx, from, to)
}

func (s *QueryService) GetAuditLog(ctx context.Context, filter activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	return s.auditLog.List(ctx, filter)
}

func (s *QueryService) GetUserStats(ctx context.Context, profileID string) (*activity.UserStats, error) {
	return s.statsReader.GetUserStats(ctx, shared.ProfileID(profileID))
}

func (s *QueryService) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	return s.statsReader.GetServerStats(ctx)
}

func (s *QueryService) GetPlayHistory(ctx context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	return s.playHistory.List(ctx, filter)
}

func (s *QueryService) GetMostPopular(ctx context.Context, limit int) ([]activity.MediaPlayCount, error) {
	return s.statsReader.GetMostPopular(ctx, limit)
}

func (s *QueryService) GetAnalytics(ctx context.Context) (*activity.AnalyticsData, error) {
	if s.analyticsReader == nil {
		return &activity.AnalyticsData{
			TopMedia:       []activity.TopMediaItem{},
			RecentActivity: []activity.RecentActivityEntry{},
		}, nil
	}
	return s.analyticsReader.GetAnalytics(ctx)
}
