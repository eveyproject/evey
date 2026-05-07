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
	err := r.db.QueryRowContext(ctx,
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
	_, err := r.db.ExecContext(ctx,
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

func (r *BackupScheduleRepository) UpdateAfterRun(ctx context.Context, lastRun, nextRun time.Time, status, errMsg string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup_schedule.UpdateAfterRun")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE backup_schedule SET
		   last_run_at = $1,
		   next_run_at = $2,
		   last_status = $3,
		   last_error  = $4,
		   updated_at  = NOW()
		 WHERE id = TRUE`,
		lastRun, nextRun, status, errMsg,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ backup.ScheduleRepository = (*BackupScheduleRepository)(nil)
