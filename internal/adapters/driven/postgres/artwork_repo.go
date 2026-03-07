package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/metadata"
)

type ArtworkRepository struct {
	db *sql.DB
}

func NewArtworkRepository(db *sql.DB) *ArtworkRepository {
	return &ArtworkRepository{db: db}
}

func (r *ArtworkRepository) Create(ctx context.Context, a *metadata.ArtworkRecord) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO artwork (id, media_item_id, person_id, type, provider, source_url, local_path, width, height, language, sort_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		a.ID, a.MediaItemID, a.PersonID, a.Type, a.Provider, a.SourceURL, a.LocalPath, a.Width, a.Height, a.Language, a.SortOrder,
	)
	return err
}

func (r *ArtworkRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, width, height, language, sort_order
		 FROM artwork WHERE media_item_id = $1 ORDER BY sort_order`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanArtworkRows(rows)
}

func (r *ArtworkRepository) GetByPerson(ctx context.Context, personID string) ([]*metadata.ArtworkRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, width, height, language, sort_order
		 FROM artwork WHERE person_id = $1 ORDER BY sort_order`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanArtworkRows(rows)
}

func (r *ArtworkRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM artwork WHERE id = $1`, id)
	return err
}

func (r *ArtworkRepository) scanArtworkRows(rows *sql.Rows) ([]*metadata.ArtworkRecord, error) {
	var result []*metadata.ArtworkRecord
	for rows.Next() {
		a := &metadata.ArtworkRecord{}
		if err := rows.Scan(&a.ID, &a.MediaItemID, &a.PersonID, &a.Type, &a.Provider,
			&a.SourceURL, &a.LocalPath, &a.Width, &a.Height, &a.Language, &a.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
