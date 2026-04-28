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

	rows, err := r.db.QueryContext(ctx,
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

	_, err := r.db.ExecContext(ctx,
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

func (r *MediaSegmentRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := r.db.ExecContext(ctx, `DELETE FROM media_segments WHERE media_item_id = $1`, string(mediaItemID))
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

	_, err := r.db.ExecContext(ctx,
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

var _ library.MediaSegmentRepository = (*MediaSegmentRepository)(nil)
