package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/lib/pq"
)

type ArtworkRepository struct {
	db *sql.DB
}

func NewArtworkRepository(db *sql.DB) *ArtworkRepository {
	return &ArtworkRepository{db: db}
}

func (r *ArtworkRepository) Create(ctx context.Context, a *metadata.ArtworkRecord) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO artwork (id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, a.MediaItemID, a.PersonID, a.Type, a.Provider, a.SourceURL, a.LocalPath, a.WebPPath, a.Width, a.Height, a.Language, a.SortOrder,
	)
	return err
}

func (r *ArtworkRepository) CreateBatch(ctx context.Context, records []*metadata.ArtworkRecord) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(records) == 0 {
		return nil
	}
	vals := make([]string, 0, len(records))
	args := make([]any, 0, len(records)*12)
	for i, a := range records {
		base := i * 12
		vals = append(vals, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12))
		args = append(args, a.ID, a.MediaItemID, a.PersonID, a.Type, a.Provider, a.SourceURL, a.LocalPath, a.WebPPath, a.Width, a.Height, a.Language, a.SortOrder)
	}
	q := `INSERT INTO artwork (id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order) VALUES ` + strings.Join(vals, ",")
	_, err := r.db.ExecContext(ctx, q, args...)
	return err
}

func (r *ArtworkRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order
		 FROM artwork WHERE media_item_id = $1 ORDER BY sort_order`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanArtworkRows(rows)
}

func (r *ArtworkRepository) GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[string][]*metadata.ArtworkRecord{}, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order
		 FROM artwork WHERE media_item_id = ANY($1) ORDER BY media_item_id, sort_order`, pq.Array(mediaItemIDs))
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
		result[string(a.MediaItemID)] = append(result[string(a.MediaItemID)], a)
	}
	return result, nil
}

func (r *ArtworkRepository) GetByPerson(ctx context.Context, personID string) ([]*metadata.ArtworkRecord, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order
		 FROM artwork WHERE person_id = $1 ORDER BY sort_order`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanArtworkRows(rows)
}

func (r *ArtworkRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `UPDATE artwork SET sort_order = $1 WHERE id = $2`, sortOrder, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) UpdateWebPPath(ctx context.Context, id string, webpPath string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `UPDATE artwork SET webp_path = $1 WHERE id = $2`, webpPath, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM artwork WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM artwork WHERE media_item_id = $1`, mediaItemID)
	return err
}

func (r *ArtworkRepository) scanArtworkRows(rows *sql.Rows) ([]*metadata.ArtworkRecord, error) {
	var result []*metadata.ArtworkRecord
	for rows.Next() {
		a := &metadata.ArtworkRecord{}
		if err := rows.Scan(&a.ID, &a.MediaItemID, &a.PersonID, &a.Type, &a.Provider,
			&a.SourceURL, &a.LocalPath, &a.WebPPath, &a.Width, &a.Height, &a.Language, &a.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

var _ metadata.ArtworkRepository = (*ArtworkRepository)(nil)
