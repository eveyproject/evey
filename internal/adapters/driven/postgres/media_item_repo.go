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
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// DefaultListLimit caps every paginated List query at 500 rows so a buggy
// or malicious caller cannot drag the whole library across the wire in a
// single request. Callers asking for 0 (or > 500) get clamped down.
const DefaultListLimit = 500

// mediaItemColumns is the set of columns SELECTed for a full MediaItem
// view: 12 root columns from media_items + the LEFT-JOIN'd sub-aggregate
// columns from media_files / media_catalog / media_classifications, plus
// a jsonb_agg subquery for the (1:N) trailers. The order matches what
// scanMediaItemFields expects.
const mediaItemColumns = `mi.id, mi.library_id, mi.parent_id, mi.type, mi.title, mi.scan_title,
	 mi.sort_title, mi.original_title, mi.overview, mi.tagline, mi.index_number,
	 mi.added_at, mi.updated_at,
	 mf.file_path, mf.file_size, mf.file_fingerprint, mf.mod_time, mf.duration_ms, mf.bitrate, mf.container_format,
	 mc.premiere_date, mc.year, mc.provider_ids, mc.genres, mc.studios, mc.tags,
	 mcl.community_rating, mcl.official_rating, mcl.certifications_by_country, mcl.provider_ratings,
	 COALESCE((SELECT jsonb_agg(jsonb_build_object('Site', mt.site, 'Key', mt.key,
	     'Language', mt.language, 'Official', mt.official, 'Type', mt.type,
	     'UseSubtitles', mt.use_subtitles, 'SubtitleLanguage', mt.subtitle_language) ORDER BY mt.position)
	     FROM media_trailers mt WHERE mt.media_item_id = mi.id), '[]'::jsonb) AS trailers`

const mediaItemFromJoins = `FROM media_items mi
	 LEFT JOIN media_files mf ON mf.media_item_id = mi.id
	 LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
	 LEFT JOIN media_classifications mcl ON mcl.media_item_id = mi.id`

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(db *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: db}
}

// withMediaItemTx runs f inside a single transaction so that a partial
// sub-aggregate write never leaves an item half-persisted.
func (r *MediaItemRepository) withMediaItemTx(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MediaItemRepository) Create(ctx context.Context, item *library.MediaItem) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	scanTitle := item.ScanTitle
	if scanTitle == "" {
		scanTitle = item.Title
	}
	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}

	err := r.withMediaItemTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO media_items (id, library_id, parent_id, type, title, scan_title, sort_title, original_title,
			 overview, tagline, index_number, added_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			item.ID, item.LibraryID, parentID, item.Type, item.Title, scanTitle,
			item.SortTitle, item.OriginalTitle, item.Overview, item.Tagline,
			item.IndexNumber, item.AddedAt, item.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert media_items: %w", err)
		}
		if err := upsertMediaFile(ctx, tx, item); err != nil {
			return err
		}
		if err := upsertMediaCatalog(ctx, tx, item); err != nil {
			return err
		}
		if err := upsertMediaClassification(ctx, tx, item); err != nil {
			return err
		}
		if err := replaceMediaTrailers(ctx, tx, string(item.ID), item.Trailers); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create: %w", err)
	}
	r.bumpSeriesLatestChildAddedAt(ctx, item)
	return nil
}

