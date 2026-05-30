package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// TranscodeCapsOverrideRepo persists CapOverride rows in the
// transcode_caps_overrides table created by migration 000106.
type TranscodeCapsOverrideRepo struct {
	db *sql.DB
}

func NewTranscodeCapsOverrideRepo(db *sql.DB) *TranscodeCapsOverrideRepo {
	return &TranscodeCapsOverrideRepo{db: db}
}

func (r *TranscodeCapsOverrideRepo) Upsert(ctx context.Context, o transcoding.CapOverride) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_caps_override.Upsert")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `
		INSERT INTO transcode_caps_overrides
			(source_class, max_height, reason, starved_ratio, samples_count, applied_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (source_class) DO UPDATE SET
			max_height    = EXCLUDED.max_height,
			reason        = EXCLUDED.reason,
			starved_ratio = EXCLUDED.starved_ratio,
			samples_count = EXCLUDED.samples_count,
			applied_at    = EXCLUDED.applied_at,
			expires_at    = EXCLUDED.expires_at`
	_, err := r.db.ExecContext(ctx, q,
		o.SourceClass, o.MaxHeight, o.Reason,
		o.StarvedRatio, o.SamplesCount,
		o.AppliedAt, o.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("transcode caps upsert: %w", err)
	}
	return nil
}

func (r *TranscodeCapsOverrideRepo) List(ctx context.Context) ([]transcoding.CapOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_caps_override.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `
		SELECT source_class, max_height, reason, starved_ratio, samples_count, applied_at, expires_at
		FROM transcode_caps_overrides
		WHERE expires_at > NOW()`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("transcode caps list: %w", err)
	}
	defer rows.Close()

	var out []transcoding.CapOverride
	for rows.Next() {
		var o transcoding.CapOverride
		if err := rows.Scan(&o.SourceClass, &o.MaxHeight, &o.Reason,
			&o.StarvedRatio, &o.SamplesCount, &o.AppliedAt, &o.ExpiresAt); err != nil {
			return nil, fmt.Errorf("transcode caps scan: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *TranscodeCapsOverrideRepo) DeleteExpired(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_caps_override.DeleteExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `DELETE FROM transcode_caps_overrides WHERE expires_at <= NOW()`
	res, err := r.db.ExecContext(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("transcode caps delete expired: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
