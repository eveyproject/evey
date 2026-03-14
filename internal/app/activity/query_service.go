package activity

import (
	"context"
	"time"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type QueryService struct {
	loginHistory activity.LoginHistoryRepository
	metrics      activity.ServerMetricRepository
	auditLog     activity.AuditLogRepository
	statsReader  activity.PlaySessionStatsReader
	playHistory  activity.PlayHistoryRepository
}

func NewQueryService(
	loginHistory activity.LoginHistoryRepository,
	metrics activity.ServerMetricRepository,
	auditLog activity.AuditLogRepository,
	statsReader activity.PlaySessionStatsReader,
	playHistory activity.PlayHistoryRepository,
) *QueryService {
	return &QueryService{
		loginHistory: loginHistory,
		metrics:      metrics,
		auditLog:     auditLog,
		statsReader:  statsReader,
		playHistory:  playHistory,
	}
}

func (s *QueryService) GetLoginHistory(ctx context.Context, userID string, limit, offset int) ([]*activity.LoginEntry, error) {
	return s.loginHistory.GetByUserID(ctx, userID, limit, offset)
}

func (s *QueryService) GetServerMetrics(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	return s.metrics.GetByTimeRange(ctx, from, to)
}

func (s *QueryService) GetAuditLog(ctx context.Context, filter activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	return s.auditLog.List(ctx, filter)
}

func (s *QueryService) GetUserStats(ctx context.Context, profileID string) (*activity.UserStats, error) {
	return s.statsReader.GetUserStats(ctx, profileID)
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