func (r *MediaItemRepository) Update(ctx context.Context, item *library.MediaItem) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}

	err := r.withMediaItemTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE media_items SET library_id=$1, parent_id=$2, type=$3, title=$4, sort_title=$5,
			 original_title=$6, overview=$7, tagline=$8, index_number=$9, updated_at=$10
			 WHERE id=$11`,
			item.LibraryID, parentID, item.Type, item.Title, item.SortTitle, item.OriginalTitle,
			item.Overview, item.Tagline, item.IndexNumber, item.UpdatedAt, item.ID,
		)
		if err != nil {
			return fmt.Errorf("update media_items: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return shared.ErrNotFound
		}
		if err := upsertMediaFile(ctx, tx, item); err != nil {
			return err
		}
		if err := upsertMediaCatalog(ctx, tx, item); err != nil {
			return err
		}
		if err := upsertMediaClassification(ctx, tx, item); err != nil {
			return err
		}
		if err := replaceMediaTrailers(ctx, tx, string(item.ID), item.Trailers); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return err
		}
		return fmt.Errorf("MediaItemRepo.Update: %w", err)
	}
	r.bumpSeriesLatestChildAddedAt(ctx, item)
	return nil
}

// upsertMediaFile writes (or deletes) the 1:1 media_files row driven by
// item.File. nil File (or empty Path) means the item owns no on-disk
// artefact (series root, season, album); we DELETE so a subsequent JOIN
// cleanly returns NULL for every file column.
func upsertMediaFile(ctx context.Context, tx *sql.Tx, item *library.MediaItem) error {
	if item.File == nil || item.File.Path == "" {
		_, err := tx.ExecContext(ctx, `DELETE FROM media_files WHERE media_item_id = $1`, string(item.ID))
		if err != nil {
			return fmt.Errorf("delete media_files: %w", err)
		}
		return nil
	}
	f := item.File
	var fingerprint *string
	if f.Fingerprint != "" {
		fingerprint = &f.Fingerprint
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO media_files (media_item_id, file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   file_path = EXCLUDED.file_path,
		   file_size = EXCLUDED.file_size,
		   file_fingerprint = EXCLUDED.file_fingerprint,
		   mod_time = EXCLUDED.mod_time,
		   duration_ms = EXCLUDED.duration_ms,
		   bitrate = EXCLUDED.bitrate,
		   container_format = EXCLUDED.container_format`,
		string(item.ID), f.Path, f.Size, fingerprint,
		f.ModTime, f.DurationMs, f.Bitrate, f.ContainerFormat,
	)
	if err != nil {
		return fmt.Errorf("upsert media_files: %w", err)
	}
	return nil
}

// upsertMediaCatalog mirrors upsertMediaFile for the catalog row. Absent
// when every catalog field is zero/empty (the row would only carry
// defaults and a stale FK).
func upsertMediaCatalog(ctx context.Context, tx *sql.Tx, item *library.MediaItem) error {
	c := item.Catalog
	hasData := c != nil && (c.PremiereDate != nil || c.Year != nil ||
		len(c.ProviderIDs) > 0 || len(c.Genres) > 0 ||
		len(c.Studios) > 0 || len(c.Tags) > 0)
	if !hasData {
		_, err := tx.ExecContext(ctx, `DELETE FROM media_catalog WHERE media_item_id = $1`, string(item.ID))
		if err != nil {
			return fmt.Errorf("delete media_catalog: %w", err)
		}
		return nil
	}
	providerJSON, err := json.Marshal(orEmptyMap(c.ProviderIDs))
	if err != nil {
		return fmt.Errorf("marshal provider_ids: %w", err)
	}
	genresJSON, err := json.Marshal(orEmptySlice(c.Genres))
	if err != nil {
		return fmt.Errorf("marshal genres: %w", err)
	}
	studiosJSON, err := json.Marshal(orEmptySlice(c.Studios))
	if err != nil {
		return fmt.Errorf("marshal studios: %w", err)
	}
	tagsJSON, err := json.Marshal(orEmptySlice(c.Tags))
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO media_catalog (media_item_id, premiere_date, year, provider_ids, genres, studios, tags)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   premiere_date = EXCLUDED.premiere_date,
		   year = EXCLUDED.year,
		   provider_ids = EXCLUDED.provider_ids,
		   genres = EXCLUDED.genres,
		   studios = EXCLUDED.studios,
		   tags = EXCLUDED.tags`,
		string(item.ID), c.PremiereDate, c.Year, providerJSON, genresJSON, studiosJSON, tagsJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert media_catalog: %w", err)
	}
	return nil
}

func upsertMediaClassification(ctx context.Context, tx *sql.Tx, item *library.MediaItem) error {
	c := item.Classification
	hasData := c != nil && (c.CommunityRating != nil || c.OfficialRating != "" ||
		len(c.CertificationsByCountry) > 0 || len(c.ProviderRatings) > 0)
	if !hasData {
		_, err := tx.ExecContext(ctx, `DELETE FROM media_classifications WHERE media_item_id = $1`, string(item.ID))
		if err != nil {
			return fmt.Errorf("delete media_classifications: %w", err)
		}
		return nil
	}
	certJSON, err := json.Marshal(orEmptyMap(c.CertificationsByCountry))
	if err != nil {
		return fmt.Errorf("marshal certifications: %w", err)
	}
	ratingsJSON, err := json.Marshal(orEmptyFloatMap(c.ProviderRatings))
	if err != nil {
		return fmt.Errorf("marshal provider_ratings: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO media_classifications (media_item_id, community_rating, official_rating, certifications_by_country, provider_ratings)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   community_rating = EXCLUDED.community_rating,
		   official_rating = EXCLUDED.official_rating,
		   certifications_by_country = EXCLUDED.certifications_by_country,
		   provider_ratings = EXCLUDED.provider_ratings`,
		string(item.ID), c.CommunityRating, c.OfficialRating, certJSON, ratingsJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert media_classifications: %w", err)
	}
	return nil
}

