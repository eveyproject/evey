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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	_, err = r.db.ExecContext(ctx,
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
	if err = r.db.QueryRowContext(ctx,
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
	res, err := r.db.ExecContext(ctx,
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
	err := r.db.QueryRowContext(ctx,
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
	err := r.db.QueryRowContext(ctx,
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
	rows, err := r.db.QueryContext(ctx,
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id = $1`, id)
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
	_, err := r.db.ExecContext(ctx,
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
	query := fmt.Sprintf(
		"SELECT %s %s INNER JOIN collection_items ci ON ci.media_item_id = mi.id WHERE ci.collection_id = $1 ORDER BY mc.premiere_date ASC NULLS LAST LIMIT %d",
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

// GetItemsByCollectionIDs fetches items for every supplied collection in a
// single query and groups the results by collection ID, eliminating the
// per-collection N+1 that ListCollections previously triggered.
//
// No per-collection LIMIT is applied: TMDB franchise collections hold
// O(10–50) items in practice. A ROW_NUMBER() OVER (PARTITION BY …) cap would
// add query complexity with no measurable benefit on realistic data.
func (r *CollectionRepository) GetItemsByCollectionIDs(ctx context.Context, collectionIDs []string) (map[string][]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.collection.GetItemsByCollectionIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("collection.count", len(collectionIDs)))

	if len(collectionIDs) == 0 {
		return map[string][]*library.MediaItem{}, nil
	}

	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	query := fmt.Sprintf(
		"SELECT ci.collection_id, %s %s INNER JOIN collection_items ci ON ci.media_item_id = mi.id WHERE ci.collection_id = ANY($1) ORDER BY ci.collection_id, mc.premiere_date ASC NULLS LAST",
		mediaItemColumns, mediaItemFromJoins)

	rows, err := r.db.QueryContext(ctx, query, pq.Array(collectionIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]*library.MediaItem, len(collectionIDs))
	for rows.Next() {
		var (
			collectionID                                    string
			parentID, scanTitle                             *string
			filePath, fingerprint, containerFormat          *string
			fileSize, durationMs, bitrate                   *int64
			modTimeRaw                                      sql.NullTime
			premiereDate                                    sql.NullTime
			year                                            sql.NullInt64
			providerJSON, genresJSON, studiosJSON, tagsJSON []byte
			communityRating                                 sql.NullFloat64
			officialRating                                  sql.NullString
			certJSON, providerRatingsJSON, trailersJSON     []byte
		)
		item := &library.MediaItem{}
		if err := rows.Scan(
			&collectionID,
			&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &scanTitle,
			&item.SortTitle, &item.OriginalTitle, &item.Overview, &item.Tagline, &item.IndexNumber,
			&item.AddedAt, &item.UpdatedAt,
			&filePath, &fileSize, &fingerprint, &modTimeRaw, &durationMs, &bitrate, &containerFormat,
			&premiereDate, &year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
			&communityRating, &officialRating, &certJSON, &providerRatingsJSON,
			&trailersJSON,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if parentID != nil {
			item.ParentID = *parentID
		}
		if scanTitle != nil {
			item.ScanTitle = *scanTitle
		}
		item.File = buildMediaFile(filePath, fileSize, fingerprint, modTimeRaw, durationMs, bitrate, containerFormat)
		cat, catErr := buildMediaCatalog(premiereDate, year, providerJSON, genresJSON, studiosJSON, tagsJSON)
		if catErr != nil {
			span.RecordError(catErr)
			span.SetStatus(codes.Error, catErr.Error())
			return nil, catErr
		}
		item.Catalog = cat
		item.Classification = buildMediaClassification(communityRating, officialRating, certJSON, providerRatingsJSON)
		if len(trailersJSON) > 0 {
			if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
				slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
			}
		}
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
	err := r.db.QueryRowContext(ctx,
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
	rows, err := r.db.QueryContext(ctx, query, args...)
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

func (q *CollectionSmartQuerier) QueryByRules(ctx context.Context, rules []library.CollectionRule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error) {
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
	query += fmt.Sprintf(" ORDER BY %s %s NULLS LAST", col, dir)

	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := q.db.QueryContext(ctx, query, args...)
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

func buildCollectionRuleCondition(rule library.CollectionRule, argIdx *int) (string, []any) {
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
		op := "="
		switch rule.Operator {
		case "gt":
			op = ">"
		case "lt":
			op = "<"
		case "gte":
			op = ">="
		case "lte":
			op = "<="
		}
		cond := fmt.Sprintf("mc.year %s $%d", op, *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "library_id":
		cond := fmt.Sprintf("mi.library_id = $%d", *argIdx)
		*argIdx++
		return cond, []any{rule.Value}
	case "rating_gte":
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
