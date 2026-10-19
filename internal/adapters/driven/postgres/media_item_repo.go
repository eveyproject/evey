package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Split layout:
//   media_item_repo.go       - type, constructor, tx helper, Create/Update/Delete/DeleteByIDs + series helpers
//   media_item_upserts.go    - upsert helpers for media_files / media_catalog / media_classifications + trailers
//   media_item_queries.go    - GetBy*, Count, List, ListBy*, SeriesAggregate, ListContainerInfo + where/sort helpers
//   media_item_row_scan.go   - rowScanner interface + scanMediaItemFields + scan method variants

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
	 mi.added_at, mi.updated_at, mi.latest_child_added_at,
	 mf.file_path, mf.file_size, mf.file_fingerprint, mf.mod_time, mf.duration_ms, mf.bitrate, mf.container_format, mf.stream_probe_attempts, mf.interleave_backstep_ms, mf.stream_probe_version,
	 mc.premiere_date, mc.year, mc.provider_ids, mc.genres, mc.studios, mc.tags,
	 mcl.community_rating, mcl.official_rating, mcl.certifications_by_country, mcl.provider_ratings,
	 scl.official_rating, scl.certifications_by_country, sct.tags,
	 COALESCE((SELECT jsonb_agg(jsonb_build_object('Site', mt.site, 'Key', mt.key,
	     'Language', mt.language, 'Official', mt.official, 'Type', mt.type,
	     'UseSubtitles', mt.use_subtitles, 'SubtitleLanguage', mt.subtitle_language) ORDER BY mt.position)
	     FROM media_trailers mt WHERE mt.media_item_id = mi.id), '[]'::jsonb) AS trailers`

// mediaItemFromJoins also joins, for a season or an episode, its series'
// classification (scl) and catalog (sct): the parental rule reads them
// (MediaItem.SeriesParental). mip is the item's parent, the series itself for
// a season, the season for an episode. Each join is on a primary key, one row
// at most, so no query built on this counts an item twice.
const mediaItemFromJoins = `FROM media_items mi
	 LEFT JOIN media_files mf ON mf.media_item_id = mi.id
	 LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
	 LEFT JOIN media_classifications mcl ON mcl.media_item_id = mi.id
	 LEFT JOIN media_items mip ON mip.id = mi.parent_id AND mi.type IN ('season', 'episode')
	 LEFT JOIN media_classifications scl ON scl.media_item_id = CASE mip.type WHEN 'series' THEN mip.id WHEN 'season' THEN mip.parent_id END
	 LEFT JOIN media_catalog sct ON sct.media_item_id = CASE mip.type WHEN 'series' THEN mip.id WHEN 'season' THEN mip.parent_id END`

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(db *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: db}
}

// sanitizeMediaItemText strips bytes that aren't valid UTF-8 from the item's
// text fields. Postgres TEXT columns reject invalid UTF-8 - e.g. a Latin-1
// filename byte (0x80) parsed into the title made the insert fail on EVERY
// scan, so the item never indexed and the log filled with the same error
// ("invalid byte sequence for encoding UTF8"). Stripping is safe: valid UTF-8
// is returned unchanged, and a mangled scan title is overwritten by the
// provider title at enrichment. A mis-encoded file path will be stored
// stripped (so the row inserts); such a file needs renaming to play.
func sanitizeMediaItemText(item *library.MediaItem) {
	v := func(s string) string { return strings.ToValidUTF8(s, "") }
	item.Title = v(item.Title)
	item.ScanTitle = v(item.ScanTitle)
	item.SortTitle = v(item.SortTitle)
	item.OriginalTitle = v(item.OriginalTitle)
	item.Overview = v(item.Overview)
	item.Tagline = v(item.Tagline)
	if item.File != nil {
		item.File.Path = v(item.File.Path)
		item.File.ContainerFormat = v(item.File.ContainerFormat)
		item.File.Fingerprint = v(item.File.Fingerprint)
	}
	if item.Catalog != nil {
		for i := range item.Catalog.Genres {
			item.Catalog.Genres[i] = v(item.Catalog.Genres[i])
		}
		for i := range item.Catalog.Studios {
			item.Catalog.Studios[i] = v(item.Catalog.Studios[i])
		}
		for i := range item.Catalog.Tags {
			item.Catalog.Tags[i] = v(item.Catalog.Tags[i])
		}
	}
}

func (r *MediaItemRepository) Create(ctx context.Context, item *library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.Create")
	defer span.End()
	sanitizeMediaItemText(item)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	scanTitle := item.ScanTitle
	if scanTitle == "" {
		scanTitle = item.Title
	}
	// sort_title drives alphabetical browsing and the keyset cursor.
	// Derive from Title whenever the caller didn't provide one - most
	// scan/enrich call sites still leave it empty, which used to make
	// the whole library sort by UUID order and broke pagination.
	sortTitle := item.SortTitle
	if sortTitle == "" {
		sortTitle = library.NormalizeSortTitle(item.Title)
	}
	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}

	// One statement, atomic on its own; an ambient transaction is joined.
	_, err := writeMediaItem(ctx, txFromCtx(ctx, r.db), item, true, scanTitle, sortTitle, parentID)
	if err != nil {
		err = fmt.Errorf("insert media_items: %w", err)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaItemRepo.Create: %w", err)
	}
	r.bumpSeriesLatestChildAddedAt(ctx, item)
	r.clearSeriesWatchedOnNewEpisode(ctx, item)
	return nil
}

func (r *MediaItemRepository) Update(ctx context.Context, item *library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.Update")
	defer span.End()
	sanitizeMediaItemText(item)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	// Refresh sort_title from Title when the caller didn't override it.
	// Enrichment renames titles (TMDB FR translation, etc.), so the
	// sort key must follow - otherwise we lock in the original filename
	// derived title forever.
	sortTitle := item.SortTitle
	if sortTitle == "" {
		sortTitle = library.NormalizeSortTitle(item.Title)
	}
	// scan_title was historically treated as immutable post-create (the
	// "stable filesystem-derived" comment in mediaItemColumns). That
	// breaks the orphan title-heal pass: when the parser improves,
	// healOrphanTitles rewrites Title+ScanTitle in memory, calls Update,
	// and previously only Title landed in the DB row. ResearchMetadata
	// (which resets Title := ScanTitle from the stale DB value) then
	// silently undid the heal on the next call. So a NON-EMPTY value from
	// the caller still wins.
	//
	// An EMPTY one no longer means "overwrite it with the display title".
	// It used to, and that substitution destroyed the one value that can
	// repair a mis-enriched row without going back to the disk: a caller
	// holding a partially-built item wrote the (possibly wrong) provider
	// title into scan_title. The COALESCE below keeps the stored value in
	// that case, and only falls back to the title when the column is NULL
	// (a row created before scan_title existed).
	scanTitle := item.ScanTitle

	// One statement, atomic on its own; an ambient transaction is joined.
	written, err := writeMediaItem(ctx, txFromCtx(ctx, r.db), item, false, scanTitle, sortTitle, parentID)
	if err != nil {
		err = fmt.Errorf("update media_items: %w", err)
	} else if !written {
		err = shared.ErrNotFound
	}
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaItemRepo.Update: %w", err)
	}
	r.bumpSeriesLatestChildAddedAt(ctx, item)
	return nil
}

func (r *MediaItemRepository) Delete(ctx context.Context, id shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// SELECT type+parent_id then DELETE then (maybe) recompute the series
	// latest_child_added_at - all three need to observe the same DB state,
	// so bind them to an ambient Tx when one exists (or fall back to the
	// shared conn). The prior non-tx flow ignored the SELECT Scan error
	// (masking transient read failures) and the recompute error, so a
	// failed recompute would silently leave the series' sort key stale.
	exec := txFromCtx(ctx, r.db)

	var itemType, parentID string
	switch err := exec.QueryRowContext(ctx,
		`SELECT type, COALESCE(parent_id, '') FROM media_items WHERE id = $1`,
		string(id),
	).Scan(&itemType, &parentID); {
	case errors.Is(err, sql.ErrNoRows):
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return shared.ErrNotFound
	case err != nil:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaItemRepo.Delete: read type: %w", err)
	}

	res, err := exec.ExecContext(ctx, `DELETE FROM media_items WHERE id = $1`, string(id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaItemRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	if itemType == string(library.Episode) && parentID != "" {
		if rcErr := r.recomputeSeriesLatestChildAddedAtFromSeason(ctx, parentID); rcErr != nil {
			span.RecordError(rcErr)
			span.SetStatus(codes.Error, rcErr.Error())
			return fmt.Errorf("MediaItemRepo.Delete: %w", rcErr)
		}
	}
	return nil
}

func (r *MediaItemRepository) DeleteByIDs(ctx context.Context, ids []string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.DeleteByIDs")
	defer span.End()
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_items WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaItemRepo.DeleteByIDs: %w", err)
	}
	return nil
}

// bumpSeriesLatestChildAddedAt keeps media_items.latest_child_added_at in
// sync on the grandparent series when an episode is created or touched.
// Best-effort - failure is logged but not propagated.
//
// IMPORTANT: only AddedAt is propagated. Folding in UpdatedAt would let any
// metadata refresh (cover art, retitle, etc.) move the series to the top of
// "Recently Added" - a full re-scan in the morning would shuffle the entire
// library to today, masking actual new content.
func (r *MediaItemRepository) bumpSeriesLatestChildAddedAt(ctx context.Context, item *library.MediaItem) {
	if item == nil || item.Type != library.Episode || item.ParentID == "" {
		return
	}
	when := item.AddedAt
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
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

// clearSeriesWatchedOnNewEpisode resets the grandparent series' "fully watched"
// flag (watch_progress.completed) for EVERY profile when a brand-new episode is
// created. A series with a new, unwatched episode is no longer fully watched, so
// it must re-surface in watched-filtered views - notably the home library
// "Séries" slider, which hides any series whose watch_progress.completed is true.
// The "Recently Added" feed already recomputes this live; the slider relies on
// the stored flag, so it goes stale without this reset. Episode → season →
// series, mirroring bumpSeriesLatestChildAddedAt. Called only on Create (a
// genuinely new episode), never on re-scan Update. Best-effort.
func (r *MediaItemRepository) clearSeriesWatchedOnNewEpisode(ctx context.Context, item *library.MediaItem) {
	if item == nil || item.Type != library.Episode || item.ParentID == "" {
		return
	}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		UPDATE watch_progress
		SET completed = FALSE
		WHERE completed = TRUE
		  AND media_item_id = (SELECT parent_id FROM media_items WHERE id = $1 AND type = 'season')
	`, item.ParentID)
	if err != nil {
		slog.Warn("clear series watched on new episode", slog.String("episode_id", string(item.ID)), slog.Any("err", err))
	}
}

