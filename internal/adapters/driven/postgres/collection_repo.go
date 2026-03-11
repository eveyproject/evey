package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type CollectionRepository struct {
	db *sql.DB
}

func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

func (r *CollectionRepository) Create(ctx context.Context, col *library.Collection) error {
	rulesJSON, err := json.Marshal(col.Rules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, type, rules, sort_by, sort_order, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		col.ID, col.Name, col.Overview, string(col.Type), rulesJSON,
		col.SortBy, col.SortOrder, col.CreatedAt, col.UpdatedAt,
	)
	return err
}

func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	col := &library.Collection{}
	var rulesJSON []byte
	var colType string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, overview, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM collections WHERE id = $1`, id,
	).Scan(&col.ID, &col.Name, &col.Overview, &colType, &rulesJSON,
		&col.SortBy, &col.SortOrder, &col.CreatedAt, &col.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, library.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	col.Type = library.CollectionType(colType)
	_ = json.Unmarshal(rulesJSON, &col.Rules)
	return col, nil
}

func (r *CollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, overview, type, rules, sort_by, sort_order, created_at, updated_at
		 FROM collections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Collection
	for rows.Next() {
		col := &library.Collection{}
		var rulesJSON []byte
		var colType string
		if err := rows.Scan(&col.ID, &col.Name, &col.Overview, &colType, &rulesJSON,
			&col.SortBy, &col.SortOrder, &col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, err
		}
		col.Type = library.CollectionType(colType)
		_ = json.Unmarshal(rulesJSON, &col.Rules)
		result = append(result, col)
	}
	return result, rows.Err()
}

func (r *CollectionRepository) Update(ctx context.Context, col *library.Collection) error {
	rulesJSON, err := json.Marshal(col.Rules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE collections SET name=$1, overview=$2, type=$3, rules=$4, sort_by=$5, sort_order=$6, updated_at=$7
		 WHERE id=$8`,
		col.Name, col.Overview, string(col.Type), rulesJSON,
		col.SortBy, col.SortOrder, col.UpdatedAt, col.ID,
	)
	return err
}

func (r *CollectionRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return library.ErrNotFound
	}
	return nil
}

func (r *CollectionRepository) AddItem(ctx context.Context, collectionID, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO collection_items (collection_id, media_item_id, sort_order)
		 VALUES ($1, $2, COALESCE((SELECT MAX(sort_order)+1 FROM collection_items WHERE collection_id=$1), 0))
		 ON CONFLICT DO NOTHING`,
		collectionID, mediaItemID,
	)
	return err
}

func (r *CollectionRepository) RemoveItem(ctx context.Context, collectionID, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM collection_items WHERE collection_id = $1 AND media_item_id = $2`,
		collectionID, mediaItemID,
	)
	return err
}

func (r *CollectionRepository) GetManualItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	query := `SELECT m.id, m.library_id, m.parent_id, m.type, m.title, m.sort_title, m.original_title,
		 m.overview, m.tagline, m.file_path, m.file_size, m.file_fingerprint, m.duration_ms, m.bitrate,
		 m.container_format, m.index_number, m.community_rating, m.official_rating,
		 m.premiere_date, m.year, m.provider_ids, m.genres, m.studios, m.tags, m.added_at, m.updated_at
		 FROM media_items m JOIN collection_items ci ON m.id = ci.media_item_id
		 WHERE ci.collection_id = $1 ORDER BY ci.sort_order`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	return r.queryMediaItems(ctx, query, collectionID)
}

