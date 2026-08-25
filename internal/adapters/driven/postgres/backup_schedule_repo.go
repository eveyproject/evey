package postgres

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type BackupScheduleRepository struct {
	db *sql.DB
}

func NewBackupScheduleRepository(db *sql.DB) *BackupScheduleRepository {
	return &BackupScheduleRepository{db: db}
}

func (r *BackupScheduleRepository) Get(ctx context.Context) (*backup.BackupSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup_schedule.Get")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	s := &backup.BackupSchedule{}
	var lastStatus, lastError sql.NullString
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT cron_expression, enabled, retention_days, last_run_at, next_run_at,
		        last_status, last_error, updated_at
		 FROM backup_schedule WHERE id = TRUE`,
	).Scan(
		&s.CronExpression, &s.Enabled, &s.RetentionDays,
		&s.LastRunAt, &s.NextRunAt,
		&lastStatus, &lastError, &s.UpdatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	s.LastStatus = lastStatus.String
	s.LastError = lastError.String
	return s, nil
}

func (r *BackupScheduleRepository) Update(ctx context.Context, s *backup.BackupSchedule) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup_schedule.Update")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE backup_schedule SET
		   cron_expression = $1,
		   enabled         = $2,
		   retention_days  = $3,
		   next_run_at     = $4,
		   updated_at      = NOW()
		 WHERE id = TRUE`,
		s.CronExpression, s.Enabled, s.RetentionDays, s.NextRunAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// AdvanceNextRun moves the schedule forward at trigger time - the one write
// that keeps a slow backup from being re-fired by every subsequent tick.
func (r *BackupScheduleRepository) AdvanceNextRun(ctx context.Context, nextRun time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup_schedule.AdvanceNextRun")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE backup_schedule SET
		   next_run_at = $1,
		   updated_at  = NOW()
		 WHERE id = TRUE`,
		nextRun,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// RecordRunResult lands the outcome of a finished run. It deliberately does
// NOT touch next_run_at: the schedule advanced when the run fired.
func (r *BackupScheduleRepository) RecordRunResult(ctx context.Context, lastRun time.Time, status, errMsg string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup_schedule.RecordRunResult")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE backup_schedule SET
		   last_run_at = $1,
		   last_status = $2,
		   last_error  = $3,
		   updated_at  = NOW()
		 WHERE id = TRUE`,
		lastRun, status, errMsg,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ backup.ScheduleRepository = (*BackupScheduleRepository)(nil)
