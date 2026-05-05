package activity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(ctx, "app.activity.RecordLogin")
	defer span.End()
	entry.CreatedAt = time.Now()
	return s.loginHistory.Create(ctx, entry)
}

func (s *CommandService) RecordMetric(ctx context.Context, metric *activity.ServerMetric) error {
	ctx, span := tracing.StartSpan(ctx, "app.activity.RecordMetric")
	defer span.End()
	metric.RecordedAt = time.Now()
	return s.metrics.Create(ctx, metric)
}

func (s *CommandService) PurgeOldMetrics(ctx context.Context, before time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "app.activity.PurgeOldMetrics")
	defer span.End()
	return s.metrics.Purge(ctx, before)
}

func (s *CommandService) CreateAuditEntry(ctx context.Context, entry *activity.AuditLogEntry) error {
	ctx, span := tracing.StartSpan(ctx, "app.activity.CreateAuditEntry")
	defer span.End()
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = time.Now()
	return s.auditLog.Create(ctx, entry)
}
