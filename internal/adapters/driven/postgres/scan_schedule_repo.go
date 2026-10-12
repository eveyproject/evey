package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ScanScheduleRepository struct {
	db *sql.DB
}

func NewScanScheduleRepository(db *sql.DB) *ScanScheduleRepository {
	return &ScanScheduleRepository{db: db}
}

func (r *ScanScheduleRepository) Upsert(ctx context.Context, s *library.ScanSchedule) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.scan_schedule.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(s.LibraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO scan_schedules (library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (library_id) DO UPDATE SET
		   cron_expression = EXCLUDED.cron_expression,
		   enabled = EXCLUDED.enabled,
		   next_run_at = EXCLUDED.next_run_at,
		   updated_at = NOW()`,
		s.LibraryID, s.CronExpression, s.Enabled, s.LastRunAt, s.NextRunAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ScanScheduleRepository) GetByLibraryID(ctx context.Context, libraryID string) (*library.ScanSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.scan_schedule.GetByLibraryID")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	s := &library.ScanSchedule{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM scan_schedules WHERE library_id = $1`, libraryID,
	).Scan(&s.LibraryID, &s.CronExpression, &s.Enabled, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return s, nil
}

func (r *ScanScheduleRepository) Delete(ctx context.Context, libraryID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.scan_schedule.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM scan_schedules WHERE library_id = $1`, libraryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ScanScheduleRepository) List(ctx context.Context) ([]*library.ScanSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.scan_schedule.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM scan_schedules ORDER BY library_id`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var result []*library.ScanSchedule
	for rows.Next() {
		s := &library.ScanSchedule{}
		if err := rows.Scan(&s.LibraryID, &s.CronExpression, &s.Enabled, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *ScanScheduleRepository) ListEnabled(ctx context.Context) ([]*library.ScanSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.scan_schedule.ListEnabled")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM scan_schedules WHERE enabled = TRUE ORDER BY library_id LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var result []*library.ScanSchedule
	for rows.Next() {
		s := &library.ScanSchedule{}
		if err := rows.Scan(&s.LibraryID, &s.CronExpression, &s.Enabled, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *ScanScheduleRepository) UpdateAfterRun(ctx context.Context, libraryID string, lastRun, nextRun time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.scan_schedule.UpdateAfterRun")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE scan_schedules SET last_run_at = $1, next_run_at = $2, updated_at = NOW() WHERE library_id = $3`,
		lastRun, nextRun, libraryID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ library.ScanScheduleRepository = (*ScanScheduleRepository)(nil)
