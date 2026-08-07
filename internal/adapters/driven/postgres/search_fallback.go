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

func (s *SQLSearchFallback) IndexMediaItem(_ context.Context, _ *library.MediaItem) error {
	return nil
}

func (s *SQLSearchFallback) IndexMediaItemsBatch(_ context.Context, _ []*library.MediaItem) error {
	return nil
}

func (s *SQLSearchFallback) RemoveMediaItem(_ context.Context, _ string) error {
	return nil
}

// Clear is a no-op: the SQL fallback queries Postgres directly (the DB IS the
// source of truth), so there is no separate index to wipe.
func (s *SQLSearchFallback) Clear(_ context.Context) error {
	return nil
}

// searchWhere builds the WHERE clause and its ordered args for the SQL
// fallback, and reports whether there is anything to search for at all.
//
// A blank query with FILTERS is a legitimate search - "everything in the
// Comedy genre" - and Meilisearch has always served it. This used to bail out
// on a blank query whatever the filters, so any filter-only browse came back
// empty while Meili was down. Only a request with neither text nor filter
// returns false: that one must not turn into a full-catalog dump.
func searchWhere(query string, filters map[string]string) (string, []any, bool) {
	q := strings.TrimSpace(query)
	var clauses []string
	var args []any
	argN := 1

	if q != "" {
		pattern := "%" + likeEscape(q) + "%"
		args = append(args, pattern, pattern)
		// Match title/original_title AND (like Meilisearch's searchable attributes)
		// translated titles and cast names, so a search for a foreign-language title or
		// an actor name still finds results while Meili is down. EXISTS subqueries keep
		// one row per media item (no JOIN fan-out, so no DISTINCT needed).
		clauses = append(clauses, fmt.Sprintf(`(mi.title ILIKE $%d ESCAPE '\'
		OR COALESCE(mi.original_title,'') ILIKE $%d ESCAPE '\'
		OR EXISTS (SELECT 1 FROM media_item_translations mt
			WHERE mt.media_item_id = mi.id AND mt.title ILIKE $%d ESCAPE '\')
		OR EXISTS (SELECT 1 FROM media_people mp JOIN people pe ON pe.id = mp.person_id
			WHERE mp.media_item_id = mi.id AND pe.name ILIKE $%d ESCAPE '\'))`, 1, 2, 1, 1))
		argN = 3
	}

	if typ, ok := filters["type"]; ok && typ != "" {
		clauses = append(clauses, fmt.Sprintf("mi.type = $%d", argN))
		args = append(args, typ)
		argN++
	}
	if libID, ok := filters["library_id"]; ok && libID != "" {
		clauses = append(clauses, fmt.Sprintf("mi.library_id = $%d", argN))
		args = append(args, libID)
		argN++
	}
	// Genre and year filters: Meilisearch applies these (search_adapter filterable
	// attributes), so the SQL fallback must too or a genre/year-filtered search
	// silently returns unfiltered results while Meili is down. media_catalog (mc)
	// is already joined by the caller.
	if g, ok := filters["genres"]; ok && g != "" {
		if gj, err := json.Marshal([]string{g}); err == nil {
			clauses = append(clauses, fmt.Sprintf("mc.genres @> $%d::jsonb", argN))
			args = append(args, string(gj))
			argN++
		}
	}
	if ymin, ok := filters["year_min"]; ok && ymin != "" {
		if n, err := strconv.Atoi(ymin); err == nil {
			clauses = append(clauses, fmt.Sprintf("mc.year >= $%d", argN))
			args = append(args, n)
			argN++
		}
	}
	if ymax, ok := filters["year_max"]; ok && ymax != "" {
		if n, err := strconv.Atoi(ymax); err == nil {
			clauses = append(clauses, fmt.Sprintf("mc.year <= $%d", argN))
			args = append(args, n)
		}
	}

	if len(clauses) == 0 {
		return "", nil, false
	}
	return strings.Join(clauses, " AND "), args, true
}

func (s *SQLSearchFallback) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args, ok := searchWhere(query, filters)
	if !ok {
		return nil, 0, nil
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