// replaceMediaTrailers deletes-then-reinserts so the in-memory slice is
// the source of truth, matching how the previous JSONB array column
// behaved. Cheap because trailers per item is typically <10.
func replaceMediaTrailers(ctx context.Context, tx *sql.Tx, mediaItemID string, trailers []library.Trailer) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_trailers WHERE media_item_id = $1`, mediaItemID); err != nil {
		return fmt.Errorf("delete media_trailers: %w", err)
	}
	for i, t := range trailers {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO media_trailers (id, media_item_id, position, site, key, language, official, type, use_subtitles, subtitle_language)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			fmt.Sprintf("tlr-%s-%d", mediaItemID, i),
			mediaItemID, i, t.Site, t.Key, t.Language, t.Official, t.Type,
			t.UseSubtitles, t.SubtitleLanguage,
		)
		if err != nil {
			return fmt.Errorf("insert media_trailers: %w", err)
		}
	}
	return nil
}

func orEmptyMap[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

func orEmptySlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func orEmptyFloatMap(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
}

// bumpSeriesLatestChildAddedAt keeps media_items.latest_child_added_at in
// sync on the grandparent series when an episode is created or touched.
// Best-effort — failure is logged but not propagated.
func (r *MediaItemRepository) bumpSeriesLatestChildAddedAt(ctx context.Context, item *library.MediaItem) {
	if item == nil || item.Type != library.Episode || item.ParentID == "" {
		return
	}
	when := item.AddedAt
	if item.UpdatedAt.After(when) {
		when = item.UpdatedAt
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_items
		SET latest_child_added_at = $1
		WHERE type = 'series'
		  AND id = (SELECT parent_id FROM media_items WHERE id = $2 AND type = 'season')
		  AND (latest_child_added_at IS NULL OR latest_child_added_at < $1)
	`, when, item.ParentID)
	if err != nil {
		slog.Warn("bump series latest_child_added_at", slog.String("episode_id", string(item.ID)), slog.Any("err", err))
	}
}

func (r *MediaItemRepository) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mi.id = $1", mediaItemColumns, mediaItemFromJoins), string(id)))
}

func (r *MediaItemRepository) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mf.file_path = $1", mediaItemColumns, mediaItemFromJoins), path))
}

func (r *MediaItemRepository) GetByFileFingerprint(ctx context.Context, fingerprint string) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mf.file_fingerprint = $1", mediaItemColumns, mediaItemFromJoins), fingerprint))
}

func (r *MediaItemRepository) GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mi.library_id = $1 AND mi.type = $2 AND mi.title = $3 AND ($4 = '' OR mi.parent_id = $4) LIMIT 1", mediaItemColumns, mediaItemFromJoins),
		string(libraryID), itemType, title, parentID))
}

