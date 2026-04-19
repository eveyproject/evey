package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lib/pq"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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

func (r *MediaItemRepository) Delete(ctx context.Context, id shared.MediaItemID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// SELECT type+parent_id then DELETE then (maybe) recompute the series
	// latest_child_added_at — all three need to observe the same DB state,
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
		return shared.ErrNotFound
	case err != nil:
		return fmt.Errorf("MediaItemRepo.Delete: read type: %w", err)
	}

	res, err := exec.ExecContext(ctx, `DELETE FROM media_items WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	if itemType == string(library.Episode) && parentID != "" {
		if rcErr := r.recomputeSeriesLatestChildAddedAtFromSeason(ctx, parentID); rcErr != nil {
			return fmt.Errorf("MediaItemRepo.Delete: %w", rcErr)
		}
	}
	return nil
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

var _ library.MediaItemRepository = (*MediaItemRepository)(nil)
