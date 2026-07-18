package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
)

// RefreshFileState re-stamps ONLY the change-detection columns (file_size,
// mod_time, and the size-mtime-derived file_fingerprint) on the media_files row
// at path, leaving every probed field - streams, keyframes_ms, duration_ms,
// container_format - untouched. It exists for the reindex path: rewriting a
// container in place (to add a Cues seek index) gives the file new bytes but
// identical logical content, so the next library scan must treat it as
// UNCHANGED (matching size+mtime) rather than re-probe it and DELETE+re-INSERT
// its streams - which drops the keyframe index and forces a transcode window
// (copy rung needs keyframes_ms) until the backfill worker catches up. The
// fingerprint format mirrors the scanner (filesystem/scanner.go: "<size>-<mtimeNs>")
// so change-detection agrees. No-op when no row matches the path.
func (r *MediaItemRepository) RefreshFileState(ctx context.Context, path string, size int64, modTime time.Time) error {
	// Bound the UPDATE like every sibling repo method: this runs from a
	// background job on a context.Background()-derived ctx with no deadline of
	// its own, and a stalled UPDATE would block the goroutine forever - wedging
	// the seek-index job state machine (running stays true, every later job is
	// refused with ErrJobRunning).
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	fingerprint := fmt.Sprintf("%d-%d", size, modTime.UnixNano())
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_files SET file_size = $1, mod_time = $2, file_fingerprint = $3 WHERE file_path = $4`,
		size, modTime, fingerprint, path); err != nil {
		return fmt.Errorf("refresh media_files state: %w", err)
	}
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
		`INSERT INTO media_files (media_item_id, file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, stream_probe_attempts)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   file_path = EXCLUDED.file_path,
		   file_size = EXCLUDED.file_size,
		   file_fingerprint = EXCLUDED.file_fingerprint,
		   mod_time = EXCLUDED.mod_time,
		   duration_ms = EXCLUDED.duration_ms,
		   bitrate = EXCLUDED.bitrate,
		   container_format = EXCLUDED.container_format,
		   stream_probe_attempts = EXCLUDED.stream_probe_attempts`,
		string(item.ID), f.Path, f.Size, fingerprint,
		f.ModTime, f.Duration.Milliseconds(), f.Bitrate, f.ContainerFormat, f.StreamProbeAttempts,
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

// orEmptyMap / orEmptySlice / orEmptyFloatMap normalise nil → empty
// container so JSON marshal produces `{}` / `[]` instead of `null`
// for catalog/classification columns that are NOT NULL in the schema.
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
