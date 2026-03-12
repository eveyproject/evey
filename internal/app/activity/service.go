package activity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type Service struct {
	loginHistory activity.LoginHistoryRepository
	metrics      activity.ServerMetricRepository
	auditLog     activity.AuditLogRepository
	statsReader  activity.PlaySessionStatsReader
	playHistory  activity.PlayHistoryRepository
	events       shared.EventBus
}

func NewService(
	loginHistory activity.LoginHistoryRepository,
	metrics activity.ServerMetricRepository,
	auditLog activity.AuditLogRepository,
	statsReader activity.PlaySessionStatsReader,
	playHistory activity.PlayHistoryRepository,
	events shared.EventBus,
) *Service {
	return &Service{
		loginHistory: loginHistory,
		metrics:      metrics,
		auditLog:     auditLog,
		statsReader:  statsReader,
		playHistory:  playHistory,
		events:       events,
	}
}

func (s *Service) RecordLogin(ctx context.Context, entry *activity.LoginEntry) error {
	entry.CreatedAt = time.Now()
	return s.loginHistory.Create(ctx, entry)
}

func (s *Service) GetLoginHistory(ctx context.Context, userID string, limit, offset int) ([]*activity.LoginEntry, error) {
	return s.loginHistory.GetByUserID(ctx, userID, limit, offset)
}

func (s *Service) RecordMetric(ctx context.Context, metric *activity.ServerMetric) error {
	metric.RecordedAt = time.Now()
	return s.metrics.Create(ctx, metric)
}

func (s *Service) GetServerMetrics(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	return s.metrics.GetByTimeRange(ctx, from, to)
}

func (s *Service) PurgeOldMetrics(ctx context.Context, before time.Time) error {
	return s.metrics.Purge(ctx, before)
}

func (s *Service) CreateAuditEntry(ctx context.Context, entry *activity.AuditLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = time.Now()
	return s.auditLog.Create(ctx, entry)
}

func (s *Service) GetAuditLog(ctx context.Context, filter activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	return s.auditLog.List(ctx, filter)
}

func (s *Service) GetUserStats(ctx context.Context, profileID string) (*activity.UserStats, error) {
	return s.statsReader.GetUserStats(ctx, profileID)
}

func (s *Service) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	return s.statsReader.GetServerStats(ctx)
}

func (s *Service) GetPlayHistory(ctx context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	return s.playHistory.List(ctx, filter)
}
