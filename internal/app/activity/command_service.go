package activity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
)

type CommandService struct {
	loginHistory activity.LoginHistoryRepository
	metrics      activity.ServerMetricRepository
	auditLog     activity.AuditLogRepository
	events       shared.EventBus
}

func NewCommandService(
	loginHistory activity.LoginHistoryRepository,
	metrics activity.ServerMetricRepository,
	auditLog activity.AuditLogRepository,
	events shared.EventBus,
) *CommandService {
	return &CommandService{
		loginHistory: loginHistory,
		metrics:      metrics,
		auditLog:     auditLog,
		events:       events,
	}
}

func (s *CommandService) RecordLogin(ctx context.Context, entry *activity.LoginEntry) error {
	entry.CreatedAt = time.Now()
	return s.loginHistory.Create(ctx, entry)
}

func (s *CommandService) RecordMetric(ctx context.Context, metric *activity.ServerMetric) error {
	metric.RecordedAt = time.Now()
	return s.metrics.Create(ctx, metric)
}

func (s *CommandService) PurgeOldMetrics(ctx context.Context, before time.Time) error {
	return s.metrics.Purge(ctx, before)
}

func (s *CommandService) CreateAuditEntry(ctx context.Context, entry *activity.AuditLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = time.Now()
	return s.auditLog.Create(ctx, entry)
}
