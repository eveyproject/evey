package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// MediaSegmentRepository persists media segments (intro/outro detection results).
type MediaSegmentRepository struct {
	db *sql.DB
}

func NewMediaSegmentRepository(db *sql.DB) *MediaSegmentRepository {
	return &MediaSegmentRepository{db: db}
}

func (r *MediaSegmentRepository) Upsert(ctx context.Context, seg *library.MediaSegment) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	now := time.Now()
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = now
	}
	seg.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (media_item_id, type) DO UPDATE SET
		  start_ms   = EXCLUDED.start_ms,
		  end_ms     = EXCLUDED.end_ms,
		  confidence = EXCLUDED.confidence,
		  source     = EXCLUDED.source,
		  updated_at = EXCLUDED.updated_at`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt, seg.UpdatedAt,
	)
	return err
}

func (r *MediaSegmentRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaSegment, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, media_item_id, type, start_ms, end_ms, confidence, source, created_at, updated_at
		FROM media_segments WHERE media_item_id = $1 ORDER BY start_ms`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var segs []*library.MediaSegment
	for rows.Next() {
		s := &library.MediaSegment{}
		if err := rows.Scan(&s.ID, &s.MediaItemID, &s.Type, &s.StartMs, &s.EndMs, &s.Confidence, &s.Source, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		segs = append(segs, s)
	}
	return segs, rows.Err()
}

func (r *MediaSegmentRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_segments WHERE media_item_id = $1`, mediaItemID)
	return err
}

// Ensure interface compliance at compile time.
var _ library.MediaSegmentRepository = (*MediaSegmentRepository)(nil)
