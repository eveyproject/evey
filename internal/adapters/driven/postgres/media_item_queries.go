package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// listByLibraryIDMaxRows caps the non-paginated library-wide list so an
// accidental caller can't slurp a whole library.
const listByLibraryIDMaxRows = 10000

func (r *MediaItemRepository) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.GetByID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mi.id = $1", mediaItemColumns, mediaItemFromJoins), string(id)))
}

func (r *MediaItemRepository) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.GetByFilePath")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mf.file_path = $1", mediaItemColumns, mediaItemFromJoins), path))
}

func (r *MediaItemRepository) GetByFileFingerprint(ctx context.Context, fingerprint string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.GetByFileFingerprint")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s %s WHERE mf.file_fingerprint = $1", mediaItemColumns, mediaItemFromJoins), fingerprint))
}

func (r *MediaItemRepository) GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.GetByLibraryTypeTitle")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(txFromCtx(ctx, r.db).QueryRowContext(ctx,
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
	if len(f.Genres) > 0 {
		// OR-match: the item is kept if ANY of its genres matches ANY requested
		// one - but compared LOWERCASED, and after expanding each request into
		// its known spellings.
		//
		// Both parts are required by the data. Genres are stored as the provider
		// wrote them, so one library holds "Drama" and "Drame" for the same
		// thing, plus IMDb's "Sci-Fi" beside TMDB's "Science-Fiction", in
		// whatever case each used. The old `?|` was an exact, case-sensitive
		// match, so picking a genre in the panel silently hid every title tagged
		// in the other language - and ListGenres now folds those duplicates
		// away, which would have made that worse by offering only one of the two
		// labels. Expansion and folding must ship together.
		//
		// The lateral subquery costs a scan of one small jsonb array per row,
		// which is what the previous operator did anyway.
		conditions = append(conditions, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM jsonb_array_elements_text(mc.genres) AS eg
			          WHERE lower(eg) = ANY($%d::text[]))`, argIdx))
		args = append(args, pq.Array(library.ExpandGenreAliases(f.Genres)))
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
	if f.MissingProviderIDs != nil {
		if *f.MissingProviderIDs {
			conditions = append(conditions, "(mc.provider_ids IS NULL OR mc.provider_ids = '{}'::jsonb)")
		} else {
			conditions = append(conditions, "mc.provider_ids IS NOT NULL AND mc.provider_ids != '{}'::jsonb")
		}
	}
	if f.AfterID != "" {
		conditions = append(conditions, fmt.Sprintf("mi.id > $%d", argIdx))
		args = append(args, f.AfterID)
		// argIdx intentionally not bumped: AfterID is the last placeholder
		// the filter produces. Keep this branch last so later additions
		// get a free slot - if you add a new condition below, re-add the
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

// mediaItemCursorSeek builds the keyset predicate that resumes after the
// cursor's row, and returns it with the args it bound.
//
// The plain `(sort, mi.id) > ($v, $id)` tuple comparison is only correct while
// the sort column cannot be NULL. Every sortable column except the mi.* ones
// reaches us through a LEFT JOIN (media_catalog, media_classifications), so it
// IS null whenever the provider has not matched the item - a fresh scan, a home
// video, anything TMDB cannot identify. Two things broke there, both found
// 2026-08-08 and both reproduced against a real Postgres:
//
//   - a row comparison involving NULL evaluates to NULL, not true, so those
//     items were silently DROPPED from every page after the first. Ascending,
//     the whole tail of the library simply vanished mid-scroll;
//   - the handler, having no value to encode for a NULL row, put an EMPTY
//     STRING in the cursor. Binding "" against a numeric column is `invalid
//     input syntax`, i.e. a 500 on ordinary data.
//
// So NULL is treated as a position in the ordering. That position is now LAST
// in BOTH directions, which is what mediaItemOrderBy pins with NULLS LAST and
// what this function must mirror exactly - a seek that disagrees with the
// ORDER BY skips rows or repeats a page forever.
//
// Last in both directions is also the only answer that is not absurd to a
// viewer: sorting a library by rating, descending, used to open on the eight
// films nobody has rated before the 8.7s (prod, 2026-09-05 - "trier par note ça
// marche pas"). An unrated item is not the best-rated item.
//
// An empty SortValue means "the previous row had no value" and is never bound,
// which is also what keeps the cast from happening. It stays unambiguous
// because every nullable sort key encodes non-empty for a present value
// (zero-padded number, RFC3339 date).
func mediaItemCursorSeek(sortField, sortOrder string, cursor shared.Cursor, args []any) (string, []any) {
	// mi.* columns come from the base table and are never NULL here; everything
	// else arrives through a LEFT JOIN and can be.
	nullable := !strings.HasPrefix(sortField, "mi.") && sortField != effectiveAddedAt
	atNullRow := nullable && cursor.SortValue == ""

	cmp := ">"
	if sortOrder == "DESC" {
		cmp = "<"
	}

	// Inside the trailing NULL block: only the rest of that block remains,
	// ordered by id in the same direction as the ORDER BY.
	if atNullRow {
		idIdx := len(args) + 1
		args = append(args, cursor.LastID)
		return fmt.Sprintf("(%s IS NULL AND mi.id %s $%d)", sortField, cmp, idIdx), args
	}

	vIdx := len(args) + 1
	args = append(args, cursor.SortValue, cursor.LastID)
	if !nullable {
		return fmt.Sprintf("((%s, mi.id) %s ($%d, $%d))", sortField, cmp, vIdx, vIdx+1), args
	}
	// The NULL block is still ahead (it is always last), so it has to be part of
	// what remains. Parenthesised as one term: AND binds tighter than OR, and
	// getting that wrong is how an earlier keyset query served the same page
	// forever.
	return fmt.Sprintf("((%s, mi.id) %s ($%d, $%d) OR %s IS NULL)", sortField, cmp, vIdx, vIdx+1, sortField), args
}

// mediaItemOrderBy renders the ordering shared by the cursor and non-cursor
// paths. NULLS LAST in both directions: Postgres would otherwise open a
// descending sort on the unrated items, and mediaItemCursorSeek is written
// against this exact placement.
func mediaItemOrderBy(sortField, sortOrder string) string {
	return fmt.Sprintf("%s %s NULLS LAST, mi.id %s", sortField, sortOrder, sortOrder)
}

func (r *MediaItemRepository) Count(ctx context.Context, f library.ListMediaItemsFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.Count")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.buildWhereClause(f)
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `SELECT COUNT(*) `+mediaItemFromJoins+where, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("MediaItemRepo.Count: %w", err)
	}
	return count, nil
}