func (r *MediaItemRepository) buildWhereClause(f library.ListMediaItemsFilter) (string, []any) {
	var conditions []string
	var args []any
	argIdx := 1
	if f.LibraryID != nil {
		conditions = append(conditions, fmt.Sprintf("mi.library_id = $%d", argIdx))
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
		conditions = append(conditions, "mi.library_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if f.Type != nil {
		conditions = append(conditions, fmt.Sprintf("mi.type = $%d", argIdx))
		args = append(args, *f.Type)
		argIdx++
	}
	if f.ParentID != nil {
		conditions = append(conditions, fmt.Sprintf("mi.parent_id = $%d", argIdx))
		args = append(args, *f.ParentID)
		argIdx++
	}
	if f.Year != nil {
		conditions = append(conditions, fmt.Sprintf("mc.year = $%d", argIdx))
		args = append(args, *f.Year)
		argIdx++
	}
	if f.YearFrom != 0 {
		conditions = append(conditions, fmt.Sprintf("mc.year >= $%d", argIdx))
		args = append(args, f.YearFrom)
		argIdx++
	}
	if f.YearTo != 0 {
		conditions = append(conditions, fmt.Sprintf("mc.year <= $%d", argIdx))
		args = append(args, f.YearTo)
		argIdx++
	}
	if f.MinRating != 0 {
		conditions = append(conditions, fmt.Sprintf("mcl.community_rating >= $%d", argIdx))
		args = append(args, f.MinRating)
		argIdx++
	}
	for _, g := range f.Genres {
		conditions = append(conditions, fmt.Sprintf("mc.genres @> $%d::jsonb", argIdx))
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
		conditions = append(conditions, "(mcl.official_rating IS NULL OR mcl.official_rating = '' OR mcl.official_rating = ANY(ARRAY["+strings.Join(placeholders, ",")+"]))")
	}
	if f.PersonID != nil {
		conditions = append(conditions, fmt.Sprintf("mi.id IN (SELECT media_item_id FROM media_people WHERE person_id = $%d)", argIdx))
		args = append(args, *f.PersonID)
		argIdx++
	}
	if f.AfterID != "" {
		conditions = append(conditions, fmt.Sprintf("mi.id > $%d", argIdx))
		args = append(args, f.AfterID)
		// argIdx intentionally not bumped: AfterID is the last placeholder
		// the filter produces. Keep this branch last so later additions
		// get a free slot — if you add a new condition below, re-add the
		// increment here.
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

// resolveSortKey returns the canonical key (matches SortColumnMap input
// names) and the SQL column for a given filter. Falls back to title.
func resolveSortKey(f library.ListMediaItemsFilter) (key, col string) {
	if f.SortField != "" {
		if c, ok := SortColumnMap[f.SortField]; ok {
			return f.SortField, c
		}
	}
	return "title", "mi.sort_title"
}

// whereAnd appends a predicate to an existing WHERE clause, or starts
// a new one if where is empty.
func whereAnd(where, predicate string) string {
	if where == "" {
		return " WHERE " + predicate
	}
	return where + " AND " + predicate
}

func (r *MediaItemRepository) Count(ctx context.Context, f library.ListMediaItemsFilter) (int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.buildWhereClause(f)
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+mediaItemFromJoins+where, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("MediaItemRepo.Count: %w", err)
	}
	return count, nil
}

func (r *MediaItemRepository) List(ctx context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.buildWhereClause(f)

	sortKey, sortField := resolveSortKey(f)
	sortOrder := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortOrder = "DESC"
	}

	// Cursor takes precedence over Offset. Reject cursors generated for a
	// different sort key — re-using them would produce undefined ordering.
	if f.Cursor.LastID != "" {
		if f.Cursor.SortField != sortKey {
			return nil, shared.ErrCursorMismatch
		}
		cmp := ">"
		if sortOrder == "DESC" {
			cmp = "<"
		}
		nextIdx := len(args) + 1
		where = whereAnd(where, fmt.Sprintf("(%s, mi.id) %s ($%d, $%d)", sortField, cmp, nextIdx, nextIdx+1))
		args = append(args, f.Cursor.SortValue, f.Cursor.LastID)
	}

	query := fmt.Sprintf("SELECT %s %s%s", mediaItemColumns, mediaItemFromJoins, where)

	switch {
	case f.Cursor.LastID != "":
		query += fmt.Sprintf(" ORDER BY %s %s, mi.id %s", sortField, sortOrder, sortOrder)
	case f.AfterID != "":
		query += " ORDER BY mi.id ASC"
	case sortField == "added_at" || sortField == "mi.added_at":
		query += fmt.Sprintf(` ORDER BY CASE WHEN mi.type = 'series'
			 THEN COALESCE(mi.latest_child_added_at, mi.added_at)
			 ELSE mi.added_at END %s`, sortOrder)
	default:
		query += fmt.Sprintf(" ORDER BY %s %s", sortField, sortOrder)
	}

	limit := f.Limit
	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Cursor.LastID == "" && f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.List scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *MediaItemRepository) Delete(ctx context.Context, id shared.MediaItemID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var itemType, parentID string
	_ = r.db.QueryRowContext(ctx, `SELECT type, COALESCE(parent_id, '') FROM media_items WHERE id = $1`, string(id)).Scan(&itemType, &parentID)
	res, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	if itemType == string(library.Episode) && parentID != "" {
		r.recomputeSeriesLatestChildAddedAtFromSeason(ctx, parentID)
	}
	return nil
}

func (r *MediaItemRepository) recomputeSeriesLatestChildAddedAtFromSeason(ctx context.Context, seasonID string) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_items s
		SET latest_child_added_at = (
			SELECT MAX(ep.added_at) FROM media_items ep
			WHERE ep.type = 'episode' AND ep.parent_id IN (
				SELECT id FROM media_items WHERE parent_id = s.id AND type = 'season'
			)
		)
		WHERE s.id = (SELECT parent_id FROM media_items WHERE id = $1 AND type = 'season')
		  AND s.type = 'series'
	`, seasonID)
	if err != nil {
		slog.Warn("recompute series latest_child_added_at", slog.String("season_id", seasonID), slog.Any("err", err))
	}
}

func (r *MediaItemRepository) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("MediaItemRepo.DeleteByIDs: %w", err)
	}
	return nil
}

func (r *MediaItemRepository) ListFilePaths(ctx context.Context, libraryID shared.LibraryID) (map[string]string, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT mi.id, mf.file_path
		 FROM media_items mi
		 INNER JOIN media_files mf ON mf.media_item_id = mi.id
		 WHERE mi.library_id = $1 AND mf.file_path <> ''`,
		libraryID,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListFilePaths: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListFilePaths scan: %w", err)
		}
		result[id] = path
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListFilePaths rows: %w", err)
	}
	return result, nil
}

