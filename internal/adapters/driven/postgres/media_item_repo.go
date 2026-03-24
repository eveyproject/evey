package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(db *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: db}
}

func (r *MediaItemRepository) Create(ctx context.Context, item *library.MediaItem) error {
	providerJSON, err := json.Marshal(item.ProviderIDs)
	if err != nil {
		return err
	}
	genres := item.Genres
	if genres == nil {
		genres = []string{}
	}
	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return err
	}
	studiosJSON, err := json.Marshal(item.Studios)
	if err != nil {
		return err
	}
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return err
	}
	trailers := item.Trailers
	if trailers == nil {
		trailers = []library.Trailer{}
	}
	trailersJSON, err := json.Marshal(trailers)
	if err != nil {
		return err
	}
	ratings := item.Ratings
	if ratings == nil {
		ratings = map[string]float64{}
	}
	ratingsJSON, err := json.Marshal(ratings)
	if err != nil {
		return err
	}

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	var filePath *string
	if item.FilePath != "" {
		filePath = &item.FilePath
	}
	var fingerprint *string
	if item.FileFingerprint != "" {
		fingerprint = &item.FileFingerprint
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating, official_rating,
		 premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)`,
		item.ID, item.LibraryID, parentID, item.Type, item.Title, item.SortTitle, item.OriginalTitle,
		item.Overview, item.Tagline, filePath, item.FileSize, fingerprint, item.ModTime, item.DurationMs, item.Bitrate,
		item.ContainerFormat, item.IndexNumber, item.CommunityRating, item.OfficialRating,
		item.PremiereDate, item.Year, providerJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON,
		item.AddedAt, item.UpdatedAt,
	)
	return err
}

func (r *MediaItemRepository) GetByID(ctx context.Context, id string) (*library.MediaItem, error) {
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at
		 FROM media_items WHERE id = $1`, id))
}

func (r *MediaItemRepository) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at
		 FROM media_items WHERE file_path = $1`, path))
}

func (r *MediaItemRepository) GetByFileFingerprint(ctx context.Context, fingerprint string) (*library.MediaItem, error) {
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at
		 FROM media_items WHERE file_fingerprint = $1`, fingerprint))
}

