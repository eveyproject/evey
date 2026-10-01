package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/feed"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type FeedQuerier struct {
	db *sql.DB
}

func NewFeedQuerier(db *sql.DB) *FeedQuerier {
	return &FeedQuerier{db: db}
}

const maxFeedLimit = 200

func clampFeedLimit(limit int) int {
	if limit <= 0 || limit > maxFeedLimit {
		return 50
	}
	return limit
}

// ContinueWatching returns in-progress films and the next episode to watch per series.
func (q *FeedQuerier) ContinueWatching(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed.ContinueWatching")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	limit = clampFeedLimit(limit)
	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, `
		WITH in_progress_movies AS (
			SELECT mi.id, ''::text AS parent_id, mi.title, mi.type,
			       wp.position_ms, ''::text AS added_at, wp.last_played_at,
			       NULL::int AS season_number, NULL::int AS episode_number
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			LEFT JOIN media_files mf ON mf.media_item_id = mi.id
			WHERE wp.profile_id = $1
			  AND mi.type = 'movie'
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
			  -- MinResumePct=5 (the classic resume floor): don't surface a film the user
			  -- only sampled for a few seconds. Items with unknown duration
			  -- bypass the gate so we don't silently drop legitimate progress.
			  AND (mf.duration_ms IS NULL OR mf.duration_ms = 0
			       OR wp.position_ms * 100 >= mf.duration_ms * 5)
		),
		series_list AS (
			SELECT DISTINCT mi.parent_id AS season_id, s.parent_id AS series_id
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
		),
		-- One card per series: its most recently played in-progress episode.
		in_progress_eps AS (
			SELECT DISTINCT ON (s.parent_id)
			       mi.id, mi.parent_id, mi.title, 'episode'::text AS type,
			       wp.position_ms, ''::text AS added_at, wp.last_played_at,
			       s.index_number AS season_number, mi.index_number AS episode_number,
			       s.parent_id AS series_id
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s ON s.id = mi.parent_id
			LEFT JOIN media_files mf ON mf.media_item_id = mi.id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
			  AND (mf.duration_ms IS NULL OR mf.duration_ms = 0
			       OR wp.position_ms * 100 >= mf.duration_ms * 5)
			ORDER BY s.parent_id, wp.last_played_at DESC, mi.id
		),
		next_unwatched AS (
			SELECT DISTINCT ON (s.parent_id)
			       mi.id, mi.parent_id, mi.title, 'episode'::text AS type,
			       0::bigint AS position_ms, ''::text AS added_at, NOW() AS last_played_at,
			       s.index_number AS season_number, mi.index_number AS episode_number
			FROM media_items mi
			INNER JOIN media_items s ON s.id = mi.parent_id
			INNER JOIN series_list sl ON sl.series_id = s.parent_id
			WHERE mi.type = 'episode'
			  AND NOT EXISTS (SELECT 1 FROM watch_progress wp2 WHERE wp2.media_item_id = mi.id AND wp2.profile_id = $1 AND wp2.completed = TRUE)
			  -- A series already resuming an episode gets no second, next-up card.
			  AND NOT EXISTS (SELECT 1 FROM in_progress_eps ip WHERE ip.series_id = s.parent_id)
			ORDER BY s.parent_id, s.index_number, mi.index_number
		)
		SELECT id, parent_id, title, type, position_ms, added_at, season_number, episode_number
		FROM (
			SELECT id, parent_id, title, type, position_ms, added_at, last_played_at, season_number, episode_number FROM in_progress_movies
			UNION ALL
			SELECT id, parent_id, title, type, position_ms, added_at, last_played_at, season_number, episode_number FROM in_progress_eps
			UNION ALL
			SELECT id, parent_id, title, type, position_ms, added_at, last_played_at, season_number, episode_number FROM next_unwatched
		) combined
		-- id tiebreak: rows can share last_played_at (ContinueWatching's next-up
		-- rows all carry NOW(); resume rows can coincide), so without it the order,
		-- and which rows survive the LIMIT, would be undefined and shift per refresh.
		ORDER BY last_played_at DESC, id
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanFeedItems(rows)
	if err != nil {
		return nil, err
	}
	q.fillPrimaryBlurhashes(ctx, items)
	return items, nil
}

// ResumePlayback returns in-progress media AND next episodes per series, sorted by last user interaction.
func (q *FeedQuerier) ResumePlayback(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed.ResumePlayback")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	limit = clampFeedLimit(limit)
	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, `
		WITH in_progress_all AS (
			SELECT mi.id, COALESCE(mi.parent_id,'') AS parent_id, mi.title, mi.type,
			       wp.position_ms, wp.last_played_at,
			       sea.index_number AS season_number, mi.index_number AS episode_number,
			       sea.parent_id AS series_id,
			       -- One card per series (its latest in-progress episode); a film is its own group.
			       ROW_NUMBER() OVER (PARTITION BY COALESCE(sea.parent_id, mi.id)
			                          ORDER BY wp.last_played_at DESC, mi.id) AS rn
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			LEFT JOIN media_items sea ON mi.type = 'episode' AND sea.id = mi.parent_id
			LEFT JOIN media_files mf ON mf.media_item_id = mi.id
			WHERE wp.profile_id = $1
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
			  -- MinResumePct=5 (the classic resume floor): don't surface zaps.
			  AND (mf.duration_ms IS NULL OR mf.duration_ms = 0
			       OR wp.position_ms * 100 >= mf.duration_ms * 5)
		),
		in_progress AS (
			SELECT * FROM in_progress_all WHERE rn = 1
		),
		watched_series AS (
			SELECT DISTINCT s.parent_id AS series_id,
			       MAX(wp.last_played_at) AS last_played_at
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s  ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
			  AND wp.completed = TRUE
			GROUP BY s.parent_id
		),
		next_ep AS (
			SELECT DISTINCT ON (ws.series_id)
			       mi.id, COALESCE(mi.parent_id,'') AS parent_id, mi.title, mi.type,
			       0::bigint AS position_ms, ws.last_played_at,
			       sea.index_number AS season_number, mi.index_number AS episode_number
			FROM watched_series ws
			INNER JOIN media_items sea ON sea.parent_id = ws.series_id AND sea.type = 'season'
			INNER JOIN media_items mi  ON mi.parent_id = sea.id       AND mi.type  = 'episode'
			WHERE NOT EXISTS (SELECT 1 FROM watch_progress wp2 WHERE wp2.media_item_id = mi.id AND wp2.profile_id = $1 AND wp2.completed = TRUE)
			-- A series already resuming an episode gets no second, next-up card.
			AND NOT EXISTS (SELECT 1 FROM in_progress ip WHERE ip.series_id = ws.series_id)
			ORDER BY ws.series_id, sea.index_number, mi.index_number
		)
		SELECT id, parent_id, title, type, position_ms, '', season_number, episode_number
		FROM (
			SELECT id, parent_id, title, type, position_ms, last_played_at, season_number, episode_number FROM in_progress
			UNION ALL
			SELECT id, parent_id, title, type, position_ms, last_played_at, season_number, episode_number FROM next_ep
		) combined
		-- id tiebreak: rows can share last_played_at (ContinueWatching's next-up
		-- rows all carry NOW(); resume rows can coincide), so without it the order,
		-- and which rows survive the LIMIT, would be undefined and shift per refresh.
		ORDER BY last_played_at DESC, id
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanFeedItems(rows)
	if err != nil {
		return nil, err
	}
	q.fillPrimaryBlurhashes(ctx, items)
	return items, nil
}