const listByLibraryIDMaxRows = 10000

func (r *MediaItemRepository) ListByLibraryID(ctx context.Context, libraryID shared.LibraryID) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mi.library_id = $1 ORDER BY mi.added_at DESC LIMIT $2", mediaItemColumns, mediaItemFromJoins),
		libraryID, listByLibraryIDMaxRows)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListByLibraryID: %w", err)
	}
	defer rows.Close()
	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListByLibraryID scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListByLibraryID rows: %w", err)
	}
	return result, nil
}

func (r *MediaItemRepository) ListScanSummaries(ctx context.Context, libraryID shared.LibraryID) ([]library.MediaItemScanRow, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	const batchSize = 5000
	var result []library.MediaItemScanRow
	for offset := 0; ; offset += batchSize {
		batch, err := r.listScanSummariesBatch(ctx, libraryID, batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListScanSummaries: %w", err)
		}
		result = append(result, batch...)
		if len(batch) < batchSize {
			break
		}
	}
	return result, nil
}

func (r *MediaItemRepository) listScanSummariesBatch(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]library.MediaItemScanRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mi.id,
		        COALESCE(mf.file_path, ''),
		        COALESCE(mf.file_fingerprint, ''),
		        COALESCE(mf.file_size, 0),
		        COALESCE(mf.mod_time, '1970-01-01 00:00:00+00'::timestamptz)
		 FROM media_items mi
		 LEFT JOIN media_files mf ON mf.media_item_id = mi.id
		 WHERE mi.library_id = $1 ORDER BY mi.id LIMIT $2 OFFSET $3`,
		libraryID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listScanSummariesBatch: %w", err)
	}
	defer rows.Close()
	var result []library.MediaItemScanRow
	for rows.Next() {
		var row library.MediaItemScanRow
		if err := rows.Scan(&row.ID, &row.FilePath, &row.FileFingerprint, &row.FileSize, &row.ModTime); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.listScanSummariesBatch scan: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listScanSummariesBatch rows: %w", err)
	}
	return result, nil
}

// SeriesAggregate walks the parent_id tree rooted at seriesID via a
// recursive CTE and sums the runtime of every episode it finds. Joined
// with media_files since duration_ms now lives there.
func (r *MediaItemRepository) SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var agg library.SeriesAggregate
	err := r.db.QueryRowContext(ctx, `
		WITH RECURSIVE descendants AS (
		  SELECT id, parent_id, type FROM media_items WHERE id = $1
		  UNION ALL
		  SELECT m.id, m.parent_id, m.type FROM media_items m
		  INNER JOIN descendants d ON m.parent_id = d.id
		)
		SELECT
		  COALESCE(SUM(COALESCE(mf.duration_ms, 0)) FILTER (WHERE d.type = 'episode' AND mf.duration_ms > 0), 0) AS total_ms,
		  COUNT(*) FILTER (WHERE d.type = 'episode') AS episode_count,
		  COUNT(DISTINCT d.parent_id) FILTER (WHERE d.type = 'episode') AS season_count
		FROM descendants d
		LEFT JOIN media_files mf ON mf.media_item_id = d.id`,
		string(seriesID)).Scan(&agg.TotalDurationMs, &agg.EpisodeCount, &agg.SeasonCount)
	if err != nil {
		return library.SeriesAggregate{}, fmt.Errorf("MediaItemRepo.SeriesAggregate: %w", err)
	}
	return agg, nil
}

func (r *MediaItemRepository) ListContainerInfo(ctx context.Context, libraryID shared.LibraryID) ([]library.MediaItemContainerRow, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	const batchSize = 5000
	var result []library.MediaItemContainerRow
	for offset := 0; ; offset += batchSize {
		batch, err := r.listContainerInfoBatch(ctx, libraryID, batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListContainerInfo: %w", err)
		}
		result = append(result, batch...)
		if len(batch) < batchSize {
			break
		}
	}
	return result, nil
}

func (r *MediaItemRepository) listContainerInfoBatch(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]library.MediaItemContainerRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, parent_id, type FROM media_items WHERE library_id = $1 ORDER BY id LIMIT $2 OFFSET $3`,
		libraryID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listContainerInfoBatch: %w", err)
	}
	defer rows.Close()
	var result []library.MediaItemContainerRow
	for rows.Next() {
		var row library.MediaItemContainerRow
		var parentID *string
		if err := rows.Scan(&row.ID, &parentID, &row.Type); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.listContainerInfoBatch scan: %w", err)
		}
		if parentID != nil {
			row.ParentID = *parentID
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listContainerInfoBatch rows: %w", err)
	}
	return result, nil
}