func (r *CollectionRepository) CountManualItems(ctx context.Context, collectionID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collection_items WHERE collection_id = $1`, collectionID,
	).Scan(&count)
	return count, err
}

var allowedSortFields = map[string]bool{
	"title": true, "sort_title": true, "added_at": true,
	"year": true, "premiere_date": true, "community_rating": true,
}

// ValidateSortParams returns ErrInvalidSortParam if sortBy or sortOrder are not in the allowlist.
func ValidateSortParams(sortBy, sortOrder string) error {
	if sortBy != "" && !allowedSortFields[sortBy] {
		return fmt.Errorf("%w: sortBy %q", library.ErrInvalidSortParam, sortBy)
	}
	upper := strings.ToUpper(sortOrder)
	if upper != "ASC" && upper != "DESC" {
		return fmt.Errorf("%w: sortOrder %q", library.ErrInvalidSortParam, sortOrder)
	}
	return nil
}

func (r *CollectionRepository) GetSmartItems(ctx context.Context, col *library.Collection, limit, offset int) ([]*library.MediaItem, error) {
	if err := ValidateSortParams(col.SortBy, col.SortOrder); err != nil {
		return nil, err
	}
	where, args := buildRulesWhere(col.Rules)
	sortField := "sort_title"
	if col.SortBy != "" {
		sortField = col.SortBy
	}
	sortOrder := "ASC"
	if strings.EqualFold(col.SortOrder, "desc") {
		sortOrder = "DESC"
	}
	query := fmt.Sprintf(`SELECT id, library_id, parent_id, type, title, sort_title, original_title,
		 overview, tagline, file_path, file_size, file_fingerprint, duration_ms, bitrate,
		 container_format, index_number, community_rating, official_rating,
		 premiere_date, year, provider_ids, genres, studios, tags, added_at, updated_at
		 FROM media_items%s ORDER BY %s %s`, where, sortField, sortOrder)
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	return r.queryMediaItems(ctx, query, args...)
}

func (r *CollectionRepository) CountSmartItems(ctx context.Context, col *library.Collection) (int, error) {
	where, args := buildRulesWhere(col.Rules)
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM media_items"+where, args...,
	).Scan(&count)
	return count, err
}

func buildRulesWhere(rules []library.CollectionRule) (string, []any) {
	var conditions []string
	var args []any
	argIdx := 1

	for _, rule := range rules {
		cond, ruleArgs := ruleToSQL(rule, &argIdx)
		if cond != "" {
			conditions = append(conditions, cond)
			args = append(args, ruleArgs...)
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func ruleToSQL(rule library.CollectionRule, argIdx *int) (string, []any) {
	switch rule.Field {
	case "genre":
		switch rule.Operator {
		case library.OpContains:
			g, _ := json.Marshal([]string{rule.Value})
			cond := fmt.Sprintf("genres @> $%d::jsonb", *argIdx)
			*argIdx++
			return cond, []any{string(g)}
		case library.OpEq:
			g, _ := json.Marshal([]string{rule.Value})
			cond := fmt.Sprintf("genres @> $%d::jsonb", *argIdx)
			*argIdx++
			return cond, []any{string(g)}
		}
	case "year":
		v, err := strconv.Atoi(rule.Value)
		if err != nil {
			return "", nil
		}
		var op string
		switch rule.Operator {
		case library.OpEq:
			op = "="
		case library.OpGt:
			op = ">"
		case library.OpLt:
			op = "<"
		case library.OpNeq:
			op = "!="
		default:
			return "", nil
		}
		cond := fmt.Sprintf("year %s $%d", op, *argIdx)
		*argIdx++
		return cond, []any{v}
	case "type":
		switch rule.Operator {
		case library.OpEq:
			cond := fmt.Sprintf("type = $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case library.OpNeq:
			cond := fmt.Sprintf("type != $%d", *argIdx)
			*argIdx++
			return cond, []any{rule.Value}
		case library.OpIn:
			vals := strings.Split(rule.Value, ",")
			placeholders := make([]string, len(vals))
			var ruleArgs []any
			for i, v := range vals {
				placeholders[i] = fmt.Sprintf("$%d", *argIdx)
				ruleArgs = append(ruleArgs, strings.TrimSpace(v))
				*argIdx++
			}
			return fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")), ruleArgs
		}
	case "rating":
		v, err := strconv.ParseFloat(rule.Value, 64)
		if err != nil {
			return "", nil
		}
		var op string
		switch rule.Operator {
		case library.OpGt:
			op = ">"
		case library.OpLt:
			op = "<"
		case library.OpEq:
			op = "="
		default:
			return "", nil
		}
		cond := fmt.Sprintf("community_rating %s $%d", op, *argIdx)
		*argIdx++
		return cond, []any{v}
	case "resolution":
		switch rule.Operator {
		case library.OpEq:
			h := resolutionToHeight(rule.Value)
			if h == 0 {
				return "", nil
			}
			cond := fmt.Sprintf(`id IN (SELECT media_item_id FROM media_streams WHERE stream_type='video' AND height >= $%d)`, *argIdx)
			*argIdx++
			return cond, []any{h}
		}
	case "added_date":
		days, err := strconv.Atoi(rule.Value)
		if err != nil {
			return "", nil
		}
		switch rule.Operator {
		case library.OpLt:
			cond := fmt.Sprintf("added_at >= $%d", *argIdx)
			*argIdx++
			return cond, []any{time.Now().AddDate(0, 0, -days)}
		}
	}
	return "", nil
}

func resolutionToHeight(res string) int {
	switch strings.ToLower(res) {
	case "4k", "2160p":
		return 2160
	case "1080p":
		return 1080
	case "720p":
		return 720
	case "480p":
		return 480
	}
	return 0
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
		_ = json.Unmarshal(providerJSON, &item.ProviderIDs)
		_ = json.Unmarshal(genresJSON, &item.Genres)
		_ = json.Unmarshal(studiosJSON, &item.Studios)
		_ = json.Unmarshal(tagsJSON, &item.Tags)
		result = append(result, item)
	}
	return result, rows.Err()
}
