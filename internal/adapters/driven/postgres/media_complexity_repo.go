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
			// Don't fail Get on bad JSON - surface class anyway. Operator
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

	// An empty class is neither NULL (ListUnclassified) nor a classified row
	// (ListProbeCandidates): the title would leave both queues for good.
	if class == "" {
		return fmt.Errorf("complexity Set: empty class for media item %q", mediaItemID)
	}
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
	// Only items the classifier can actually sample: a video file with a known
	// duration. Tracks/photos and zero-duration rows made the worker skip
	// without persisting, so this query re-served the same IDs on every batch,
	// forever (prod 2026-07-26: a continuous silent retry loop). The type list
	// mirrors trickplay's ListMissingMediaItemIDs; a zero duration re-enters
	// the candidate set by itself once a later scan fills it in.
	const q = `
		SELECT mi.id
		FROM media_items mi
		INNER JOIN media_files mf ON mf.media_item_id = mi.id
		WHERE mi.complexity_class IS NULL
		  AND mf.file_path <> ''
		  AND mi.type IN ('movie', 'episode')
		  AND mf.duration_ms > 0
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

// ListProbeCandidates returns up to `limit` media IDs whose PERSISTED
// complexity makes them a per-title probe candidate (plan P3): the video item
// was never classified, OR its motion is outside the cost model's fitted
// range, OR it is the animation weak spot - and it has NOT already been probed
// at the current model version. Ceiling-hit candidates come from the runtime
// signal store, not this query. The motion bounds match CostModelV2's
// YDIFMin/YDIFMax; they are passed so a model recalibration re-selects
// correctly without a schema change.
func (r *MediaComplexityRepo) ListProbeCandidates(ctx context.Context, modelVersion, limit int) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.complexity.ListProbeCandidates")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	ydifMin := domaintranscoding.CostModelV2.YDIFMin
	ydifMax := domaintranscoding.CostModelV2.YDIFMax
	minFrames := domaintranscoding.CostModelV2.MinFrames
	protocol := domaintranscoding.TitleProbeProtocol
	encoder := domaintranscoding.TitleProbeEncoderID
	// A row is "already probed for this identity" when its metrics carry a probe
	// stamped with the current protocol AND model version AND encoder (failed or
	// not - a failed probe must not be retried until an identity bump). This
	// mirrors TitleProbe.CurrentIdentity so the SQL queue and the in-memory
	// ProbeCandidate agree: a bump of the protocol or the encoder tuning alone -
	// not just the model version - re-queues every title, instead of leaving a
	// stale ground truth in place because model_version happened to still match.
	//
	// A never-classified title (no complexity row, or a pre-canonical one) is
	// NOT here: the cheap classifier handles it first, and the probe fires only
	// if the classifier's result then trips a trigger. The triggers are: the
	// classifier ran but sampled no motion (a canonical row with a class yet
	// frames < MinFrames - the sample-free fallback), the animation weak spot,
	// or a motion outside the model's fitted range. Ceiling hits come from the
	// runtime signal store, not this query.
	const q = `
		SELECT mi.id
		FROM media_items mi
		INNER JOIN media_files mf ON mf.media_item_id = mi.id
		WHERE mf.file_path <> ''
		  AND mi.type IN ('movie', 'episode')
		  AND mf.duration_ms > 0
		  AND mi.complexity_class IS NOT NULL
		  AND COALESCE((mi.complexity_metrics->>'metric_version')::int, 1) >= 2
		  AND NOT (
		        COALESCE((mi.complexity_metrics->'probe'->>'protocol')::int, 0) = $6
		    AND COALESCE((mi.complexity_metrics->'probe'->>'model_version')::int, 0) = $1
		    AND COALESCE(mi.complexity_metrics->'probe'->>'encoder', '') = $7
		      )
		  AND (
		        COALESCE((mi.complexity_metrics->>'frames_sampled')::int, 0) < $4
		     OR mi.complexity_class = 'animation'
		     OR (
		          COALESCE((mi.complexity_metrics->>'frames_sampled')::int, 0) >= $4
		          AND (
		               COALESCE((mi.complexity_metrics->>'motion_mean')::float, 0) < $2
		            OR COALESCE((mi.complexity_metrics->>'motion_mean')::float, 0) > $3
		          )
		        )
		      )
		ORDER BY mi.added_at ASC
		LIMIT $5`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, modelVersion, ydifMin, ydifMax, minFrames, limit, protocol, encoder)
	if err != nil {
		return nil, fmt.Errorf("complexity ListProbeCandidates: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("complexity ListProbeCandidates scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
