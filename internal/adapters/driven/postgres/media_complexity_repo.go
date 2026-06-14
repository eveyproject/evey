package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// MediaComplexityRepo persists per-title classification rows on the
// media_items table (columns complexity_class, complexity_metrics,
// complexity_computed_at added by migration 105).
type MediaComplexityRepo struct {
	db *sql.DB
}

func NewMediaComplexityRepo(db *sql.DB) *MediaComplexityRepo {
	return &MediaComplexityRepo{db: db}
}

var _ domaintranscoding.ComplexityRepo = (*MediaComplexityRepo)(nil)

func (r *MediaComplexityRepo) Get(ctx context.Context, mediaItemID string) (domaintranscoding.ComplexityEntry, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.complexity.Get")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var (
		entry      domaintranscoding.ComplexityEntry
		class      sql.NullString
		metricsRaw sql.NullString
		computedAt sql.NullTime
	)
	const q = `
		SELECT complexity_class, complexity_metrics::text, complexity_computed_at
		FROM media_items
		WHERE id = $1`
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, q, mediaItemID).Scan(&class, &metricsRaw, &computedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entry, false, nil
		}
		return entry, false, fmt.Errorf("complexity Get: %w", err)
	}
	if !class.Valid {
		return entry, false, nil
	}
	entry.Class = domaintranscoding.ContentClass(class.String)
	if metricsRaw.Valid && metricsRaw.String != "" {
		if jerr := json.Unmarshal([]byte(metricsRaw.String), &entry.Metrics); jerr != nil {
			// Don't fail Get on bad JSON — surface class anyway. Operator
			// can reclassify to refresh.
			_ = jerr
		}
	}
	if computedAt.Valid {
		entry.ComputedAt = computedAt.Time
	}
	return entry, true, nil
}

func (r *MediaComplexityRepo) Set(ctx context.Context, mediaItemID string, class domaintranscoding.ContentClass, metrics domaintranscoding.ComplexityMetrics, computedAt time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.complexity.Set")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	metricsJSON, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("complexity Set: marshal metrics: %w", err)
	}
	const q = `
		UPDATE media_items
		SET complexity_class = $1, complexity_metrics = $2::jsonb, complexity_computed_at = $3
		WHERE id = $4`
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, q, string(class), string(metricsJSON), computedAt, mediaItemID)
	if err != nil {
		return fmt.Errorf("complexity Set: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("complexity Set: no media item with id %q", mediaItemID)
	}
	return nil
}

func (r *MediaComplexityRepo) ListUnclassified(ctx context.Context, limit int) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.complexity.ListUnclassified")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	// Only items that actually have a file the classifier can probe —
	// no file = nothing to sample.
	const q = `
		SELECT mi.id
		FROM media_items mi
		INNER JOIN media_files mf ON mf.media_item_id = mi.id
		WHERE mi.complexity_class IS NULL
		  AND mf.file_path <> ''
		ORDER BY mi.added_at ASC
		LIMIT $1`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("complexity ListUnclassified: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("complexity ListUnclassified scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
