package activity

import (
	"context"
	"log/slog"
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

// PurgeOldAuditLogs deletes audit-log entries older than before. The
// audit_log table is append-only in normal operation (no per-row delete),
// so this retention sweep is the only thing that bounds its growth.
func (s *CommandService) PurgeOldAuditLogs(ctx context.Context, before time.Time) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "app.activity.PurgeOldAuditLogs")
	defer span.End()
	return s.auditLog.Purge(ctx, before)
}

// runRetention performs one purge pass for whichever tables have a positive
// retention window (days <= 0 = keep forever). Failures are logged, never
// fatal — a single bad pass must not kill the daily loop.
func (s *CommandService) runRetention(ctx context.Context, auditDays, metricsDays int) {
	// "Never fatal" covers panics too, not just errors: the caller runs this in a
	// detached daily goroutine, so an unrecovered panic (e.g. a nil repo or a
	// malformed row) would take down the whole process AND stop every future
	// purge. Recover, log, and let the next tick fire.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("retention purge panic recovered", slog.Any("panic", r))
		}
	}()
	now := time.Now()
	if auditDays > 0 {
		if n, err := s.PurgeOldAuditLogs(ctx, now.AddDate(0, 0, -auditDays)); err != nil {
			slog.Warn("audit-log retention purge failed", slog.Any("err", err))
		} else if n > 0 {
			slog.Info("audit-log retention purge", slog.Int64("removed", n), slog.Int("retention_days", auditDays))
		}
	}
	if metricsDays > 0 {
		if err := s.PurgeOldMetrics(ctx, now.AddDate(0, 0, -metricsDays)); err != nil {
			slog.Warn("server-metric retention purge failed", slog.Any("err", err))
		}
	}
}

// StartRetentionPurge spawns a daily background sweep that trims audit-log
// and server-metric rows past their retention window. A retention of <= 0
// disables that table; when both are disabled this is a no-op (no goroutine).
// It runs one pass at startup, then every 24h, and exits when ctx is done.
func (s *CommandService) StartRetentionPurge(ctx context.Context, auditDays, metricsDays int) {
	if auditDays <= 0 && metricsDays <= 0 {
		slog.Info("retention purge disabled", slog.Int("audit_days", auditDays), slog.Int("metrics_days", metricsDays))
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("retention purge panic recovered", slog.Any("panic", r))
			}
		}()
		s.runRetention(ctx, auditDays, metricsDays)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runRetention(ctx, auditDays, metricsDays)
			}
		}
	}()
}
