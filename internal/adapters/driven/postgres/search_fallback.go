package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
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
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, 0, nil
	}

	pattern := "%" + likeEscape(q) + "%"
	args := []any{pattern, pattern}
	where := "(mi.title ILIKE $1 ESCAPE '\\' OR COALESCE(mi.original_title,'') ILIKE $2 ESCAPE '\\')"
	argN := 3

	if typ, ok := filters["type"]; ok && typ != "" {
		where += fmt.Sprintf(" AND mi.type = $%d", argN)
		args = append(args, typ)
		argN++
	}
	if libID, ok := filters["library_id"]; ok && libID != "" {
		where += fmt.Sprintf(" AND mi.library_id = $%d", argN)
		args = append(args, libID)
		argN++
	}
	// Genre and year filters: Meilisearch applies these (search_adapter filterable
	// attributes), so the SQL fallback must too or a genre/year-filtered search
	// silently returns unfiltered results while Meili is down. media_catalog (mc)
	// is already joined below.
	if g, ok := filters["genres"]; ok && g != "" {
		gj, err := json.Marshal([]string{g})
		if err == nil {
			where += fmt.Sprintf(" AND mc.genres @> $%d::jsonb", argN)
			args = append(args, string(gj))
			argN++
		}
	}
	if ymin, ok := filters["year_min"]; ok && ymin != "" {
		if n, err := strconv.Atoi(ymin); err == nil {
			where += fmt.Sprintf(" AND mc.year >= $%d", argN)
			args = append(args, n)
			argN++
		}
	}
	if ymax, ok := filters["year_max"]; ok && ymax != "" {
		if n, err := strconv.Atoi(ymax); err == nil {
			where += fmt.Sprintf(" AND mc.year <= $%d", argN)
			args = append(args, n)
			argN++
		}
	}

	if limit <= 0 {
		limit = 20
	}

	const fromJoins = `FROM media_items mi
	 LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
	 LEFT JOIN media_classifications mcl ON mcl.media_item_id = mi.id`

	var total int64
	countSQL := "SELECT COUNT(*) " + fromJoins + " WHERE " + where
	if err := txFromCtx(ctx, s.db).QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("search count: %w", err)
	}

	orderClause := "mi.title"
	if len(sort) > 0 {
		orderClause = buildSortClause(sort)
	}

	dataSQL := fmt.Sprintf(
		"SELECT mi.id, mi.library_id, mi.title, mc.year, mi.type, mc.genres, mcl.official_rating %s WHERE %s ORDER BY %s LIMIT %d",
		fromJoins, where, orderClause, limit,
	)
	rows, err := txFromCtx(ctx, s.db).QueryContext(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var hits []library.SearchHit
	for rows.Next() {
		var h library.SearchHit
		var genresJSON []byte
		var year sql.NullInt32
		var officialRating sql.NullString
		if err := rows.Scan(&h.ID, &h.LibraryID, &h.Title, &year, &h.Type, &genresJSON, &officialRating); err != nil {
			return nil, 0, fmt.Errorf("search scan: %w", err)
		}
		if year.Valid {
			y := int(year.Int32)
			h.Year = &y
		}
		if officialRating.Valid {
			h.OfficialRating = officialRating.String
		}
		h.Genres = parseJSONStringArray(genresJSON)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search rows: %w", err)
	}

	return hits, total, nil
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
		col, ok := SortColumnMap[field]
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

var _ library.SearchIndex = (*SQLSearchFallback)(nil)
