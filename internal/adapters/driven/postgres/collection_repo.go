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
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CollectionRepository struct {
	db *sql.DB
}

func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

func (r *CollectionRepository) Create(ctx context.Context, col *library.Collection) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(col.Rules)
	if err != nil {
		rulesJSON = []byte("[]")
	}
	sortBy := col.SortBy
	if sortBy == "" {
		sortBy = "added_at"
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		col.ID, col.Name, col.Overview, col.TmdbID, col.PosterURL, col.BackdropURL,
		rulesJSON, sortBy, col.CreatedAt, col.UpdatedAt,
	)
	return err
}

func (r *CollectionRepository) Upsert(ctx context.Context, col *library.Collection) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(col.Rules)
	if err != nil {
		rulesJSON = []byte("[]")
	}
	sortBy := col.SortBy
	if sortBy == "" {
		sortBy = "added_at"
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tmdb_id) WHERE tmdb_id IS NOT NULL DO UPDATE SET
		   name = EXCLUDED.name,
		   overview = EXCLUDED.overview,
		   poster_url = EXCLUDED.poster_url,
		   backdrop_url = EXCLUDED.backdrop_url,
		   rules = EXCLUDED.rules,
		   sort_by = EXCLUDED.sort_by,
		   updated_at = EXCLUDED.updated_at`,
		col.ID, col.Name, col.Overview, col.TmdbID, col.PosterURL, col.BackdropURL,
		rulesJSON, sortBy, col.CreatedAt, col.UpdatedAt,
	)
	return err
}

func (r *CollectionRepository) Update(ctx context.Context, col *library.Collection) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rulesJSON, err := json.Marshal(col.Rules)
	if err != nil {
		rulesJSON = []byte("[]")
	}
	sortBy := col.SortBy
	if sortBy == "" {
		sortBy = "added_at"
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE collections SET name = $1, overview = $2, rules = $3, sort_by = $4, updated_at = $5
		 WHERE id = $6`,
		col.Name, col.Overview, rulesJSON, sortBy, col.UpdatedAt, col.ID,
	)
	if err != nil {
		return err
	}
	n, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		slog.Warn("rows affected", slog.Any("err", rowsErr))
	}
	if n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	col := &library.Collection{}
	var rulesJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at
		 FROM collections WHERE id = $1`, id,
	).Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
		&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return col, nil
}

func (r *CollectionRepository) GetByTmdbID(ctx context.Context, tmdbID int) (*library.Collection, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	col := &library.Collection{}
	var rulesJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at
		 FROM collections WHERE tmdb_id = $1`, tmdbID,
	).Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
		&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return col, nil
}

func (r *CollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at
		 FROM collections ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Collection
	for rows.Next() {
		col := &library.Collection{}
		var rulesJSON []byte
		if err := rows.Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
			&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
		}
		result = append(result, col)
	}
	return result, rows.Err()
}

func (r *CollectionRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	if n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *CollectionRepository) LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO collection_items (collection_id, media_item_id, sort_order)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		collectionID, mediaItemID, sortOrder,
	)
	return err
}

func (r *CollectionRepository) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collection_items WHERE collection_id = $1`, collectionID,
	).Scan(&count)
	return count, err
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

// CollectionSmartQuerier evaluates collection rules directly against media_items.
type CollectionSmartQuerier struct {
	db *sql.DB
}

func NewCollectionSmartQuerier(db *sql.DB) *CollectionSmartQuerier {
	return &CollectionSmartQuerier{db: db}
}

func (q *CollectionSmartQuerier) QueryByRules(ctx context.Context, rules []library.CollectionRule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	query := `SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		file_path, file_size, file_fingerprint, duration_ms, bitrate, container_format, index_number, community_rating,
		official_rating, premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at
		FROM media_items`

	var conditions []string
	var args []any
	argIdx := 1

	for _, rule := range rules {
		cond, ruleArgs := buildCollectionRuleCondition(rule, &argIdx)
		if cond != "" {
			conditions = append(conditions, cond)
			args = append(args, ruleArgs...)
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	validSort := map[string]string{
		"title":    "sort_title",
		"year":     "year",
		"added_at": "added_at",
		"rating":   "community_rating",
	}
	col, ok := validSort[sortBy]
	if !ok {
		col = "added_at"
	}
	dir := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		dir = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s NULLS LAST", col, dir)

	if limit <= 0 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return q.scanItems(rows)
}

func buildCollectionRuleCondition(rule library.CollectionRule, argIdx *int) (string, []any) {
	switch rule.Field {
	case "genre":
		val, err := json.Marshal([]string{rule.Value})
		if err != nil {
			slog.Warn("marshal genre rule", slog.Any("err", err))
			return "", nil
		}
		cond := fmt.Sprintf("genres @> $%d::jsonb", *argIdx)
		*argIdx++
		return cond, []any{string(val)}
	case "type":
		cond := fmt.Sprintf("type = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "year":
		switch rule.Operator {
		case "gt":
			cond := fmt.Sprintf("year > $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case "lt":
			cond := fmt.Sprintf("year < $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case "gte":
			cond := fmt.Sprintf("year >= $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case "lte":
			cond := fmt.Sprintf("year <= $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		default:
			cond := fmt.Sprintf("year = $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		}
	case "library_id":
		cond := fmt.Sprintf("library_id = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "rating_gte":
		cond := fmt.Sprintf("community_rating >= $%d::double precision", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	default:
		return "", nil
	}
}

func (q *CollectionSmartQuerier) scanItems(rows *sql.Rows) ([]*library.MediaItem, error) {
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

var _ library.CollectionCommandRepo = (*CollectionRepository)(nil)
var _ library.CollectionQueryRepo = (*CollectionRepository)(nil)