func (r *MediaItemRepository) recomputeSeriesLatestChildAddedAtFromSeason(ctx context.Context, seasonID string) error {
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
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
		return fmt.Errorf("recompute series latest_child_added_at: %w", err)
	}
	return nil
}

// BackfillEmptySortTitles fills the sort_title column for every row where
// it is currently empty, computing the value from the existing title via
// library.NormalizeSortTitle. Returns the number of rows touched.
//
// Existed because pre-fix scans inserted media_items with sort_title=” -
// the alphabetical sort and the keyset cursor both rely on this column,
// so an empty value silently broke pagination on production libraries.
// Idempotent: once a row has a non-empty sort_title, this is a no-op.
func (r *MediaItemRepository) BackfillEmptySortTitles(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.BackfillEmptySortTitles")
	defer span.End()

	const batchSize = 500
	var totalUpdated int

	type pair struct{ id, sortTitle string }
	// One batch per call so rows can be closed by defer (sqlclosecheck) while
	// the outer loop keeps issuing fresh queries until the table is drained.
	readBatch := func() ([]pair, error) {
		// Per BATCH, not once around the loop: this runs from a boot goroutine
		// on the process-lifetime context, and it loops until the table is
		// drained. A single bound would either expire mid-backfill on a big
		// library or, taken from the process context, never expire at all -
		// parking a pool connection for as long as the server lives.
		batchCtx, cancel := withTimeout(ctx, defaultQueryTimeout)
		defer cancel()
		rows, err := txFromCtx(batchCtx, r.db).QueryContext(batchCtx,
			`SELECT id, title FROM media_items WHERE sort_title = '' AND title <> '' LIMIT $1`,
			batchSize,
		)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.BackfillEmptySortTitles select: %w", err)
		}
		defer rows.Close()
		var batch []pair
		for rows.Next() {
			var id, title string
			if err := rows.Scan(&id, &title); err != nil {
				return nil, fmt.Errorf("MediaItemRepo.BackfillEmptySortTitles scan: %w", err)
			}
			batch = append(batch, pair{id: id, sortTitle: library.NormalizeSortTitle(title)})
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.BackfillEmptySortTitles rows: %w", err)
		}
		return batch, nil
	}

	for {
		batch, err := readBatch()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return totalUpdated, err
		}
		if len(batch) == 0 {
			break
		}

		// Single bulk UPDATE per batch via UNNEST + FROM. Cheaper than
		// N round-trips and stays within one transaction for atomicity
		// of the batch.
		ids := make([]string, len(batch))
		sorts := make([]string, len(batch))
		for i, p := range batch {
			ids[i] = p.id
			sorts[i] = p.sortTitle
		}
		writeCtx, cancelWrite := withTimeout(ctx, defaultQueryTimeout)
		res, err := txFromCtx(writeCtx, r.db).ExecContext(writeCtx,
			`UPDATE media_items SET sort_title = data.sort_title
			 FROM (SELECT UNNEST($1::text[]) AS id, UNNEST($2::text[]) AS sort_title) AS data
			 WHERE media_items.id = data.id AND media_items.sort_title = ''`,
			pq.Array(ids), pq.Array(sorts),
		)
		cancelWrite()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return totalUpdated, fmt.Errorf("MediaItemRepo.BackfillEmptySortTitles update: %w", err)
		}
		n, _ := res.RowsAffected()
		totalUpdated += int(n)
		if len(batch) < batchSize {
			break
		}
	}
	return totalUpdated, nil
}

var _ library.MediaItemCommandRepo = (*MediaItemRepository)(nil)