func (r *MediaItemRepository) List(ctx context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.buildWhereClause(f)

	sortKey, sortField := resolveSortKey(f)
	sortOrder := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortOrder = "DESC"
	}

	// Cursor takes precedence over Offset. Reject cursors generated for a
	// different sort key - re-using them would produce undefined ordering.
	if f.Cursor.LastID != "" {
		if f.Cursor.SortField != sortKey {
			return nil, shared.ErrCursorMismatch
		}
		var seek string
		seek, args = mediaItemCursorSeek(sortField, sortOrder, f.Cursor, args)
		where = whereAnd(where, seek)
	}

	query := fmt.Sprintf("SELECT %s %s%s", mediaItemColumns, mediaItemFromJoins, where)

	switch {
	case f.Cursor.LastID != "":
		query += " ORDER BY " + mediaItemOrderBy(sortField, sortOrder)
	case f.AfterID != "" || f.KeysetByID:
		query += " ORDER BY mi.id ASC"
	default:
		// Same ordering as the cursor branch, or the first page an offset serves
		// would not agree with the pages a cursor serves after it.
		query += " ORDER BY " + mediaItemOrderBy(sortField, sortOrder)
	}

	limit := f.Limit
	if limit <= 0 || limit > DefaultListLimit {
		limit = DefaultListLimit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Cursor.LastID == "" && f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
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

func (r *MediaItemRepository) ListFilePaths(ctx context.Context, libraryID shared.LibraryID) (map[string]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.ListFilePaths")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
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

func (r *MediaItemRepository) ListByLibraryID(ctx context.Context, libraryID shared.LibraryID) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.ListByLibraryID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
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
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.ListScanSummaries")
	defer span.End()
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
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.id,
		        COALESCE(mf.file_path, ''),
		        COALESCE(mf.file_fingerprint, ''),
		        COALESCE(mf.file_size, 0),
		        COALESCE(mf.mod_time, '1970-01-01 00:00:00+00'::timestamptz),
		        COALESCE(mf.stream_probe_attempts, 0)
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
		if err := rows.Scan(&row.ID, &row.FilePath, &row.FileFingerprint, &row.FileSize, &row.ModTime, &row.StreamProbeAttempts); err != nil {
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
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.SeriesAggregate")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var agg library.SeriesAggregate
	var totalMs int64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
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
		string(seriesID)).Scan(&totalMs, &agg.EpisodeCount, &agg.SeasonCount)
	if err != nil {
		return library.SeriesAggregate{}, fmt.Errorf("MediaItemRepo.SeriesAggregate: %w", err)
	}
	dur, derr := shared.NewDurationMs(totalMs)
	if derr != nil {
		return library.SeriesAggregate{}, fmt.Errorf("MediaItemRepo.SeriesAggregate: build duration: %w", derr)
	}
	agg.TotalDuration = dur
	return agg, nil
}

func (r *MediaItemRepository) ListContainerInfo(ctx context.Context, libraryID shared.LibraryID) ([]library.MediaItemContainerRow, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.ListContainerInfo")
	defer span.End()
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
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
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