func (r *MediaItemRepository) GetByLibraryTypeTitle(ctx context.Context, libraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at
		 FROM media_items WHERE library_id = $1 AND type = $2 AND title = $3 AND ($4 = '' OR parent_id = $4) LIMIT 1`, libraryID, itemType, title, parentID))
}

func (r *MediaItemRepository) buildWhereClause(f library.ListMediaItemsFilter) (string, []any) {
	var conditions []string
	var args []any
	argIdx := 1
	if f.LibraryID != nil {
		conditions = append(conditions, fmt.Sprintf("library_id = $%d", argIdx))
		args = append(args, *f.LibraryID)
		argIdx++
	}
	if len(f.LibraryIDs) > 0 {
		placeholders := make([]string, len(f.LibraryIDs))
		for i, id := range f.LibraryIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, id)
			argIdx++
		}
		conditions = append(conditions, "library_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if f.Type != nil {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *f.Type)
		argIdx++
	}
	if f.ParentID != nil {
		conditions = append(conditions, fmt.Sprintf("parent_id = $%d", argIdx))
		args = append(args, *f.ParentID)
		argIdx++
	}
	if f.Year != nil {
		conditions = append(conditions, fmt.Sprintf("year = $%d", argIdx))
		args = append(args, *f.Year)
		argIdx++
	}
	if f.YearFrom != 0 {
		conditions = append(conditions, fmt.Sprintf("year >= $%d", argIdx))
		args = append(args, f.YearFrom)
		argIdx++
	}
	if f.YearTo != 0 {
		conditions = append(conditions, fmt.Sprintf("year <= $%d", argIdx))
		args = append(args, f.YearTo)
		argIdx++
	}
	if f.MinRating != 0 {
		conditions = append(conditions, fmt.Sprintf("community_rating >= $%d", argIdx))
		args = append(args, f.MinRating)
		argIdx++
	}
	for _, g := range f.Genres {
		conditions = append(conditions, fmt.Sprintf("genres @> $%d::jsonb", argIdx))
		gJSON, err := json.Marshal([]string{g})
		if err != nil {
			slog.Warn("marshal genre filter", slog.Any("err", err))
			continue
		}
		args = append(args, string(gJSON))
		argIdx++
	}
	if len(f.AllowedRatings) > 0 {
		placeholders := make([]string, len(f.AllowedRatings))
		for i, r := range f.AllowedRatings {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, r)
			argIdx++
		}
		conditions = append(conditions, "(official_rating = '' OR official_rating IS NULL OR official_rating = ANY(ARRAY["+strings.Join(placeholders, ",")+"]))")
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

func (r *MediaItemRepository) Count(ctx context.Context, f library.ListMediaItemsFilter) (int, error) {
	where, args := r.buildWhereClause(f)
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items"+where, args...).Scan(&count)
	return count, err
}

func (r *MediaItemRepository) List(ctx context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	where, args := r.buildWhereClause(f)
	query := `SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at
		 FROM media_items` + where

	sortField := "sort_title"
	if f.SortField != "" {
		if col, ok := SortColumnMap[f.SortField]; ok {
			sortField = col
		}
	}
	sortOrder := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortOrder = "DESC"
	}
	if sortField == "added_at" {
		// Rewrite: use LEFT JOIN instead of correlated subquery for series added_at
		query = `SELECT media_items.id, media_items.library_id, media_items.parent_id, media_items.type,
		 media_items.title, media_items.sort_title, media_items.original_title, media_items.overview, media_items.tagline,
		 media_items.file_path, media_items.file_size, media_items.file_fingerprint, media_items.mod_time,
		 media_items.duration_ms, media_items.bitrate, media_items.container_format, media_items.index_number,
		 media_items.community_rating, media_items.official_rating, media_items.premiere_date, media_items.year,
		 media_items.provider_ids, media_items.genres, media_items.studios, media_items.tags, media_items.trailers,
		 media_items.ratings, media_items.added_at, media_items.updated_at
		 FROM media_items
		 LEFT JOIN (
			SELECT s.parent_id AS series_id, MAX(e.added_at) AS max_added
			FROM media_items e
			INNER JOIN media_items s ON s.id = e.parent_id
			WHERE e.type = 'episode'
			GROUP BY s.parent_id
		 ) ep_latest ON media_items.type = 'series' AND ep_latest.series_id = media_items.id` + where
		query += fmt.Sprintf(` ORDER BY CASE WHEN media_items.type = 'series'
		 THEN COALESCE(ep_latest.max_added, media_items.added_at)
		 ELSE media_items.added_at END %s`, sortOrder)
	} else {
		query += fmt.Sprintf(" ORDER BY %s %s", sortField, sortOrder)
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *MediaItemRepository) Update(ctx context.Context, item *library.MediaItem) error {
	providerJSON, err := json.Marshal(item.ProviderIDs)
	if err != nil {
		return err
	}
	genres := item.Genres
	if genres == nil {
		genres = []string{}
	}
	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return err
	}
	studiosJSON, err := json.Marshal(item.Studios)
	if err != nil {
		return err
	}
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return err
	}
	trailers := item.Trailers
	if trailers == nil {
		trailers = []library.Trailer{}
	}
	trailersJSON, err := json.Marshal(trailers)
	if err != nil {
		return err
	}
	ratings := item.Ratings
	if ratings == nil {
		ratings = map[string]float64{}
	}
	ratingsJSON, err := json.Marshal(ratings)
	if err != nil {
		return err
	}

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	var filePath *string
	if item.FilePath != "" {
		filePath = &item.FilePath
	}
	var fingerprint *string
	if item.FileFingerprint != "" {
		fingerprint = &item.FileFingerprint
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE media_items SET library_id=$1, parent_id=$2, type=$3, title=$4, sort_title=$5, original_title=$6,
		 overview=$7, tagline=$8, file_path=$9, file_size=$10, file_fingerprint=$11, mod_time=$12, duration_ms=$13, bitrate=$14, container_format=$15,
		 index_number=$16, community_rating=$17, official_rating=$18, premiere_date=$19, year=$20,
		 provider_ids=$21, genres=$22, studios=$23, tags=$24, trailers=$25, ratings=$26, updated_at=$27 WHERE id=$28`,
		item.LibraryID, parentID, item.Type, item.Title, item.SortTitle, item.OriginalTitle,
		item.Overview, item.Tagline, filePath, item.FileSize, fingerprint, item.ModTime, item.DurationMs, item.Bitrate,
		item.ContainerFormat, item.IndexNumber, item.CommunityRating, item.OfficialRating,
		item.PremiereDate, item.Year, providerJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON,
		item.UpdatedAt, item.ID,
	)
	return err
}

