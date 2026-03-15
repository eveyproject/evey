package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type ScanScheduleRepository struct {
	db *sql.DB
}

func NewScanScheduleRepository(db *sql.DB) *ScanScheduleRepository {
	return &ScanScheduleRepository{db: db}
}

func (r *ScanScheduleRepository) Upsert(ctx context.Context, s *library.ScanSchedule) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO scan_schedules (library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (library_id) DO UPDATE SET
		   cron_expression = EXCLUDED.cron_expression,
		   enabled = EXCLUDED.enabled,
		   next_run_at = EXCLUDED.next_run_at,
		   updated_at = NOW()`,
		s.LibraryID, s.CronExpression, s.Enabled, s.LastRunAt, s.NextRunAt,
	)
	return err
}

func (r *ScanScheduleRepository) GetByLibraryID(ctx context.Context, libraryID string) (*library.ScanSchedule, error) {
	s := &library.ScanSchedule{}
	err := r.db.QueryRowContext(ctx,
		`SELECT library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM scan_schedules WHERE library_id = $1`, libraryID,
	).Scan(&s.LibraryID, &s.CronExpression, &s.Enabled, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	return s, err
}

func (r *ScanScheduleRepository) Delete(ctx context.Context, libraryID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM scan_schedules WHERE library_id = $1`, libraryID)
	return err
}

func (r *ScanScheduleRepository) List(ctx context.Context) ([]*library.ScanSchedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM scan_schedules ORDER BY library_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*library.ScanSchedule
	for rows.Next() {
		s := &library.ScanSchedule{}
		if err := rows.Scan(&s.LibraryID, &s.CronExpression, &s.Enabled, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *ScanScheduleRepository) ListEnabled(ctx context.Context) ([]*library.ScanSchedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT library_id, cron_expression, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM scan_schedules WHERE enabled = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*library.ScanSchedule
	for rows.Next() {
		s := &library.ScanSchedule{}
		if err := rows.Scan(&s.LibraryID, &s.CronExpression, &s.Enabled, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *ScanScheduleRepository) UpdateAfterRun(ctx context.Context, libraryID string, lastRun, nextRun time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE scan_schedules SET last_run_at = $1, next_run_at = $2, updated_at = NOW() WHERE library_id = $3`,
		lastRun, nextRun, libraryID,
	)
	return err
}
