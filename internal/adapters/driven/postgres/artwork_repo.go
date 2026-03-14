package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

func (r *ArtworkRepository) GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error) {
	if len(mediaItemIDs) == 0 {
		return map[string][]*metadata.ArtworkRecord{}, nil
	}
	placeholders := make([]string, len(mediaItemIDs))
	args := make([]any, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, width, height, language, sort_order
		 FROM artwork WHERE media_item_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY media_item_id, sort_order`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records, err := r.scanArtworkRows(rows)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]*metadata.ArtworkRecord, len(mediaItemIDs))
	for _, a := range records {
		result[a.MediaItemID] = append(result[a.MediaItemID], a)
	}
	return result, nil
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

func (r *ArtworkRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE artwork SET sort_order = $1 WHERE id = $2`, sortOrder, id)
	return err
}

func (r *ArtworkRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM artwork WHERE id = $1`, id)
	return err
}

func (r *ArtworkRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM artwork WHERE media_item_id = $1`, mediaItemID)
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
