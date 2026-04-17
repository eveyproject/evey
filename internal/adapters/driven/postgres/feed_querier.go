package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/evey-project/evey/internal/domain/feed"
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
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	limit = clampFeedLimit(limit)
	rows, err := q.db.QueryContext(ctx, `
		WITH in_progress_movies AS (
			SELECT mi.id, ''::text AS parent_id, mi.title, mi.type,
			       wp.position_ms, ''::text AS added_at, wp.last_played_at,
			       NULL::int AS season_number, NULL::int AS episode_number
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'movie'
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
		),
		series_list AS (
			SELECT DISTINCT mi.parent_id AS season_id, s.parent_id AS series_id
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
		),
		in_progress_eps AS (
			SELECT mi.id, mi.parent_id, mi.title, 'episode'::text AS type,
			       wp.position_ms, ''::text AS added_at, wp.last_played_at,
			       s.index_number AS season_number, mi.index_number AS episode_number
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
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
			  AND NOT EXISTS (SELECT 1 FROM in_progress_eps ip WHERE ip.id = mi.id)
			ORDER BY s.parent_id, s.index_number, mi.index_number
		)
		SELECT id, parent_id, title, type, position_ms, added_at, season_number, episode_number
		FROM (
			SELECT * FROM in_progress_movies
			UNION ALL
			SELECT * FROM in_progress_eps
			UNION ALL
			SELECT * FROM next_unwatched
		) combined
		ORDER BY last_played_at DESC
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedItems(rows)
}

// ResumePlayback returns in-progress media AND next episodes per series, sorted by last user interaction.
func (q *FeedQuerier) ResumePlayback(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	limit = clampFeedLimit(limit)
	rows, err := q.db.QueryContext(ctx, `
		WITH in_progress AS (
			SELECT mi.id, COALESCE(mi.parent_id,'') AS parent_id, mi.title, mi.type,
			       wp.position_ms, wp.last_played_at,
			       sea.index_number AS season_number, mi.index_number AS episode_number
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			LEFT JOIN media_items sea ON mi.type = 'episode' AND sea.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
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
			AND NOT EXISTS (SELECT 1 FROM in_progress ip WHERE ip.id = mi.id)
			ORDER BY ws.series_id, sea.index_number, mi.index_number
		)
		SELECT id, parent_id, title, type, position_ms, '', season_number, episode_number
		FROM (
			SELECT id, parent_id, title, type, position_ms, last_played_at, season_number, episode_number FROM in_progress
			UNION ALL
			SELECT id, parent_id, title, type, position_ms, last_played_at, season_number, episode_number FROM next_ep
		) combined
		ORDER BY last_played_at DESC
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedItems(rows)
}

// NextEpisode returns the next unwatched episode per series where the user completed at least one episode.
func (q *FeedQuerier) NextEpisode(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	limit = clampFeedLimit(limit)
	rows, err := q.db.QueryContext(ctx, `
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
	return scanFeedItems(rows)
}

// RecentlyAdded returns media items added within daysWindow days, most recent first.
func (q *FeedQuerier) RecentlyAdded(ctx context.Context, daysWindow, limit int) ([]*feed.FeedItem, error) {
	if daysWindow <= 0 {
		daysWindow = 30
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, COALESCE(parent_id,''), title, type, 0, effective_added::text, NULL::int, NULL::int
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
		WHERE effective_added > NOW() - ($1 || ' days')::interval
		ORDER BY effective_added DESC
		LIMIT $2
	`, daysWindow, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedItems(rows)
}

// Recommended returns unwatched items matching the given genres, scored by match count.
func (q *FeedQuerier) Recommended(ctx context.Context, profileID string, genres []string, limit int) ([]*feed.FeedItem, error) {
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
		scoreParts[i] = fmt.Sprintf("CASE WHEN jsonb_typeof(mi.genres) = 'array' AND mi.genres ? $%d THEN 1 ELSE 0 END", i+2)
	}
	scoreExpr := strings.Join(scoreParts, " + ")
	args = append(args, limit)
	limitIdx := len(args)

	// Exclude items that the user has watched, finished, OR is currently
	// watching (any watch_progress row): for movies this is the row on mi
	// itself; for series this is any episode whose chain leads back to mi.
	q2 := fmt.Sprintf(`
		SELECT mi.id, COALESCE(mi.parent_id,''), mi.title, mi.type, 0, '', (%s) AS score
		FROM media_items mi
		WHERE mi.type IN ('movie', 'series')
		  AND NOT EXISTS (SELECT 1 FROM watch_progress wp WHERE wp.media_item_id = mi.id AND wp.profile_id = $1)
		  AND NOT EXISTS (
		    SELECT 1
		    FROM watch_progress wp_ep
		    JOIN media_items ep ON ep.id = wp_ep.media_item_id
		    JOIN media_items se ON se.id = ep.parent_id
		    WHERE wp_ep.profile_id = $1 AND se.parent_id = mi.id
		  )
		  AND (%s) >= 1
		ORDER BY score DESC, mi.community_rating DESC NULLS LAST
		LIMIT $%d
	`, scoreExpr, scoreExpr, limitIdx)

	rows, err := q.db.QueryContext(ctx, q2, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedItemsWithScore(rows)
}

// SimilarItems returns unwatched items with similar genres/director to the given item.
func (q *FeedQuerier) SimilarItems(ctx context.Context, mediaItemID, profileID string, limit int) ([]*feed.FeedItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := q.db.QueryContext(ctx, `
		WITH target AS (
			SELECT CASE WHEN jsonb_typeof(genres) = 'array' THEN genres ELSE '[]'::jsonb END AS genres
			FROM media_items WHERE id = $1
		),
		target_genres AS (
			SELECT g.value AS genre FROM target, jsonb_array_elements_text(target.genres) AS g(value)
		)
		SELECT mi.id, COALESCE(mi.parent_id,''), mi.title, mi.type, 0, '',
		       gs.score
		FROM media_items mi
		INNER JOIN LATERAL (
			SELECT COUNT(*)::int AS score
			FROM jsonb_array_elements_text(
				CASE WHEN jsonb_typeof(mi.genres) = 'array' THEN mi.genres ELSE '[]'::jsonb END
			) AS g(value)
			WHERE g.value IN (SELECT genre FROM target_genres)
		) gs ON gs.score >= 1
		WHERE mi.id != $1
		  AND mi.type IN ('movie', 'series')
		  AND NOT EXISTS (SELECT 1 FROM watch_progress wp2 WHERE wp2.media_item_id = mi.id AND wp2.profile_id = $2 AND wp2.completed = TRUE)
		ORDER BY gs.score DESC, mi.community_rating DESC NULLS LAST
		LIMIT $3
	`, mediaItemID, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedItemsWithScore(rows)
}

func scanFeedItems(rows *sql.Rows) ([]*feed.FeedItem, error) {
	var result []*feed.FeedItem
	for rows.Next() {
		item := &feed.FeedItem{}
		var season, episode sql.NullInt64
		if err := rows.Scan(&item.MediaItemID, &item.ParentID, &item.Title, &item.Type, &item.PositionMs, &item.AddedAt, &season, &episode); err != nil {
			return nil, err
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
		if err := rows.Scan(&item.MediaItemID, &item.ParentID, &item.Title, &item.Type, &item.PositionMs, &item.AddedAt, &item.Score); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

var _ feed.FeedQuerier = (*FeedQuerier)(nil)
