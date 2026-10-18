package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/domain/library"
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

// SetInterleaveBackstep records the media-health scan's interleave measure on
// the file's row, so the playback gate (MediaFile.NeedsRemux) sees a defect the
// library scan did not measure itself - and its clearing after a re-mux.
func (r *MediaItemRepository) SetInterleaveBackstep(ctx context.Context, path string, backstepMs int64) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_files SET interleave_backstep_ms = $1 WHERE file_path = $2`,
		backstepMs, path); err != nil {
		return fmt.Errorf("set media_files interleave backstep: %w", err)
	}
	return nil
}

// itemWriteQuery builds the ONE statement that persists a MediaItem and the
// sub-tables it owns (media_files, media_catalog, media_classifications,
// media_trailers). A scan writes an item per file, so the chained form -
// BEGIN, the item, a fingerprint clear, one upsert or delete per sub-table, a
// trailer delete and insert, COMMIT - cost 8 to 10 round trips each, a hundred
// thousand on a library of ten thousand files. A single statement is atomic on
// its own, so no private transaction is needed; an ambient one (TxUnit.Do) is
// joined by the caller exactly as before.
//
// Every child hangs off `u`, the CTE that returns the item's id only when the
// item row was written. An UPDATE that matches nothing therefore writes nothing
// anywhere, which is what the old early ErrNotFound return guaranteed.
//
// Sub-table semantics are unchanged: a nil or empty part DELETEs its row (nil
// means erase), a part with data upserts it. On create the deletes are skipped:
// the row was just inserted, so no child can exist yet.
//
// Two ordering rules the planner would not honour on its own, because the
// sub-statements of one WITH run in no defined order:
//   - the fingerprint clear must land before the file row takes the value (the
//     unique index would refuse it otherwise); the file insert reads the clear's
//     row count, which forces the clear to run to completion first;
//   - the trailer delete must land before the re-insert of the same ids; same
//     device.
func itemWriteQuery(item *library.MediaItem, create bool, scanTitle, sortTitle string, parentID *string) (string, []any, error) {
	var args []any
	// arg appends a parameter and returns its placeholder, cast when the
	// context (a SELECT list, unlike VALUES) gives Postgres nothing to infer from.
	arg := func(v any, cast string) string {
		args = append(args, v)
		if cast == "" {
			return fmt.Sprintf("$%d", len(args))
		}
		return fmt.Sprintf("$%d::%s", len(args), cast)
	}
	var ctes []string

	if create {
		ph := make([]string, 0, 13)
		for _, v := range []any{item.ID, item.LibraryID, parentID, item.Type, item.Title, scanTitle,
			sortTitle, item.OriginalTitle, item.Overview, item.Tagline, item.IndexNumber, item.AddedAt, item.UpdatedAt} {
			ph = append(ph, arg(v, ""))
		}
		ctes = append(ctes, `u AS (INSERT INTO media_items (id, library_id, parent_id, type, title, scan_title, sort_title, original_title,
			 overview, tagline, index_number, added_at, updated_at)
			 VALUES (`+strings.Join(ph, ",")+`) RETURNING id)`)
	} else {
		lib, par, typ, title := arg(item.LibraryID, ""), arg(parentID, ""), arg(item.Type, ""), arg(item.Title, "")
		scan, sortT, orig := arg(scanTitle, ""), arg(sortTitle, ""), arg(item.OriginalTitle, "")
		over, tag, idx, upd, id := arg(item.Overview, ""), arg(item.Tagline, ""), arg(item.IndexNumber, ""), arg(item.UpdatedAt, ""), arg(item.ID, "")
		// scan_title: see Update for why an empty value keeps the stored one.
		ctes = append(ctes, `u AS (UPDATE media_items SET library_id=`+lib+`, parent_id=`+par+`, type=`+typ+`, title=`+title+`,
			 scan_title=COALESCE(NULLIF(`+scan+`, ''), media_items.scan_title, `+title+`),
			 sort_title=`+sortT+`, original_title=`+orig+`, overview=`+over+`, tagline=`+tag+`, index_number=`+idx+`, updated_at=`+upd+`
			 WHERE id=`+id+` RETURNING id)`)
	}

	// media_files: nil File (or empty Path) means the item owns no on-disk
	// artefact (series root, season, album); the row is deleted so a JOIN
	// returns NULL for every file column.
	if f := item.File; f != nil && f.Path != "" {
		var fingerprint *string
		if f.Fingerprint != "" {
			fingerprint = &f.Fingerprint
		}
		gate := ""
		// The fingerprint is size + mtime: a HINT about whether a file changed,
		// not an identity. Two distinct files genuinely can share one (a copy
		// made with cp -p, two files restored from the same backup), and the
		// unique index turned that coincidence into an item whose every update
		// failed - twelve times in one day on prod, for ever.
		//
		// The row being written was just scanned, so it is the fresher claim on
		// the value. Whoever else holds it gives up the HINT, not the row: their
		// file is simply re-probed at the next scan.
		if fingerprint != nil {
			fp := arg(*fingerprint, "text")
			ctes = append(ctes, `fc AS (UPDATE media_files SET file_fingerprint = NULL
			 WHERE file_fingerprint = `+fp+` AND media_item_id <> (SELECT id FROM u) RETURNING 1)`)
			gate = ", (SELECT count(*) FROM fc) AS gate"
		}
		ctes = append(ctes, `fi AS (INSERT INTO media_files (media_item_id, file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, stream_probe_attempts, interleave_backstep_ms, stream_probe_version)
		 SELECT u.id, `+arg(f.Path, "text")+`, `+arg(f.Size, "bigint")+`, `+arg(fingerprint, "text")+`, `+arg(f.ModTime, "timestamptz")+`,
		   `+arg(f.Duration.Milliseconds(), "bigint")+`, `+arg(f.Bitrate, "bigint")+`, `+arg(f.ContainerFormat, "text")+`,
		   `+arg(f.StreamProbeAttempts, "integer")+`, `+arg(f.InterleaveBackstepMs, "bigint")+`, `+arg(f.StreamProbeVersion, "integer")+`
		 FROM u`+gate+` WHERE true
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   file_path = EXCLUDED.file_path,
		   file_size = EXCLUDED.file_size,
		   file_fingerprint = EXCLUDED.file_fingerprint,
		   mod_time = EXCLUDED.mod_time,
		   duration_ms = EXCLUDED.duration_ms,
		   bitrate = EXCLUDED.bitrate,
		   container_format = EXCLUDED.container_format,
		   stream_probe_attempts = EXCLUDED.stream_probe_attempts,
		   interleave_backstep_ms = EXCLUDED.interleave_backstep_ms,
		   stream_probe_version = EXCLUDED.stream_probe_version)`)
	} else if !create {
		ctes = append(ctes, `fd AS (DELETE FROM media_files WHERE media_item_id IN (SELECT id FROM u))`)
	}

	// media_catalog: absent when every field is zero/empty (the row would only
	// carry defaults and a stale FK).
	if c := item.Catalog; c != nil && (c.PremiereDate != nil || c.Year != nil ||
		len(c.ProviderIDs) > 0 || len(c.Genres) > 0 || len(c.Studios) > 0 || len(c.Tags) > 0) {
		providerJSON, err := json.Marshal(orEmptyMap(c.ProviderIDs))
		if err != nil {
			return "", nil, fmt.Errorf("marshal provider_ids: %w", err)
		}
		genresJSON, err := json.Marshal(orEmptySlice(c.Genres))
		if err != nil {
			return "", nil, fmt.Errorf("marshal genres: %w", err)
		}
		studiosJSON, err := json.Marshal(orEmptySlice(c.Studios))
		if err != nil {
			return "", nil, fmt.Errorf("marshal studios: %w", err)
		}
		tagsJSON, err := json.Marshal(orEmptySlice(c.Tags))
		if err != nil {
			return "", nil, fmt.Errorf("marshal tags: %w", err)
		}
		ctes = append(ctes, `ci AS (INSERT INTO media_catalog (media_item_id, premiere_date, year, provider_ids, genres, studios, tags)
		 SELECT u.id, `+arg(c.PremiereDate, "timestamptz")+`, `+arg(c.Year, "integer")+`, `+arg(string(providerJSON), "jsonb")+`,
		   `+arg(string(genresJSON), "jsonb")+`, `+arg(string(studiosJSON), "jsonb")+`, `+arg(string(tagsJSON), "jsonb")+`
		 FROM u WHERE true
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   premiere_date = EXCLUDED.premiere_date,
		   year = EXCLUDED.year,
		   provider_ids = EXCLUDED.provider_ids,
		   genres = EXCLUDED.genres,
		   studios = EXCLUDED.studios,
		   tags = EXCLUDED.tags)`)
	} else if !create {
		ctes = append(ctes, `cd AS (DELETE FROM media_catalog WHERE media_item_id IN (SELECT id FROM u))`)
	}

	if c := item.Classification; c != nil && (c.CommunityRating != nil || c.OfficialRating != "" ||
		len(c.CertificationsByCountry) > 0 || len(c.ProviderRatings) > 0) {
		certJSON, err := json.Marshal(orEmptyMap(c.CertificationsByCountry))
		if err != nil {
			return "", nil, fmt.Errorf("marshal certifications: %w", err)
		}
		ratingsJSON, err := json.Marshal(orEmptyFloatMap(c.ProviderRatings))
		if err != nil {
			return "", nil, fmt.Errorf("marshal provider_ratings: %w", err)
		}
		ctes = append(ctes, `cli AS (INSERT INTO media_classifications (media_item_id, community_rating, official_rating, certifications_by_country, provider_ratings)
		 SELECT u.id, `+arg(c.CommunityRating, "double precision")+`, `+arg(c.OfficialRating, "text")+`,
		   `+arg(string(certJSON), "jsonb")+`, `+arg(string(ratingsJSON), "jsonb")+`
		 FROM u WHERE true
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   community_rating = EXCLUDED.community_rating,
		   official_rating = EXCLUDED.official_rating,
		   certifications_by_country = EXCLUDED.certifications_by_country,
		   provider_ratings = EXCLUDED.provider_ratings)`)
	} else if !create {
		ctes = append(ctes, `cld AS (DELETE FROM media_classifications WHERE media_item_id IN (SELECT id FROM u))`)
	}

	// media_trailers: the in-memory slice is the source of truth, so an update
	// deletes then re-inserts (the ids are positional, "tlr-<item>-<i>").
	gate := ""
	if !create {
		ctes = append(ctes, `td AS (DELETE FROM media_trailers WHERE media_item_id IN (SELECT id FROM u) RETURNING 1)`)
		gate = ", (SELECT count(*) FROM td) AS gate"
	}
	if n := len(item.Trailers); n > 0 {
		site, key, lang, typ, subLang := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
		official, useSubs := make([]bool, n), make([]bool, n)
		for i, t := range item.Trailers {
			site[i], key[i], lang[i], typ[i], subLang[i] = t.Site, t.Key, t.Language, t.Type, t.SubtitleLanguage
			official[i], useSubs[i] = t.Official, t.UseSubtitles
		}
		ctes = append(ctes, `ti AS (INSERT INTO media_trailers (id, media_item_id, position, site, key, language, official, type, use_subtitles, subtitle_language)
		 SELECT 'tlr-' || u.id || '-' || (t.ord - 1), u.id, (t.ord - 1)::integer, t.site, t.key, t.language, t.official, t.type, t.use_subtitles, t.subtitle_language
		 FROM u`+gate+`, unnest(`+arg(pq.Array(site), "text[]")+`, `+arg(pq.Array(key), "text[]")+`, `+arg(pq.Array(lang), "text[]")+`,
		   `+arg(pq.Array(official), "boolean[]")+`, `+arg(pq.Array(typ), "text[]")+`, `+arg(pq.Array(useSubs), "boolean[]")+`,
		   `+arg(pq.Array(subLang), "text[]")+`) WITH ORDINALITY AS t(site, key, language, official, type, use_subtitles, subtitle_language, ord))`)
	}

	return "WITH " + strings.Join(ctes, ",\n") + "\nSELECT count(*) FROM u", args, nil
}

// writeMediaItem runs itemWriteQuery on the ambient transaction when there is
// one, on the pool otherwise, and reports whether the item row was written
// (false only for an UPDATE whose id matches nothing).
func writeMediaItem(ctx context.Context, exec dbExecutor, item *library.MediaItem, create bool, scanTitle, sortTitle string, parentID *string) (bool, error) {
	query, args, err := itemWriteQuery(item, create, scanTitle, sortTitle, parentID)
	if err != nil {
		return false, err
	}
	var n int
	if err := exec.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
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
