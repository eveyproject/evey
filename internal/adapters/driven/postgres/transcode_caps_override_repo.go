package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// TranscodeCapsOverrideRepo persists CapOverride rows in the
// transcode_caps_overrides table created by migration 000106.
type TranscodeCapsOverrideRepo struct {
	db *sql.DB
}

func NewTranscodeCapsOverrideRepo(db *sql.DB) *TranscodeCapsOverrideRepo {
	return &TranscodeCapsOverrideRepo{db: db}
}

// AggregateStarvation runs the CMCD/QoE feedback aggregation (Phase C): per
// (codec, resolution) source-class bucket over `window`, the count of
// buffer-starved events vs total, keeping only buckets with at least
// minSamples events. The codec CASE mirrors the app-layer normaliseSourceCodec
// so the bucket key matches what the handler looks up at request time. Lives
// here (not the app service) so the app layer holds no raw *sql.DB.
func (r *TranscodeCapsOverrideRepo) AggregateStarvation(ctx context.Context, window time.Duration, minSamples int) ([]transcoding.CapAggregateRow, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_caps_override.AggregateStarvation")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	// $1::interval - pass an "N seconds" literal and let pg parse it.
	winSecs := max(int(window.Seconds()), 1)

	const q = `
		WITH source_meta AS (
			SELECT
				mi.id AS media_item_id,
				MAX(ms.height) AS height,
				LOWER(MAX(ms.codec) FILTER (WHERE ms.stream_type = 'video')) AS codec
			FROM media_items mi
			JOIN media_streams ms ON ms.media_item_id = mi.id AND ms.stream_type = 'video'
			GROUP BY mi.id
		), normalised AS (
			SELECT
				sm.media_item_id,
				sm.height,
				CASE
					WHEN sm.codec IN ('h264', 'avc', 'avc1')          THEN 'h264'
					WHEN sm.codec IN ('hevc', 'h265', 'hev1', 'hvc1') THEN 'hevc'
					WHEN sm.codec IN ('av1', 'av01')                  THEN 'av1'
					WHEN sm.codec = 'vp9'                              THEN 'vp9'
					ELSE 'any'
				END AS codec_bucket
			FROM source_meta sm
		), buckets AS (
			SELECT
				CASE
					WHEN nm.height >= 2160 THEN nm.codec_bucket || '/2160p'
					WHEN nm.height >= 1440 THEN nm.codec_bucket || '/1440p'
					WHEN nm.height >= 1080 THEN nm.codec_bucket || '/1080p'
					ELSE NULL
				END AS source_class,
				COUNT(*) FILTER (WHERE pe.buffer_starved = TRUE) AS starved,
				COUNT(*)                                          AS total
			FROM playback_qoe_events pe
			JOIN normalised nm ON nm.media_item_id = pe.media_item_id
			WHERE pe.ts >= NOW() - $1::interval
			GROUP BY source_class
		)
		SELECT source_class, starved, total
		FROM buckets
		WHERE source_class IS NOT NULL
		  AND total >= $2`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, fmt.Sprintf("%d seconds", winSecs), minSamples)
	if err != nil {
		return nil, fmt.Errorf("transcode caps aggregate: %w", err)
	}
	defer rows.Close()

	var out []transcoding.CapAggregateRow
	for rows.Next() {
		var row transcoding.CapAggregateRow
		if err := rows.Scan(&row.SourceClass, &row.Starved, &row.Total); err != nil {
			return nil, fmt.Errorf("transcode caps aggregate scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
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
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, q,
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
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q)
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

func (r *TranscodeCapsOverrideRepo) Delete(ctx context.Context, sourceClass string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_caps_override.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `DELETE FROM transcode_caps_overrides WHERE source_class = $1`
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, q, sourceClass)
	if err != nil {
		return false, fmt.Errorf("transcode caps delete: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *TranscodeCapsOverrideRepo) DeleteExpired(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_caps_override.DeleteExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `DELETE FROM transcode_caps_overrides WHERE expires_at <= NOW()`
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("transcode caps delete expired: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