func (r *MediaItemRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = $1`, id)
	return err
}

func (r *MediaItemRepository) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = ANY($1)`, pq.Array(ids))
	return err
}

func (r *MediaItemRepository) ListFilePaths(ctx context.Context, libraryID string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, file_path FROM media_items WHERE library_id = $1 AND file_path IS NOT NULL AND file_path <> ''`,
		libraryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		result[id] = path
	}
	return result, rows.Err()
}

const listByLibraryIDMaxRows = 10000

func (r *MediaItemRepository) ListByLibraryID(ctx context.Context, libraryID string) ([]*library.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at
		 FROM media_items WHERE library_id = $1 LIMIT $2`, libraryID, listByLibraryIDMaxRows)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *MediaItemRepository) ListScanSummaries(ctx context.Context, libraryID string) ([]library.MediaItemScanRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, file_path, file_fingerprint, file_size, mod_time
		 FROM media_items WHERE library_id = $1`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []library.MediaItemScanRow
	for rows.Next() {
		var row library.MediaItemScanRow
		var filePath, fingerprint *string
		if err := rows.Scan(&row.ID, &filePath, &fingerprint, &row.FileSize, &row.ModTime); err != nil {
			return nil, err
		}
		if filePath != nil {
			row.FilePath = *filePath
		}
		if fingerprint != nil {
			row.FileFingerprint = *fingerprint
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MediaItemRepository) ListContainerInfo(ctx context.Context, libraryID string) ([]library.MediaItemContainerRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, parent_id, type FROM media_items WHERE library_id = $1`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []library.MediaItemContainerRow
	for rows.Next() {
		var row library.MediaItemContainerRow
		var parentID *string
		if err := rows.Scan(&row.ID, &parentID, &row.Type); err != nil {
			return nil, err
		}
		if parentID != nil {
			row.ParentID = *parentID
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MediaItemRepository) scanMediaItem(row *sql.Row) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	var parentID, filePath, fingerprint *string
	var providerJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON []byte

	err := row.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &item.SortTitle, &item.OriginalTitle,
		&item.Overview, &item.Tagline, &filePath, &item.FileSize, &fingerprint, &item.ModTime, &item.DurationMs, &item.Bitrate,
		&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
		&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON, &trailersJSON, &ratingsJSON,
		&item.AddedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
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
	if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
	}
	if err := json.Unmarshal(ratingsJSON, &item.Ratings); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "ratings"), slog.Any("err", err))
	}
	return item, nil
}

func (r *MediaItemRepository) scanMediaItemFromRows(rows *sql.Rows) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	var parentID, filePath, fingerprint *string
	var providerJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON []byte

	err := rows.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &item.SortTitle, &item.OriginalTitle,
		&item.Overview, &item.Tagline, &filePath, &item.FileSize, &fingerprint, &item.ModTime, &item.DurationMs, &item.Bitrate,
		&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
		&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON, &trailersJSON, &ratingsJSON,
		&item.AddedAt, &item.UpdatedAt,
	)
	if err != nil {
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
	if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
	}
	if err := json.Unmarshal(ratingsJSON, &item.Ratings); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "ratings"), slog.Any("err", err))
	}
	return item, nil
}

var _ library.MediaItemRepository = (*MediaItemRepository)(nil)
