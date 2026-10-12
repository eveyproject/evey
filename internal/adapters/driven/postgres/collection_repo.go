package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type CollectionRepository struct {
	db *sql.DB
}

func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

func (r *CollectionRepository) Create(ctx context.Context, col *library.Collection) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.Create")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", col.ID))
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
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		col.ID, col.Name, col.Overview, col.TmdbID, col.PosterURL, col.BackdropURL,
		rulesJSON, sortBy, col.CreatedAt, col.UpdatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *CollectionRepository) Upsert(ctx context.Context, col *library.Collection) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", col.ID))
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
	// RETURNING id so col.ID reflects the PERSISTED row id. On the ON CONFLICT
	// (tmdb_id) update path the surviving row keeps its original id, which differs
	// from the freshly-generated col.ID the caller passed in. Without scanning it
	// back, downstream LinkMediaItem(col.ID, …) targets a non-existent collection
	// id and the FK silently drops every link after the first enrichment of a
	// given TMDB collection (only the first movie ever links).
	if err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`INSERT INTO collections (id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tmdb_id) WHERE tmdb_id IS NOT NULL DO UPDATE SET
		   name = EXCLUDED.name,
		   overview = EXCLUDED.overview,
		   poster_url = EXCLUDED.poster_url,
		   backdrop_url = EXCLUDED.backdrop_url,
		   rules = EXCLUDED.rules,
		   sort_by = EXCLUDED.sort_by,
		   updated_at = EXCLUDED.updated_at
		 RETURNING id`,
		col.ID, col.Name, col.Overview, col.TmdbID, col.PosterURL, col.BackdropURL,
		rulesJSON, sortBy, col.CreatedAt, col.UpdatedAt,
	).Scan(&col.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *CollectionRepository) Update(ctx context.Context, col *library.Collection) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.Update")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", col.ID))
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
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE collections SET name = $1, overview = $2, rules = $3, sort_by = $4, updated_at = $5
		 WHERE id = $6`,
		col.Name, col.Overview, rulesJSON, sortBy, col.UpdatedAt, col.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	n, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		slog.Warn("rows affected", slog.Any("err", rowsErr))
	}
	if n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	col := &library.Collection{}
	var rulesJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at
		 FROM collections WHERE id = $1`, id,
	).Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
		&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return col, nil
}