// rowScanner abstracts sql.Row.Scan and sql.Rows.Scan so the row variant
// and the iterator variant can share the same column-mapping logic.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanMediaItemFields populates a flat MediaItem from the column set
// produced by mediaItemColumns + mediaItemFromJoins. Sub-aggregate
// columns come back NULL when the LEFT JOIN found no matching row, so
// every read is guarded with a sql.Null* helper or a *string pointer.
func scanMediaItemFields(src rowScanner, item *library.MediaItem) error {
	var (
		parentID, scanTitle                              *string
		filePath, fingerprint, containerFormat           *string
		fileSize, durationMs, bitrate                    *int64
		modTimeRaw                                       sql.NullTime
		premiereDate                                     sql.NullTime
		year                                             sql.NullInt64
		providerJSON, genresJSON, studiosJSON, tagsJSON  []byte
		communityRating                                  sql.NullFloat64
		officialRating                                   sql.NullString
		certJSON, providerRatingsJSON, trailersJSON      []byte
	)

	if err := src.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &scanTitle,
		&item.SortTitle, &item.OriginalTitle, &item.Overview, &item.Tagline, &item.IndexNumber,
		&item.AddedAt, &item.UpdatedAt,
		&filePath, &fileSize, &fingerprint, &modTimeRaw, &durationMs, &bitrate, &containerFormat,
		&premiereDate, &year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
		&communityRating, &officialRating, &certJSON, &providerRatingsJSON,
		&trailersJSON,
	); err != nil {
		return err
	}
	if parentID != nil {
		item.ParentID = *parentID
	}
	if scanTitle != nil {
		item.ScanTitle = *scanTitle
	}

	// File sub-aggregate. Allocated only when the LEFT JOIN found a
	// media_files row (filePath non-nil). Container types (series/season/
	// album) leave item.File == nil.
	if filePath != nil {
		f := &library.MediaFile{Path: *filePath}
		if fileSize != nil {
			f.Size = *fileSize
		}
		if fingerprint != nil {
			f.Fingerprint = *fingerprint
		}
		if modTimeRaw.Valid {
			f.ModTime = modTimeRaw.Time
		}
		if durationMs != nil {
			f.DurationMs = *durationMs
		}
		if bitrate != nil {
			f.Bitrate = *bitrate
		}
		if containerFormat != nil {
			f.ContainerFormat = *containerFormat
		}
		item.File = f
	}

	// Catalog sub-aggregate. Allocated only when the LEFT JOIN found a
	// media_catalog row OR any of the JSONB arrays came back non-empty.
	if premiereDate.Valid || year.Valid || len(providerJSON) > 0 || len(genresJSON) > 0 || len(studiosJSON) > 0 || len(tagsJSON) > 0 {
		cat := &library.MediaCatalog{}
		if premiereDate.Valid {
			t := premiereDate.Time
			cat.PremiereDate = &t
		}
		if year.Valid {
			y := int(year.Int64)
			cat.Year = &y
		}
		// Required: provider_ids and genres drive joins/filters; corrupt JSON
		// here is a real DB integrity error, not a soft warning.
		if len(providerJSON) > 0 {
			if err := json.Unmarshal(providerJSON, &cat.ProviderIDs); err != nil {
				return fmt.Errorf("unmarshal provider_ids: %w", err)
			}
		}
		if len(genresJSON) > 0 {
			if err := json.Unmarshal(genresJSON, &cat.Genres); err != nil {
				return fmt.Errorf("unmarshal genres: %w", err)
			}
		}
		if len(studiosJSON) > 0 {
			if err := json.Unmarshal(studiosJSON, &cat.Studios); err != nil {
				slog.Warn("json unmarshal", slog.String("field", "studios"), slog.Any("err", err))
			}
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &cat.Tags); err != nil {
				slog.Warn("json unmarshal", slog.String("field", "tags"), slog.Any("err", err))
			}
		}
		item.Catalog = cat
	}

	// Classification sub-aggregate. Allocated only when at least one
	// rating-related column came back non-NULL.
	if communityRating.Valid || officialRating.Valid || len(certJSON) > 2 || len(providerRatingsJSON) > 2 {
		cl := &library.MediaClassification{}
		if communityRating.Valid {
			v := communityRating.Float64
			cl.CommunityRating = &v
		}
		if officialRating.Valid {
			cl.OfficialRating = officialRating.String
		}
		if len(certJSON) > 0 {
			if err := json.Unmarshal(certJSON, &cl.CertificationsByCountry); err != nil {
				slog.Warn("json unmarshal", slog.String("field", "certifications_by_country"), slog.Any("err", err))
			}
		}
		if len(providerRatingsJSON) > 0 {
			if err := json.Unmarshal(providerRatingsJSON, &cl.ProviderRatings); err != nil {
				slog.Warn("json unmarshal", slog.String("field", "provider_ratings"), slog.Any("err", err))
			}
		}
		item.Classification = cl
	}

	// Trailers (1:N → JSONB array via subquery → flat slice).
	if len(trailersJSON) > 0 {
		if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
		}
	}

	return nil
}

func (r *MediaItemRepository) scanMediaItem(row *sql.Row) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	err := scanMediaItemFields(row, item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.scanMediaItem: %w", err)
	}
	return item, nil
}

func (r *MediaItemRepository) scanMediaItemFromRows(rows *sql.Rows) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	if err := scanMediaItemFields(rows, item); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.scanMediaItemFromRows: %w", err)
	}
	return item, nil
}

var _ library.MediaItemRepository = (*MediaItemRepository)(nil)
