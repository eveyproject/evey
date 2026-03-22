package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type SQLSearchFallback struct {
	db *sql.DB
}

func NewSQLSearchFallback(db *sql.DB) *SQLSearchFallback {
	return &SQLSearchFallback{db: db}
}

func (s *SQLSearchFallback) IndexMediaItem(_ context.Context, _ *library.MediaItem, _ []string) error {
	return nil
}

func (s *SQLSearchFallback) IndexMediaItemsBatch(_ context.Context, _ []*library.MediaItem) error {
	return nil
}

func (s *SQLSearchFallback) RemoveMediaItem(_ context.Context, _ string) error {
	return nil
}

func (s *SQLSearchFallback) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, 0, nil
	}

	pattern := "%" + q + "%"
	args := []any{pattern, pattern}
	where := "(LOWER(title) LIKE $1 OR LOWER(COALESCE(original_title,'')) LIKE $2)"
	argN := 3

	if typ, ok := filters["type"]; ok && typ != "" {
		where += fmt.Sprintf(" AND type = $%d", argN)
		args = append(args, typ)
		argN++
	}
	if libID, ok := filters["library_id"]; ok && libID != "" {
		where += fmt.Sprintf(" AND library_id = $%d", argN)
		args = append(args, libID)
	}

	if limit <= 0 {
		limit = 20
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM media_items WHERE " + where
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("search count: %w", err)
	}

	orderClause := "title"
	if len(sort) > 0 {
		orderClause = buildSortClause(sort)
	}

	dataSQL := fmt.Sprintf(
		"SELECT id, title, year, type, genres, official_rating FROM media_items WHERE %s ORDER BY %s LIMIT %d",
		where, orderClause, limit,
	)
	rows, err := s.db.QueryContext(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var hits []library.SearchHit
	for rows.Next() {
		var h library.SearchHit
		var genresJSON []byte
		var year sql.NullInt32
		if err := rows.Scan(&h.ID, &h.Title, &year, &h.Type, &genresJSON, &h.OfficialRating); err != nil {
			return nil, 0, fmt.Errorf("search scan: %w", err)
		}
		if year.Valid {
			y := int(year.Int32)
			h.Year = &y
		}
		h.Genres = parseJSONStringArray(genresJSON)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search rows: %w", err)
	}

	return hits, total, nil
}

var sortColumnMap = map[string]string{
	"title": "sort_title", "sort_title": "sort_title",
	"added_at": "added_at", "date_added": "added_at",
	"year": "year", "premiere_date": "premiere_date",
	"community_rating": "community_rating",
}

func buildSortClause(sort []string) string {
	parts := make([]string, 0, len(sort))
	for _, s := range sort {
		field := s
		dir := "ASC"
		if f, ok := strings.CutSuffix(s, ":desc"); ok {
			field = f
			dir = "DESC"
		} else if f, ok := strings.CutSuffix(s, ":asc"); ok {
			field = f
		}
		col, ok := sortColumnMap[field]
		if !ok {
			continue
		}
		parts = append(parts, col+" "+dir)
	}
	if len(parts) == 0 {
		return "title"
	}
	return strings.Join(parts, ", ")
}

func parseJSONStringArray(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
