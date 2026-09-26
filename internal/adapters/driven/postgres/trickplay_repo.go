package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type TrickplayRepository struct {
	db *sql.DB
}

func NewTrickplayRepository(db *sql.DB) *TrickplayRepository {
	return &TrickplayRepository{db: db}
}

func (r *TrickplayRepository) Create(ctx context.Context, m *library.TrickplayManifest) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.trickplay.Create")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(m.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO trickplay_manifests (id, media_item_id, interval_sec, width, height, tile_width, tile_height, image_count, image_path, algo_version, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   interval_sec = EXCLUDED.interval_sec,
		   width = EXCLUDED.width,
		   height = EXCLUDED.height,
		   tile_width = EXCLUDED.tile_width,
		   tile_height = EXCLUDED.tile_height,
		   image_count = EXCLUDED.image_count,
		   image_path = EXCLUDED.image_path,
		   algo_version = EXCLUDED.algo_version,
		   created_at = EXCLUDED.created_at`,
		m.ID, m.MediaItemID, m.Interval, m.Width, m.Height, m.TileWidth, m.TileHeight, m.ImageCount, m.ImagePath, m.AlgoVersion, m.CreatedAt,
	)
	if err != nil {
		// A foreign-key violation here means the media item was deleted while
		// the (minutes-long) trickplay generation was running: the manifest has
		// nothing to attach to. The caller treats this as a skip, not a
		// failure, so classify it rather than surfacing a raw pq error.
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return library.ErrMediaItemGone
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	// A success settles the question a recorded failure asked - without this,
	// deleting the manifest later would leave the item suppressed forever.
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM trickplay_failures WHERE media_item_id = $1`, string(m.MediaItemID)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// RecordFailure remembers that generation failed against this exact file so
// ListMissingMediaItemIDs stops re-serving the item while the media_files row
// still matches the recorded (size, mod time) pair. Upsert: a later failure
// against replaced bytes overwrites the stale record.
func (r *TrickplayRepository) RecordFailure(ctx context.Context, mediaItemID shared.MediaItemID, fileSize int64, modTime time.Time, reason string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.trickplay.RecordFailure")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO trickplay_failures (media_item_id, file_size, mod_time, reason)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   file_size = EXCLUDED.file_size,
		   mod_time = EXCLUDED.mod_time,
		   reason = EXCLUDED.reason,
		   failed_at = now()`,
		string(mediaItemID), fileSize, modTime, reason)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *TrickplayRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) (*library.TrickplayManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.trickplay.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	m := &library.TrickplayManifest{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, media_item_id, interval_sec, width, height, tile_width, tile_height, image_count, image_path, algo_version, created_at
		 FROM trickplay_manifests WHERE media_item_id = $1`, mediaItemID,
	).Scan(&m.ID, &m.MediaItemID, &m.Interval, &m.Width, &m.Height, &m.TileWidth, &m.TileHeight, &m.ImageCount, &m.ImagePath, &m.AlgoVersion, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return m, nil
}

// GetByMediaItemIDs batches GetByMediaItemID for many items into a single
// query, keyed by media item id. An id with no manifest is simply absent from
// the returned map. Used by intro propagation's sibling sweeps to replace a
// per-sibling GetByMediaItemID call with one query for the whole season page.
func (r *TrickplayRepository) GetByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID]*library.TrickplayManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.trickplay.GetByMediaItemIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[shared.MediaItemID]*library.TrickplayManifest{}, nil
	}
	ids := make([]string, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		ids[i] = string(id)
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, interval_sec, width, height, tile_width, tile_height, image_count, image_path, algo_version, created_at
		 FROM trickplay_manifests WHERE media_item_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[shared.MediaItemID]*library.TrickplayManifest, len(mediaItemIDs))
	for rows.Next() {
		m := &library.TrickplayManifest{}
		if err := rows.Scan(&m.ID, &m.MediaItemID, &m.Interval, &m.Width, &m.Height, &m.TileWidth, &m.TileHeight, &m.ImageCount, &m.ImagePath, &m.AlgoVersion, &m.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[m.MediaItemID] = m
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *TrickplayRepository) Delete(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.trickplay.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM trickplay_manifests WHERE media_item_id = $1`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *TrickplayRepository) ListMissingMediaItemIDs(ctx context.Context, minAlgoVersion int) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.trickplay.ListMissingMediaItemIDs")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.id FROM media_items mi
		 INNER JOIN media_files mf ON mf.media_item_id = mi.id
		 WHERE `+trickplayEligibleSQL+`
		   AND `+trickplayPendingSQL("$1")+`
		 ORDER BY mi.added_at DESC
		 LIMIT 10000`, minAlgoVersion)
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

var _ library.TrickplayRepository = (*TrickplayRepository)(nil)
