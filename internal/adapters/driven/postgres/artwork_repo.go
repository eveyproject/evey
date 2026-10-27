package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ArtworkRepository struct {
	db *sql.DB
}

func NewArtworkRepository(db *sql.DB) *ArtworkRepository {
	return &ArtworkRepository{db: db}
}

func (r *ArtworkRepository) Create(ctx context.Context, a *metadata.ArtworkRecord) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.Create")
	defer span.End()
	span.SetAttributes(attribute.String("artwork.id", a.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO artwork (id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		a.ID, a.MediaItemID, a.PersonID, a.Type, a.Provider, a.SourceURL, a.LocalPath, a.WebPPath, a.Width, a.Height, a.Language, a.SortOrder, a.Blurhash,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ArtworkRepository) CreateBatch(ctx context.Context, records []*metadata.ArtworkRecord) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.CreateBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("artwork.count", len(records)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(records) == 0 {
		return nil
	}
	vals := make([]string, 0, len(records))
	args := make([]any, 0, len(records)*13)
	for i, a := range records {
		base := i * 13
		vals = append(vals, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12, base+13))
		args = append(args, a.ID, a.MediaItemID, a.PersonID, a.Type, a.Provider, a.SourceURL, a.LocalPath, a.WebPPath, a.Width, a.Height, a.Language, a.SortOrder, a.Blurhash)
	}
	q := `INSERT INTO artwork (id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash) VALUES ` + strings.Join(vals, ",")
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, q, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	// New artwork is a change of its items (see UpdateLocalPath).
	seen := make(map[string]bool, len(records))
	itemIDs := make([]string, 0, len(records))
	for _, a := range records {
		if id := string(a.MediaItemID); id != "" && !seen[id] {
			seen[id] = true
			itemIDs = append(itemIDs, id)
		}
	}
	if len(itemIDs) > 0 {
		if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
			`UPDATE media_items SET updated_at = NOW() WHERE id = ANY($1)`, pq.Array(itemIDs)); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	return nil
}

func (r *ArtworkRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.GetByMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash
		 FROM artwork WHERE media_item_id = $1 ORDER BY sort_order`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := r.scanArtworkRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *ArtworkRepository) GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.GetByMediaItems")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[string][]*metadata.ArtworkRecord{}, nil
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash
		 FROM artwork WHERE media_item_id = ANY($1) ORDER BY media_item_id, sort_order`, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	records, err := r.scanArtworkRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	result := make(map[string][]*metadata.ArtworkRecord, len(mediaItemIDs))
	for _, a := range records {
		result[string(a.MediaItemID)] = append(result[string(a.MediaItemID)], a)
	}
	return result, nil
}

func (r *ArtworkRepository) GetByPerson(ctx context.Context, personID string) ([]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.GetByPerson")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", personID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash
		 FROM artwork WHERE person_id = $1 ORDER BY sort_order`, personID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := r.scanArtworkRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *ArtworkRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.UpdateSortOrder")
	defer span.End()
	span.SetAttributes(attribute.String("artwork.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE artwork SET sort_order = $1 WHERE id = $2`, sortOrder, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) ListMissingBlurhash(ctx context.Context) ([]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.ListMissingBlurhash")
	defer span.End()
	ctx, cancel := withTimeout(ctx, 2*defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash
		 FROM artwork WHERE local_path IS NOT NULL AND local_path <> '' AND blurhash IS NULL ORDER BY id`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := r.scanArtworkRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *ArtworkRepository) ListMissingLocalPath(ctx context.Context) ([]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.ListMissingLocalPath")
	defer span.End()
	ctx, cancel := withTimeout(ctx, 2*defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, person_id, type, provider, source_url, local_path, webp_path, width, height, language, sort_order, blurhash
		 FROM artwork WHERE source_url IS NOT NULL AND (local_path IS NULL OR local_path = '') ORDER BY id`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := r.scanArtworkRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *ArtworkRepository) LocalPathStats(ctx context.Context) (metadata.ArtworkStats, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.LocalPathStats")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var stats metadata.ArtworkStats
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT
			COUNT(*) AS total,
			COUNT(CASE WHEN local_path IS NOT NULL AND local_path <> '' THEN 1 END) AS with_local,
			COUNT(CASE WHEN local_path IS NULL OR local_path = '' THEN 1 END) AS missing_local
		 FROM artwork`)
	if err := row.Scan(&stats.TotalRecords, &stats.WithLocalPath, &stats.MissingLocalPath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return stats, err
	}
	// TotalSizeBytes is not tracked in the DB (would require a dedicated column).
	// The frontend receives 0 and can display it as "N/A" or omit it.
	return stats, nil
}

func (r *ArtworkRepository) UpdateLocalPath(ctx context.Context, id string, localPath string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.UpdateLocalPath")
	defer span.End()
	span.SetAttributes(attribute.String("artwork.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE artwork SET local_path = $1 WHERE id = $2`, localPath, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	// A poster that just landed is a change of the item: the clients version
	// their artwork URLs by the item's updated_at, and Android's image cache
	// had kept the "no artwork" answer for the old URL for good.
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_items SET updated_at = NOW() WHERE id = (SELECT media_item_id FROM artwork WHERE id = $1)`, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ArtworkRepository) UpdateWebPPath(ctx context.Context, id string, webpPath string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.UpdateWebPPath")
	defer span.End()
	span.SetAttributes(attribute.String("artwork.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE artwork SET webp_path = $1 WHERE id = $2`, webpPath, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) UpdateBlurhash(ctx context.Context, id string, blurhash string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.UpdateBlurhash")
	defer span.End()
	span.SetAttributes(attribute.String("artwork.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE artwork SET blurhash = $1 WHERE id = $2`, blurhash, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("artwork.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM artwork WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *ArtworkRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM artwork WHERE media_item_id = $1`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// DeleteByMediaItemIDs is the batch form of DeleteByMediaItemID: one
// statement for a whole level of items instead of one round-trip per item.
func (r *ArtworkRepository) DeleteByMediaItemIDs(ctx context.Context, mediaItemIDs []string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.artwork.DeleteByMediaItemIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	if len(mediaItemIDs) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM artwork WHERE media_item_id = ANY($1)`, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ArtworkRepository) scanArtworkRows(rows *sql.Rows) ([]*metadata.ArtworkRecord, error) {
	var result []*metadata.ArtworkRecord
	for rows.Next() {
		a := &metadata.ArtworkRecord{}
		if err := rows.Scan(&a.ID, &a.MediaItemID, &a.PersonID, &a.Type, &a.Provider,
			&a.SourceURL, &a.LocalPath, &a.WebPPath, &a.Width, &a.Height, &a.Language, &a.SortOrder, &a.Blurhash); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

var (
	_ metadata.ArtworkCommandRepo = (*ArtworkRepository)(nil)
	_ metadata.ArtworkQueryRepo   = (*ArtworkRepository)(nil)
)
