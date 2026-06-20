package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type MediaSegmentRepository struct {
	db *sql.DB
}

func NewMediaSegmentRepository(db *sql.DB) *MediaSegmentRepository {
	return &MediaSegmentRepository{db: db}
}

func (r *MediaSegmentRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, type, start_ms, end_ms, confidence, source, created_at
		 FROM media_segments WHERE media_item_id = $1 ORDER BY start_ms`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaSegment
	for rows.Next() {
		seg := &library.MediaSegment{}
		if err := rows.Scan(&seg.ID, &seg.MediaItemID, &seg.Type, &seg.StartMs, &seg.EndMs, &seg.Confidence, &seg.Source, &seg.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, seg)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaSegmentRepository) Upsert(ctx context.Context, seg *library.MediaSegment) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(seg.MediaItemID)), attribute.String("segment.type", seg.Type))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = time.Now()
	}

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (media_item_id, type) DO UPDATE SET
		   start_ms   = EXCLUDED.start_ms,
		   end_ms     = EXCLUDED.end_ms,
		   confidence = EXCLUDED.confidence,
		   source     = EXCLUDED.source`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// UpsertUnlessManual inserts the segment, or updates the existing
// (media_item_id, type) row only when its source is not an operator mark. The
// ON CONFLICT ... DO UPDATE ... WHERE clause evaluates the manual-lock test
// against the locked existing row, so this is atomic: a concurrent
// auto-detection can't read "no manual lock" and then overwrite a manual mark
// set in the race window (the read-then-write that GetByMediaItemID + Upsert
// allowed). Returns applied=false when the existing row's manual lock skipped
// the update.
func (r *MediaSegmentRepository) UpsertUnlessManual(ctx context.Context, seg *library.MediaSegment) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.UpsertUnlessManual")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(seg.MediaItemID)), attribute.String("segment.type", seg.Type))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = time.Now()
	}

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (media_item_id, type) DO UPDATE SET
		   start_ms   = EXCLUDED.start_ms,
		   end_ms     = EXCLUDED.end_ms,
		   confidence = EXCLUDED.confidence,
		   source     = EXCLUDED.source
		 WHERE media_segments.source NOT IN ('manual', 'manual_propagated')`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpsertUnlessOperatorManual is UpsertUnlessManual that locks ONLY against the
// operator's hard 'manual' mark — 'manual_propagated' rows ARE overwritable. The
// intro propagation path needs this: it writes 'manual_propagated' and must be
// able to refresh its own previous propagation on a re-run, while the atomic
// WHERE still closes the TOCTOU against an operator 'manual' that lands between
// the propagation read-guard and this write. Returns applied=false when an
// existing 'manual' mark skipped the update.
func (r *MediaSegmentRepository) UpsertUnlessOperatorManual(ctx context.Context, seg *library.MediaSegment) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.UpsertUnlessOperatorManual")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(seg.MediaItemID)), attribute.String("segment.type", seg.Type))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = time.Now()
	}

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (media_item_id, type) DO UPDATE SET
		   start_ms   = EXCLUDED.start_ms,
		   end_ms     = EXCLUDED.end_ms,
		   confidence = EXCLUDED.confidence,
		   source     = EXCLUDED.source
		 WHERE media_segments.source <> 'manual'`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *MediaSegmentRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_segments WHERE media_item_id = $1`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaSegmentRepository) DeleteByMediaItemIDAndType(ctx context.Context, mediaItemID shared.MediaItemID, segType string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.DeleteByMediaItemIDAndType")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", string(mediaItemID)),
		attribute.String("segment.type", segType),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM media_segments WHERE media_item_id = $1 AND type = $2`,
		string(mediaItemID), segType,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// ListSeasonsMissingSegments returns the parent_ids of seasons whose
// episodes (type='episode') have a trickplay manifest but no media_segments
// rows yet. Drives the startup backfill — only returns seasons that can
// actually be detected (trickplay required by the pHash pass).
func (r *MediaSegmentRepository) ListSeasonsMissingSegments(ctx context.Context) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.ListSeasonsMissingSegments")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT DISTINCT mi.parent_id
		FROM media_items mi
		JOIN trickplay_manifests tm ON tm.media_item_id = mi.id
		LEFT JOIN media_segments ms ON ms.media_item_id = mi.id
		WHERE mi.type = 'episode'
		  AND mi.parent_id <> ''
		  AND ms.id IS NULL
	`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return ids, nil
}

var _ library.MediaSegmentRepository = (*MediaSegmentRepository)(nil)
