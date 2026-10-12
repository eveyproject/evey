package activity

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"github.com/google/uuid"
)

type CommandService struct {
	loginHistory activity.LoginHistoryRepository
	metrics      activity.ServerMetricRepository
	auditLog     activity.AuditLogRepository
	events       shared.EventBus
	// qoeEvents bounds the CMCD table. Optional (nil = no sweep) because it
	// belongs to another context and only retention is shared.
	qoeEvents activity.QoEEventPurger
	// webhookDeliveries bounds the webhook delivery log. Optional (nil = no
	// sweep), same contract as qoeEvents.
	webhookDeliveries activity.WebhookDeliveryPurger
}

// WithWebhookDeliveryPurger wires the webhook delivery retention sweep.
// Chainable; leaving it unset keeps the previous behaviour (no purge).
func (s *CommandService) WithWebhookDeliveryPurger(p activity.WebhookDeliveryPurger) *CommandService {
	s.webhookDeliveries = p
	return s
}

// WithQoEEventPurger wires the CMCD retention sweep. Chainable; leaving it
// unset keeps the previous behaviour (no QoE purge).
func (s *CommandService) WithQoEEventPurger(p activity.QoEEventPurger) *CommandService {
	s.qoeEvents = p
	return s
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
	// Under "connect as" the entry's user is the impersonated one: name the
	// admin who actually acted, or the trail blames the wrong person.
	if admin := activity.ImpersonatorFrom(ctx); admin != "" {
		details := make(map[string]any, len(entry.Details)+1)
		maps.Copy(details, entry.Details)
		details["impersonated_by"] = admin
		entry.Details = details
	}
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
// fatal - a single bad pass must not kill the daily loop.
// defaultWebhookDeliveryRetentionDays bounds the delivery log when no audit
// retention is configured. The table is a diagnostic, never evidence.
const defaultWebhookDeliveryRetentionDays = 30

func (s *CommandService) runRetention(ctx context.Context, auditDays, metricsDays, qoeDays int) {
	// "Never fatal" covers panics too, not just errors: the caller runs this in a
	// detached daily goroutine, so an unrecovered panic (e.g. a nil repo or a
	// malformed row) would take down the whole process AND stop every future
	// purge. Recover, log, and let the next tick fire.
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "retention purge panic recovered", slog.Any("panic", r))
		}
	}()
	now := time.Now()
	if auditDays > 0 {
		if n, err := s.PurgeOldAuditLogs(ctx, now.AddDate(0, 0, -auditDays)); err != nil {
			slog.WarnContext(ctx, "audit-log retention purge failed", slog.Any("err", err))
		} else if n > 0 {
			slog.InfoContext(ctx, "audit-log retention purge", slog.Int64("removed", n), slog.Int("retention_days", auditDays))
		}
	}
	if metricsDays > 0 {
		if err := s.PurgeOldMetrics(ctx, now.AddDate(0, 0, -metricsDays)); err != nil {
			slog.WarnContext(ctx, "server-metric retention purge failed", slog.Any("err", err))
		}
	}
	// Webhook deliveries ride the audit-log retention window: both are operator
	// diagnostics with the same "keep enough to investigate, not forever" shape.
	// Defaults to 30 days when audit retention is disabled, because unlike
	// audit_log this table has no compliance argument for keeping it.
	if s.webhookDeliveries != nil {
		days := auditDays
		if days <= 0 {
			days = defaultWebhookDeliveryRetentionDays
		}
		if n, err := s.webhookDeliveries.Purge(ctx, now.AddDate(0, 0, -days)); err != nil {
			slog.WarnContext(ctx, "webhook-delivery retention purge failed", slog.Any("err", err))
		} else if n > 0 {
			slog.InfoContext(ctx, "webhook-delivery retention purge", slog.Int64("removed", n), slog.Int("retention_days", days))
		}
	}
	if qoeDays > 0 && s.qoeEvents != nil {
		if n, err := s.qoeEvents.Purge(ctx, now.AddDate(0, 0, -qoeDays)); err != nil {
			slog.WarnContext(ctx, "qoe retention purge failed", slog.Any("err", err))
		} else if n > 0 {
			slog.InfoContext(ctx, "qoe retention purge", slog.Int64("removed", n), slog.Int("retention_days", qoeDays))
		}
	}
}

// StartRetentionPurge spawns a daily background sweep that trims audit-log
// and server-metric rows past their retention window. The windows come from a
// getter re-read at EVERY pass, so an admin change from the settings panel
// takes effect at the next sweep without a restart - which is also why the
// goroutine spawns even when every window is currently <= 0 (disabled): a
// window enabled at runtime must start purging without a reboot. A retention
// of <= 0 skips that table for the pass. A nil getter disables the sweep.
// It runs one pass at startup, then every 24h, and exits when ctx is done.
func (s *CommandService) StartRetentionPurge(ctx context.Context, retention func() (auditDays, metricsDays, qoeDays int)) {
	if retention == nil {
		slog.InfoContext(ctx, "retention purge disabled: no retention getter wired")
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "retention purge panic recovered", slog.Any("panic", r))
			}
		}()
		pass := func() {
			auditDays, metricsDays, qoeDays := retention()
			s.runRetention(ctx, auditDays, metricsDays, qoeDays)
		}
		pass()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pass()
			}
		}
	}()
}