// NextEpisode returns the next unwatched episode per series where the user completed at least one episode.
func (q *FeedQuerier) NextEpisode(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed.NextEpisode")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	limit = clampFeedLimit(limit)
	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, `
		WITH watched_series AS (
			SELECT DISTINCT s.parent_id AS series_id
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s  ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
			  AND wp.completed = TRUE
		),
		next_ep AS (
			SELECT DISTINCT ON (ws.series_id)
			       mi.id, mi.parent_id, mi.title, mi.type,
			       0::bigint AS position_ms, '' AS added_at,
			       sea.index_number AS season_number, mi.index_number AS episode_number
			FROM watched_series ws
			INNER JOIN media_items sea ON sea.parent_id = ws.series_id AND sea.type = 'season'
			INNER JOIN media_items mi  ON mi.parent_id = sea.id       AND mi.type  = 'episode'
			WHERE NOT EXISTS (SELECT 1 FROM watch_progress wp2 WHERE wp2.media_item_id = mi.id AND wp2.profile_id = $1 AND wp2.completed = TRUE)
			ORDER BY ws.series_id, sea.index_number, mi.index_number
		)
		SELECT id, parent_id, title, type, position_ms, added_at, season_number, episode_number
		FROM next_ep
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanFeedItems(rows)
	if err != nil {
		return nil, err
	}
	q.fillPrimaryBlurhashes(ctx, items)
	return items, nil
}

// RecentlyAdded returns media items added within daysWindow days, most recent
// first. When profileID is non-empty, items the profile has fully watched are
// filtered out (movies via wp.completed; series when every episode has
// wp.completed). This matches the homepage UX: clicking the watched badge
// makes the item leave the home immediately. Pass an empty profileID to
// disable filtering (admin tooling, audits, etc.).
func (q *FeedQuerier) RecentlyAdded(ctx context.Context, profileID string, daysWindow, limit int) ([]*feed.FeedItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed.RecentlyAdded")
	defer span.End()
	limit = clampFeedLimit(limit)
	if daysWindow <= 0 {
		daysWindow = 30
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, `
		SELECT sub.id, COALESCE(sub.parent_id,''), sub.title, sub.type, 0, sub.effective_added::text, NULL::int, NULL::int
		FROM (
			SELECT m.id, m.parent_id, m.title, m.type,
				CASE WHEN m.type = 'series'
					THEN COALESCE(ep_max.max_added, m.added_at)
				ELSE m.added_at END AS effective_added
			FROM media_items m
			LEFT JOIN (
				SELECT s.parent_id AS series_id, MAX(e.added_at) AS max_added
				FROM media_items e
				INNER JOIN media_items s ON s.id = e.parent_id
				WHERE e.type = 'episode'
				  AND e.added_at > NOW() - ($1 || ' days')::interval
				GROUP BY s.parent_id
			) ep_max ON m.type = 'series' AND ep_max.series_id = m.id
			WHERE m.type IN ('movie', 'series', 'album')
		) sub
		LEFT JOIN LATERAL (
			SELECT CASE
				WHEN $3 = '' THEN FALSE
				WHEN sub.type = 'movie' THEN
					COALESCE((SELECT completed FROM watch_progress wp WHERE wp.media_item_id = sub.id AND wp.profile_id = $3 LIMIT 1), FALSE)
				WHEN sub.type = 'series' THEN
					-- A series is "fully watched" only when EVERY current episode
					-- has a completed wp row. A newly-added episode without any
					-- wp row makes the series re-surface on the home page.
					NOT EXISTS (
						SELECT 1
						FROM media_items se
						JOIN media_items ep ON ep.parent_id = se.id
						WHERE se.parent_id = sub.id
						  AND ep.type = 'episode'
						  AND NOT EXISTS (
							SELECT 1 FROM watch_progress wp
							WHERE wp.media_item_id = ep.id
							  AND wp.profile_id = $3
							  AND wp.completed = TRUE
						  )
					)
				ELSE FALSE END AS completed
		) w ON TRUE
		WHERE sub.effective_added > NOW() - ($1 || ' days')::interval
		  AND w.completed = FALSE
		ORDER BY sub.effective_added DESC
		LIMIT $2
	`, daysWindow, limit, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanFeedItems(rows)
	if err != nil {
		return nil, err
	}
	q.fillPrimaryBlurhashes(ctx, items)
	return items, nil
}

// Recommended returns items matching the given genres, scored by match count.
// Items the profile has fully watched are filtered out so clicking the
// watched badge on a card removes it from the home immediately. (Earlier
// behaviour kept watched items at the bottom of the list - the explicit
// click is now treated as a hide-from-home signal.)
func (q *FeedQuerier) Recommended(ctx context.Context, profileID string, genres []string, limit int) ([]*feed.FeedItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed.Recommended")
	defer span.End()
	limit = clampFeedLimit(limit)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(genres) == 0 {
		return nil, nil
	}

	// Build genre match score using JSONB operators
	scoreParts := make([]string, len(genres))
	args := []any{profileID}
	for i, g := range genres {
		args = append(args, g)
		scoreParts[i] = fmt.Sprintf("CASE WHEN jsonb_typeof(mc.genres) = 'array' AND mc.genres ? $%d THEN 1 ELSE 0 END", i+2)
	}
	scoreExpr := strings.Join(scoreParts, " + ")
	args = append(args, limit)
	limitIdx := len(args)
	args = append(args, pq.Array(genres))
	genresArrIdx := len(args)

	// Filter out items the profile has fully watched. For series, "completed"
	// means every episode has wp.completed=TRUE; for movies it's the item's
	// own wp row being completed. Movies/series with NO wp row for the profile
	// are unwatched (COALESCE FALSE).
	q2 := fmt.Sprintf(`
		SELECT mi.id, COALESCE(mi.parent_id,''), mi.title, mi.type, 0, '', (%s) AS score
		FROM media_items mi
		LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
		LEFT JOIN media_classifications mcl ON mcl.media_item_id = mi.id
		LEFT JOIN LATERAL (
			SELECT CASE
				WHEN mi.type = 'movie' THEN
					COALESCE((SELECT completed FROM watch_progress wp WHERE wp.media_item_id = mi.id AND wp.profile_id = $1 LIMIT 1), FALSE)
				WHEN mi.type = 'series' THEN
					-- A series is "fully watched" only when EVERY current episode
					-- has a completed wp row. A newly-added episode without any
					-- wp row makes the series re-appear in recommendations.
					NOT EXISTS (
						SELECT 1
						FROM media_items se
						JOIN media_items ep ON ep.parent_id = se.id
						WHERE se.parent_id = mi.id
						  AND ep.type = 'episode'
						  AND NOT EXISTS (
							SELECT 1 FROM watch_progress wp
							WHERE wp.media_item_id = ep.id
							  AND wp.profile_id = $1
							  AND wp.completed = TRUE
						  )
					)
				ELSE FALSE END AS completed
		) w ON TRUE
		WHERE mi.type IN ('movie', 'series')
		  -- GIN pre-filter (idx_media_catalog_genres_gin): prune to genre-
		  -- overlapping candidates by index before the per-row scoring, instead
		  -- of seq-scanning every movie/series. Equivalent to (scoreExpr) >= 1
		  -- below - both mean "shares at least one of the profile's genres".
		  AND mc.genres ?| $%d::text[]
		  AND (%s) >= 1
		  AND w.completed = FALSE
		ORDER BY score DESC, mcl.community_rating DESC NULLS LAST
		LIMIT $%d
	`, scoreExpr, genresArrIdx, scoreExpr, limitIdx)

	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, q2, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanFeedItemsWithScore(rows)
	if err != nil {
		return nil, err
	}
	q.fillPrimaryBlurhashes(ctx, items)
	return items, nil
}

// SimilarItems returns unwatched items with similar genres/director to the given item.
func (q *FeedQuerier) SimilarItems(ctx context.Context, mediaItemID, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed.SimilarItems")
	defer span.End()
	limit = clampFeedLimit(limit)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Fetch the target's genres first (PK lookup) so the candidate query can
	// prune via the GIN index (mc.genres ?| $genres) instead of running a per-row
	// LATERAL genre expansion over every movie/series.
	var targetGenres []string
	if err := txFromCtx(ctx, q.db).QueryRowContext(ctx, `
		SELECT COALESCE(array_agg(g.value), '{}')
		FROM media_catalog mc, jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(mc.genres) = 'array' THEN mc.genres ELSE '[]'::jsonb END) AS g(value)
		WHERE mc.media_item_id = $1
	`, mediaItemID).Scan(pq.Array(&targetGenres)); err != nil {
		return nil, err
	}
	if len(targetGenres) == 0 {
		return nil, nil // nothing to be similar on
	}
	targetCount := len(targetGenres)
	// A series' "Sci-Fi & Fantasy" meets a film's "Science Fiction" and "Fantasy".
	targetGenres = expandGenreFamilies(targetGenres)

	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx, `
		SELECT mi.id, COALESCE(mi.parent_id,''), mi.title, mi.type, 0, '',
		       (SELECT COUNT(*) FROM jsonb_array_elements_text(
		           CASE WHEN jsonb_typeof(mc.genres) = 'array' THEN mc.genres ELSE '[]'::jsonb END) AS g(value)
		        WHERE g.value = ANY($3::text[]))::int AS score
		FROM media_items mi
		LEFT JOIN media_catalog mc ON mc.media_item_id = mi.id
		LEFT JOIN media_classifications mcl ON mcl.media_item_id = mi.id
		LEFT JOIN LATERAL (
			SELECT CASE
				WHEN mi.type = 'movie' THEN
					COALESCE((SELECT completed FROM watch_progress wp WHERE wp.media_item_id = mi.id AND wp.profile_id = $2 LIMIT 1), FALSE)
				WHEN mi.type = 'series' THEN
					-- A series counts as watched only when EVERY current episode has a
					-- completed wp row; a series has no wp row of its own, so the old
					-- "wp.media_item_id = mi.id" check never matched and fully-watched
					-- series leaked into Similar (unlike Recommended/RecentlyAdded).
					NOT EXISTS (
						SELECT 1
						FROM media_items se
						JOIN media_items ep ON ep.parent_id = se.id
						WHERE se.parent_id = mi.id
						  AND ep.type = 'episode'
						  AND NOT EXISTS (
							SELECT 1 FROM watch_progress wp
							WHERE wp.media_item_id = ep.id
							  AND wp.profile_id = $2
							  AND wp.completed = TRUE
						  )
					)
				ELSE FALSE END AS completed
		) w ON TRUE
		WHERE mi.id != $1
		  AND mi.type IN ('movie', 'series')
		  -- GIN pre-filter (idx_media_catalog_genres_gin): only candidates that
		  -- share ≥1 linking genre (see linkingGenres), by index. Replaces the
		  -- INNER JOIN LATERAL score>=1 that forced a per-row genre scan.
		  AND mc.genres ?| $5::text[]
		  AND w.completed = FALSE
		ORDER BY score DESC, mcl.community_rating DESC NULLS LAST
		LIMIT $4
	`, mediaItemID, profileID, pq.Array(targetGenres), limit, pq.Array(linkingGenres(targetGenres)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanFeedItemsWithScore(rows)
	if err != nil {
		return nil, err
	}
	items = keepMostRelevant(items, targetCount)
	q.fillPrimaryBlurhashes(ctx, items)
	return items, nil
}

// fillPrimaryBlurhashes batch-resolves the primary-artwork blurhash for the
// given items and stamps it on each FeedItem in place. Best-effort: a query
// failure is logged and the items return without the placeholder hint -
// missing hashes degrade gracefully on the client (solid background).
func (q *FeedQuerier) fillPrimaryBlurhashes(ctx context.Context, items []*feed.FeedItem) {
	if len(items) == 0 {
		return
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = string(it.MediaItemID)
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, q.db).QueryContext(ctx,
		`SELECT media_item_id, blurhash FROM artwork
		 WHERE media_item_id = ANY($1) AND type = 'poster' AND blurhash IS NOT NULL`,
		pq.Array(ids))
	if err != nil {
		slog.Debug("feed: fetch blurhashes failed", slog.Any("err", err))
		return
	}
	defer rows.Close()
	hashes := make(map[string]string, len(items))
	for rows.Next() {
		var mediaID, hash string
		if err := rows.Scan(&mediaID, &hash); err != nil {
			slog.Debug("feed: scan blurhash row failed", slog.Any("err", err))
			return
		}
		// First hit wins - sort_order on the artwork side means the
		// most-preferred poster is returned first.
		if _, exists := hashes[mediaID]; !exists {
			hashes[mediaID] = hash
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		slog.Debug("feed: iterate blurhash rows failed", slog.Any("err", rowsErr))
		return
	}
	for _, it := range items {
		if h, ok := hashes[string(it.MediaItemID)]; ok {
			h := h
			it.PrimaryBlurhash = &h
		}
	}
}

func scanFeedItems(rows *sql.Rows) ([]*feed.FeedItem, error) {
	var result []*feed.FeedItem
	for rows.Next() {
		item := &feed.FeedItem{}
		var season, episode sql.NullInt64
		var rawPos int64
		if err := rows.Scan(&item.MediaItemID, &item.ParentID, &item.Title, &item.Type, &rawPos, &item.AddedAt, &season, &episode); err != nil {
			return nil, err
		}
		if d, err := shared.NewDurationMs(rawPos); err == nil {
			item.Position = d
		}
		if season.Valid {
			item.SeasonNumber = int(season.Int64)
		}
		if episode.Valid {
			item.EpisodeNumber = int(episode.Int64)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanFeedItemsWithScore(rows *sql.Rows) ([]*feed.FeedItem, error) {
	var result []*feed.FeedItem
	for rows.Next() {
		item := &feed.FeedItem{}
		var rawPos int64
		if err := rows.Scan(&item.MediaItemID, &item.ParentID, &item.Title, &item.Type, &rawPos, &item.AddedAt, &item.Score); err != nil {
			return nil, err
		}
		if d, err := shared.NewDurationMs(rawPos); err == nil {
			item.Position = d
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

var _ feed.FeedQuerier = (*FeedQuerier)(nil)
