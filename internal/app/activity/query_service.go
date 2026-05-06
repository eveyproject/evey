package activity

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetLoginHistory")
	defer span.End()
	return s.loginHistory.GetByUserID(ctx, shared.UserID(userID), limit, offset)
}

func (s *QueryService) GetLoginHistoryCursor(ctx context.Context, userID string, successOnly bool, from, to *time.Time, cursor *activity.LoginHistoryCursor, limit int) ([]*activity.LoginEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetLoginHistoryCursor")
	defer span.End()
	return s.loginHistory.ListByUserIDCursor(ctx, shared.UserID(userID), successOnly, from, to, cursor, limit)
}

func (s *QueryService) GetServerMetrics(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetServerMetrics")
	defer span.End()
	return s.metrics.GetByTimeRange(ctx, from, to)
}

func (s *QueryService) GetAuditLog(ctx context.Context, filter activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetAuditLog")
	defer span.End()
	return s.auditLog.List(ctx, filter)
}

func (s *QueryService) GetUserStats(ctx context.Context, profileID string) (*activity.UserStats, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetUserStats")
	defer span.End()
	return s.statsReader.GetUserStats(ctx, shared.ProfileID(profileID))
}

func (s *QueryService) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetServerStats")
	defer span.End()
	return s.statsReader.GetServerStats(ctx)
}

func (s *QueryService) GetPlayHistory(ctx context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetPlayHistory")
	defer span.End()
	return s.playHistory.List(ctx, filter)
}

func (s *QueryService) GetMostPopular(ctx context.Context, profileID string, limit int) ([]activity.MediaPlayCount, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetMostPopular")
	defer span.End()
	return s.statsReader.GetMostPopular(ctx, profileID, limit)
}

func (s *QueryService) GetAnalytics(ctx context.Context) (*activity.AnalyticsData, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.GetAnalytics")
	defer span.End()
	if s.analyticsReader == nil {
		return &activity.AnalyticsData{
			TopMedia:       []activity.TopMediaItem{},
			RecentActivity: []activity.RecentActivityEntry{},
		}, nil
	}
	return s.analyticsReader.GetAnalytics(ctx)
}
