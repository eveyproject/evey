package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type TrickplayRepository struct {
	db *sql.DB
}

func NewTrickplayRepository(db *sql.DB) *TrickplayRepository {
	return &TrickplayRepository{db: db}
}

func (r *TrickplayRepository) Create(ctx context.Context, m *library.TrickplayManifest) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO trickplay_manifests (id, media_item_id, interval_sec, width, height, tile_width, tile_height, image_count, image_path, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   interval_sec = EXCLUDED.interval_sec,
		   width = EXCLUDED.width,
		   height = EXCLUDED.height,
		   tile_width = EXCLUDED.tile_width,
		   tile_height = EXCLUDED.tile_height,
		   image_count = EXCLUDED.image_count,
		   image_path = EXCLUDED.image_path,
		   created_at = EXCLUDED.created_at`,
		m.ID, m.MediaItemID, m.Interval, m.Width, m.Height, m.TileWidth, m.TileHeight, m.ImageCount, m.ImagePath, m.CreatedAt,
	)
	return err
}

func (r *TrickplayRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) (*library.TrickplayManifest, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	m := &library.TrickplayManifest{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, media_item_id, interval_sec, width, height, tile_width, tile_height, image_count, image_path, created_at
		 FROM trickplay_manifests WHERE media_item_id = $1`, mediaItemID,
	).Scan(&m.ID, &m.MediaItemID, &m.Interval, &m.Width, &m.Height, &m.TileWidth, &m.TileHeight, &m.ImageCount, &m.ImagePath, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	return m, err
}

func (r *TrickplayRepository) Delete(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM trickplay_manifests WHERE media_item_id = $1`, mediaItemID)
	return err
}

func (r *TrickplayRepository) ListMissingMediaItemIDs(ctx context.Context) ([]string, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM media_items mi
		 WHERE mi.file_path != ''
		   AND mi.type IN ('movie', 'episode')
		   AND NOT EXISTS (SELECT 1 FROM trickplay_manifests tm WHERE tm.media_item_id = mi.id)
		 ORDER BY mi.added_at DESC
		 LIMIT 10000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var _ library.TrickplayRepository = (*TrickplayRepository)(nil)