func (r *CollectionRepository) GetByTmdbID(ctx context.Context, tmdbID int) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.GetByTmdbID")
	defer span.End()
	span.SetAttributes(attribute.Int("collection.tmdb_id", tmdbID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	col := &library.Collection{}
	var rulesJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at
		 FROM collections WHERE tmdb_id = $1`, tmdbID,
	).Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
		&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
	}
	return col, nil
}

func (r *CollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, overview, tmdb_id, poster_url, backdrop_url, rules, sort_by, created_at, updated_at
		 FROM collections ORDER BY name LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.Collection
	for rows.Next() {
		col := &library.Collection{}
		var rulesJSON []byte
		if err := rows.Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
			&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
		}
		result = append(result, col)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *CollectionRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM collections WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	if n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *CollectionRepository) LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.LinkItem")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", collectionID), attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO collection_items (collection_id, media_item_id, sort_order)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		collectionID, mediaItemID, sortOrder,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *CollectionRepository) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.GetItems")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", collectionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	// Release order is the reading order of a saga and stays the default. A
	// collection whose sort_by is "manual" is ordered by the hand-set
	// position instead - the only way to express a chronology that differs
	// from the release order (prequels), and until now sort_order was written
	// by LinkItem and read by nobody. The CASE keeps one query for both: on
	// any other sort_by every row scores 0 and the date decides, exactly as
	// before.
	query := fmt.Sprintf(
		`SELECT %s %s
		 INNER JOIN collection_items ci ON ci.media_item_id = mi.id
		 INNER JOIN collections c ON c.id = ci.collection_id
		 WHERE ci.collection_id = $1
		 ORDER BY CASE WHEN c.sort_by = 'manual' THEN ci.sort_order ELSE 0 END ASC,
		          mc.premiere_date ASC NULLS LAST, mi.id
		 LIMIT %d`,
		mediaItemColumns, mediaItemFromJoins, limit)
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	items, err := r.queryMediaItems(ctx, query, collectionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return items, nil
}

// GetItemsByCollectionIDs returns, per supplied collection, a VISIBILITY
// PROJECTION of its items - only what ListCollections needs to decide whether a
// collection is visible to the caller: id, library_id and the parental-rating
// fields (tags, official_rating, certifications_by_country). It deliberately
// does NOT hydrate file metadata, trailers, genres, overview, etc.: the full
// hydration ran a per-row trailers subquery plus several JSONB parses just to
// test visibility. Grouped by collection ID in one query - no per-collection
// N+1. This is the only caller; do NOT use it where a fully populated MediaItem
// is expected.
//
// No per-collection LIMIT is applied: TMDB franchise collections hold O(10-50)
// items in practice.
func (r *CollectionRepository) ListMoviesWithoutCollection(ctx context.Context, afterID string, limit int) ([]library.UnlinkedMovie, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.ListMoviesWithoutCollection")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.id, mc.provider_ids->>'tmdb'
		 FROM media_items mi
		 JOIN media_catalog mc ON mc.media_item_id = mi.id
		 LEFT JOIN collection_items ci ON ci.media_item_id = mi.id
		 WHERE mi.type = 'movie'
		   AND ci.media_item_id IS NULL
		   AND COALESCE(mc.provider_ids->>'tmdb', '') <> ''
		   AND mi.id > $1
		 ORDER BY mi.id
		 LIMIT $2`, afterID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var out []library.UnlinkedMovie
	for rows.Next() {
		var m library.UnlinkedMovie
		if err := rows.Scan(&m.MediaItemID, &m.ProviderID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

func (r *CollectionRepository) GetItemsByCollectionIDs(ctx context.Context, collectionIDs []string) (map[string][]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.GetItemsByCollectionIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("collection.count", len(collectionIDs)))

	if len(collectionIDs) == 0 {
		return map[string][]*library.MediaItem{}, nil
	}

	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT ci.collection_id, mi.id, mi.library_id, mc.tags,
		        mcl.official_rating, mcl.certifications_by_country
		   FROM media_items mi
		   INNER JOIN collection_items ci ON ci.media_item_id = mi.id
		   LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
		   LEFT JOIN media_classifications mcl ON mcl.media_item_id = mi.id
		  WHERE ci.collection_id = ANY($1)
		  ORDER BY ci.collection_id, mc.premiere_date ASC NULLS LAST`,
		pq.Array(collectionIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]*library.MediaItem, len(collectionIDs))
	for rows.Next() {
		var (
			collectionID       string
			tagsJSON, certJSON []byte
			officialRating     sql.NullString
		)
		item := &library.MediaItem{}
		if err := rows.Scan(&collectionID, &item.ID, &item.LibraryID, &tagsJSON,
			&officialRating, &certJSON); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		// Partial item: only the fields the visibility filters read.
		cat, catErr := buildMediaCatalog(sql.NullTime{}, sql.NullInt64{}, nil, nil, nil, tagsJSON)
		if catErr != nil {
			span.RecordError(catErr)
			span.SetStatus(codes.Error, catErr.Error())
			return nil, catErr
		}
		item.Catalog = cat
		item.Classification = buildMediaClassification(sql.NullFloat64{}, officialRating, certJSON, nil)
		result[collectionID] = append(result[collectionID], item)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *CollectionRepository) CountItems(ctx context.Context, collectionID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.CountItems")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", collectionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collection_items WHERE collection_id = $1`, collectionID,
	).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return count, nil
}

func (r *CollectionRepository) queryMediaItems(ctx context.Context, query string, args ...any) ([]*library.MediaItem, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaItem
	for rows.Next() {
		item := &library.MediaItem{}
		if err := scanMediaItemFields(rows, item); err != nil {
			return nil, err
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

func (q *CollectionSmartQuerier) QueryByRules(ctx context.Context, rules []library.CollectionRule, sortBy, sortOrder string, limit, offset int) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.QueryByRules")
	defer span.End()
	span.SetAttributes(attribute.Int("collection.rule.count", len(rules)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	query := fmt.Sprintf("SELECT %s %s", mediaItemColumns, mediaItemFromJoins)

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

	// sortBy values map to columns in the joined schema. Year + rating
	// live in their sub-tables now.
	validSort := map[string]string{
		"title":    "mi.sort_title",
		"year":     "mc.year",
		"added_at": "mi.added_at",
		"rating":   "mcl.community_rating",
	}
	col, ok := validSort[sortBy]
	if !ok {
		col = "mi.added_at"
	}
	dir := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		dir = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s NULLS LAST, mi.id", col, dir)

	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}

	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	items, err := q.scanItems(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return items, nil
}

func (q *CollectionSmartQuerier) CountByRules(ctx context.Context, rules []library.CollectionRule) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.CountByRules")
	defer span.End()
	span.SetAttributes(attribute.Int("collection.rule.count", len(rules)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Same FROM+joins+WHERE as QueryByRules but only counts - DISTINCT mi.id so a
	// 1:many join can't inflate the total.
	query := "SELECT COUNT(DISTINCT mi.id) " + mediaItemFromJoins

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

	var count int
	if err := txFromCtx(ctx, q.db).QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return count, nil
}

func buildCollectionRuleCondition(rule library.CollectionRule, argIdx *int) (string, []any) {
	// Defensive: the web client already strips empty-value rules before save
	// (SmartCollectionDialog filters r.value.trim() !== ""), but a malformed
	// API client could still send one. An empty value would yield a
	// match-nothing predicate (e.g. mi.type = ''), so skip the rule instead.
	if rule.Value == "" {
		return "", nil
	}
	switch rule.Field {
	case "genre":
		val, err := json.Marshal([]string{rule.Value})
		if err != nil {
			slog.Warn("marshal genre rule", slog.Any("err", err))
			return "", nil
		}
		cond := fmt.Sprintf("mc.genres @> $%d::jsonb", *argIdx)
		*argIdx++
		return cond, []any{string(val)}
	case "type":
		cond := fmt.Sprintf("mi.type = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "year":
		var op string
		switch rule.Operator {
		case "", "eq":
			op = "="
		case "gt":
			op = ">"
		case "lt":
			op = "<"
		case "gte":
			op = ">="
		case "lte":
			op = "<="
		default:
			// Unknown operator from a malformed client: skip rather than
			// silently coercing to "=".
			return "", nil
		}
		// year is an integer column; a non-numeric value would raise a
		// runtime cast error (500). Skip the rule instead.
		if _, err := strconv.Atoi(rule.Value); err != nil {
			return "", nil
		}
		cond := fmt.Sprintf("mc.year %s $%d", op, *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "library_id":
		cond := fmt.Sprintf("mi.library_id = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "rating_gte":
		// community_rating is numeric; a non-numeric value would raise a
		// runtime cast error (500). Skip the rule instead.
		if _, err := strconv.ParseFloat(rule.Value, 64); err != nil {
			return "", nil
		}
		cond := fmt.Sprintf("mcl.community_rating >= $%d::double precision", *argIdx)
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
		if err := scanMediaItemFields(rows, item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

var _ library.CollectionCommandRepo = (*CollectionRepository)(nil)
var _ library.CollectionQueryRepo = (*CollectionRepository)(nil)

// UnlinkItem removes one member from a collection. Absent membership is not an
// error: the caller asked for a state, and that state already holds.
func (r *CollectionRepository) UnlinkItem(ctx context.Context, collectionID, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.UnlinkItem")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", collectionID), attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM collection_items WHERE collection_id = $1 AND media_item_id = $2`,
		collectionID, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// SetItemOrder stamps the hand-set position of the supplied members: position
// in the slice IS the new sort_order. Members left out keep theirs, so a
// caller may reorder a page without shipping the whole collection.
func (r *CollectionRepository) SetItemOrder(ctx context.Context, collectionID string, mediaItemIDs []string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.SetItemOrder")
	defer span.End()
	span.SetAttributes(attribute.String("collection.id", collectionID), attribute.Int("items", len(mediaItemIDs)))
	if len(mediaItemIDs) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Own transaction (not txFromCtx): the drift check below must be able to
	// roll the UPDATE back, which an ambient caller-owned tx would not allow.
	// Same choice as PlaylistItemRepo.Reorder, its exact mirror.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("CollectionRepo.SetItemOrder begin tx: %w", err)
	}
	defer tx.Rollback()
	// One statement: a per-row UPDATE loop would leave a half-reordered
	// collection behind if the connection dropped mid-way.
	res, err := tx.ExecContext(ctx,
		`UPDATE collection_items ci
		 SET sort_order = pos.ord
		 FROM (SELECT unnest($2::text[]) AS media_item_id, generate_subscripts($2::text[], 1) AS ord) pos
		 WHERE ci.collection_id = $1 AND ci.media_item_id = pos.media_item_id`,
		collectionID, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	// The handler's membership pre-check runs OUTSIDE this transaction: an
	// item removed in between would silently half-apply the reorder. Fewer
	// matched rows than distinct requested ids = drift, roll back.
	if n, aerr := res.RowsAffected(); aerr == nil {
		distinct := make(map[string]struct{}, len(mediaItemIDs))
		for _, mid := range mediaItemIDs {
			distinct[mid] = struct{}{}
		}
		if int(n) != len(distinct) {
			span.SetStatus(codes.Error, library.ErrCollectionReorderDrift.Error())
			return library.ErrCollectionReorderDrift
		}
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("CollectionRepo.SetItemOrder commit: %w", err)
	}
	return nil
}

// GetByMediaItemID returns the collections holding this title.
func (r *CollectionRepository) GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT c.id, c.name, c.overview, c.tmdb_id, c.poster_url, c.backdrop_url, c.rules, c.sort_by, c.created_at, c.updated_at
		 FROM collections c
		 INNER JOIN collection_items ci ON ci.collection_id = c.id
		 WHERE ci.media_item_id = $1
		 ORDER BY c.name`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var out []*library.Collection
	for rows.Next() {
		col := &library.Collection{}
		var rulesJSON []byte
		if err := rows.Scan(&col.ID, &col.Name, &col.Overview, &col.TmdbID, &col.PosterURL, &col.BackdropURL,
			&rulesJSON, &col.SortBy, &col.CreatedAt, &col.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &col.Rules); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "rules"), slog.Any("err", err))
		}
		out = append(out, col)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}
