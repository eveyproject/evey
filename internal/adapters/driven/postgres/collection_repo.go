package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type CollectionRepository struct {
	db *sql.DB
}

func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

func (r *CollectionRepository) Create(ctx context.Context, col *library.Collection) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, tmdb_id, poster_url, backdrop_url, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		col.ID, col.Name, col.Overview, col.TmdbID, col.PosterURL, col.BackdropURL,
		col.CreatedAt, col.UpdatedAt,
	)
	return err
}

func (r *CollectionRepository) Upsert(ctx context.Context, col *library.Collection) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, tmdb_id, poster_url, backdrop_url, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tmdb_id) WHERE tmdb_id IS NOT NULL DO UPDATE SET
		   name = EXCLUDED.name,
		   overview = EXCLUDED.overview,
		   poster_url = EXCLUDED.poster_url,
		   backdrop_url = EXCLUDED.backdrop_url,
		   updated_at = EXCLUDED.updated_at`,
		col.ID, col.Name, col.Overview, col.TmdbID, col.PosterURL, col.BackdropURL,
		col.CreatedAt, col.UpdatedAt,
	)
	return err
}

func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	col := &library.Collection{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, created_at, updated_at
		 FROM collections WHERE id = $1`, id,
	).Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
		&col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, library.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return col, nil
}

func (r *CollectionRepository) GetByTmdbID(ctx context.Context, tmdbID int) (*library.Collection, error) {
	col := &library.Collection{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, created_at, updated_at
		 FROM collections WHERE tmdb_id = $1`, tmdbID,
	).Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
		&col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, library.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return col, nil
}

func (r *CollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, created_at, updated_at
		 FROM collections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Collection
	for rows.Next() {
		col := &library.Collection{}
		if err := rows.Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
			&col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, col)
	}
	return result, rows.Err()
}

func (r *CollectionRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	if n == 0 {
		return library.ErrNotFound
	}
	return nil
}

func (r *CollectionRepository) LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO collection_items (collection_id, media_item_id, sort_order)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		collectionID, mediaItemID, sortOrder,
	)
	return err
}

func (r *CollectionRepository) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	query := `SELECT m.id, m.library_id, m.parent_id, m.type, m.title, m.sort_title, m.original_title,
		 m.overview, m.tagline, m.file_path, m.file_size, m.file_fingerprint, m.duration_ms, m.bitrate,
		 m.container_format, m.index_number, m.community_rating, m.official_rating,
		 m.premiere_date, m.year, m.provider_ids, m.genres, m.studios, m.tags, m.added_at, m.updated_at
		 FROM media_items m JOIN collection_items ci ON m.id = ci.media_item_id
		 WHERE ci.collection_id = $1 ORDER BY m.premiere_date ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	return r.queryMediaItems(ctx, query, collectionID)
}

func (r *CollectionRepository) CountItems(ctx context.Context, collectionID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collection_items WHERE collection_id = $1`, collectionID,
	).Scan(&count)
	return count, err
}

var validSortCols = map[string]string{
	"title": "title", "sort_title": "sort_title", "added_at": "added_at",
	"year": "year", "premiere_date": "premiere_date", "community_rating": "community_rating",
}

var validSortDirs = map[string]string{
	"asc": "ASC", "desc": "DESC",
}

// SafeSortCol returns a safe column name from the allowlist (default: sort_title).
func SafeSortCol(col string) string {
	if v, ok := validSortCols[strings.ToLower(col)]; ok {
		return v
	}
	return "sort_title"
}

// SafeSortDir returns "ASC" or "DESC" (default: ASC).
func SafeSortDir(dir string) string {
	if v, ok := validSortDirs[strings.ToLower(dir)]; ok {
		return v
	}
	return "ASC"
}

func (r *CollectionRepository) queryMediaItems(ctx context.Context, query string, args ...any) ([]*library.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaItem
	for rows.Next() {
		item := &library.MediaItem{}
		var parentID, filePath, fingerprint *string
		var providerJSON, genresJSON, studiosJSON, tagsJSON []byte
		if err := rows.Scan(
			&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &item.SortTitle, &item.OriginalTitle,
			&item.Overview, &item.Tagline, &filePath, &item.FileSize, &fingerprint, &item.DurationMs, &item.Bitrate,
			&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
			&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
			&item.AddedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if parentID != nil {
			item.ParentID = *parentID
		}
		if filePath != nil {
			item.FilePath = *filePath
		}
		if fingerprint != nil {
			item.FileFingerprint = *fingerprint
		}
		if err := json.Unmarshal(providerJSON, &item.ProviderIDs); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "provider_ids"), slog.Any("err", err))
		}
		if err := json.Unmarshal(genresJSON, &item.Genres); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "genres"), slog.Any("err", err))
		}
		if err := json.Unmarshal(studiosJSON, &item.Studios); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "studios"), slog.Any("err", err))
		}
		if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "tags"), slog.Any("err", err))
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
