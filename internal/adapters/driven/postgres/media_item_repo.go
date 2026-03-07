package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
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
	genresJSON, err := json.Marshal(item.Genres)
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

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	var filePath *string
	if item.FilePath != "" {
		filePath = &item.FilePath
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, duration_ms, bitrate, container_format, index_number, community_rating, official_rating,
		 premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		item.ID, item.LibraryID, parentID, item.Type, item.Title, item.SortTitle, item.OriginalTitle,
		item.Overview, item.Tagline, filePath, item.FileSize, item.DurationMs, item.Bitrate,
		item.ContainerFormat, item.IndexNumber, item.CommunityRating, item.OfficialRating,
		item.PremiereDate, item.Year, providerJSON, genresJSON, studiosJSON, tagsJSON,
		item.AddedAt, item.UpdatedAt,
	)
	return err
}

func (r *MediaItemRepository) GetByID(ctx context.Context, id string) (*library.MediaItem, error) {
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at
		 FROM media_items WHERE id = $1`, id))
}

func (r *MediaItemRepository) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		`SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at
		 FROM media_items WHERE file_path = $1`, path))
}

func (r *MediaItemRepository) List(ctx context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if f.LibraryID != nil {
		conditions = append(conditions, fmt.Sprintf("library_id = $%d", argIdx))
		args = append(args, *f.LibraryID)
		argIdx++
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
	for _, g := range f.Genres {
		conditions = append(conditions, fmt.Sprintf("genres @> $%d::jsonb", argIdx))
		gJSON, _ := json.Marshal([]string{g})
		args = append(args, string(gJSON))
		argIdx++
	}

	query := `SELECT id, library_id, parent_id, type, title, sort_title, original_title, overview, tagline,
		 file_path, file_size, duration_ms, bitrate, container_format, index_number, community_rating,
		 official_rating, premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at
		 FROM media_items`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	sortField := "sort_title"
	if f.SortField != "" {
		allowed := map[string]bool{"title": true, "sort_title": true, "added_at": true, "year": true, "premiere_date": true, "community_rating": true}
		if allowed[f.SortField] {
			sortField = f.SortField
		}
	}
	sortOrder := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortOrder = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortField, sortOrder)

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
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
	genresJSON, err := json.Marshal(item.Genres)
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

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	var filePath *string
	if item.FilePath != "" {
		filePath = &item.FilePath
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE media_items SET library_id=$1, parent_id=$2, type=$3, title=$4, sort_title=$5, original_title=$6,
		 overview=$7, tagline=$8, file_path=$9, file_size=$10, duration_ms=$11, bitrate=$12, container_format=$13,
		 index_number=$14, community_rating=$15, official_rating=$16, premiere_date=$17, year=$18,
		 provider_ids=$19, genres=$20, studios=$21, tags=$22, updated_at=$23 WHERE id=$24`,
		item.LibraryID, parentID, item.Type, item.Title, item.SortTitle, item.OriginalTitle,
		item.Overview, item.Tagline, filePath, item.FileSize, item.DurationMs, item.Bitrate,
		item.ContainerFormat, item.IndexNumber, item.CommunityRating, item.OfficialRating,
		item.PremiereDate, item.Year, providerJSON, genresJSON, studiosJSON, tagsJSON,
		item.UpdatedAt, item.ID,
	)
	return err
}

func (r *MediaItemRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = $1`, id)
	return err
}

func (r *MediaItemRepository) scanMediaItem(row *sql.Row) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	var parentID, filePath *string
	var providerJSON, genresJSON, studiosJSON, tagsJSON []byte

	err := row.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &item.SortTitle, &item.OriginalTitle,
		&item.Overview, &item.Tagline, &filePath, &item.FileSize, &item.DurationMs, &item.Bitrate,
		&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
		&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
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
	_ = json.Unmarshal(providerJSON, &item.ProviderIDs)
	_ = json.Unmarshal(genresJSON, &item.Genres)
	_ = json.Unmarshal(studiosJSON, &item.Studios)
	_ = json.Unmarshal(tagsJSON, &item.Tags)
	return item, nil
}

func (r *MediaItemRepository) scanMediaItemFromRows(rows *sql.Rows) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	var parentID, filePath *string
	var providerJSON, genresJSON, studiosJSON, tagsJSON []byte

	err := rows.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &item.SortTitle, &item.OriginalTitle,
		&item.Overview, &item.Tagline, &filePath, &item.FileSize, &item.DurationMs, &item.Bitrate,
		&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
		&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
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
	_ = json.Unmarshal(providerJSON, &item.ProviderIDs)
	_ = json.Unmarshal(genresJSON, &item.Genres)
	_ = json.Unmarshal(studiosJSON, &item.Studios)
	_ = json.Unmarshal(tagsJSON, &item.Tags)
	return item, nil
}
