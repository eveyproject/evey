package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/feed"
)

type FeedQuerier struct {
	db *sql.DB
}

func NewFeedQuerier(db *sql.DB) *FeedQuerier {
	return &FeedQuerier{db: db}
}

// ContinueWatching returns in-progress films and the next episode to watch per series.
func (q *FeedQuerier) ContinueWatching(ctx context.Context, profileID string, limit int) ([]*feed.FeedItem, error) {
	// Movies in progress
	rows, err := q.db.QueryContext(ctx, `
		SELECT mi.id, '', mi.title, mi.type, wp.position_ms, ''
		FROM watch_progress wp
		INNER JOIN media_items mi ON mi.id = wp.media_item_id
		WHERE wp.profile_id = $1
		  AND mi.type = 'movie'
		  AND wp.position_ms > 0
		  AND wp.completed = FALSE
		ORDER BY wp.last_played_at DESC
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	movies, err := scanFeedItems(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}

	// Episodes: in-progress or next unwatched per series
	rows, err = q.db.QueryContext(ctx, `
		WITH series_list AS (
			SELECT DISTINCT mi.parent_id AS season_id, s.parent_id AS series_id
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
		),
		in_progress AS (
			SELECT mi.id, mi.parent_id, mi.title, wp.position_ms, s.parent_id AS series_id, wp.last_played_at
			FROM watch_progress wp
			INNER JOIN media_items mi ON mi.id = wp.media_item_id
			INNER JOIN media_items s ON s.id = mi.parent_id
			WHERE wp.profile_id = $1
			  AND mi.type = 'episode'
			  AND wp.position_ms > 0
			  AND wp.completed = FALSE
		),
		next_unwatched AS (
			SELECT DISTINCT ON (s.parent_id) mi.id, mi.parent_id, mi.title, 0 AS position_ms, s.parent_id AS series_id, NOW() AS last_played_at
			FROM media_items mi
			INNER JOIN media_items s ON s.id = mi.parent_id
			INNER JOIN series_list sl ON sl.series_id = s.parent_id
			WHERE mi.type = 'episode'
			  AND mi.id NOT IN (SELECT media_item_id FROM watch_progress WHERE profile_id = $1 AND completed = TRUE)
			  AND mi.id NOT IN (SELECT id FROM in_progress)
			ORDER BY s.parent_id, s.index_number, mi.index_number
		)
		SELECT id, parent_id, title, 'episode', position_ms, ''
		FROM (
			SELECT id, parent_id, title, position_ms, series_id, last_played_at FROM in_progress
			UNION ALL
			SELECT id, parent_id, title, position_ms, series_id, last_played_at FROM next_unwatched
		) combined
		ORDER BY series_id, last_played_at DESC
		LIMIT $2
	`, profileID, limit)
	if err != nil {
		return nil, err
	}
	episodes, err := scanFeedItems(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}

	result := append(movies, episodes...)
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// RecentlyAdded returns media items added within daysWindow days, most recent first.
func (q *FeedQuerier) RecentlyAdded(ctx context.Context, daysWindow, limit int) ([]*feed.FeedItem, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, COALESCE(parent_id,''), title, type, 0, added_at::text
		FROM media_items
		WHERE added_at > NOW() - ($1 || ' days')::interval
		  AND type IN ('movie', 'series', 'album')
		ORDER BY added_at DESC
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
	if len(genres) == 0 {
		return nil, nil
	}

	// Build genre match score using JSONB operators
	scoreParts := make([]string, len(genres))
	args := []interface{}{profileID}
	for i, g := range genres {
		args = append(args, g)
		scoreParts[i] = fmt.Sprintf("CASE WHEN mi.genres ? $%d THEN 1 ELSE 0 END", i+2)
	}
	scoreExpr := strings.Join(scoreParts, " + ")
	args = append(args, limit)
	limitIdx := len(args)

	q2 := fmt.Sprintf(`
		SELECT mi.id, COALESCE(mi.parent_id,''), mi.title, mi.type, 0, '', (%s) AS score
		FROM media_items mi
		WHERE mi.type IN ('movie', 'series')
		  AND mi.id NOT IN (
		      SELECT media_item_id FROM watch_progress WHERE profile_id = $1 AND completed = TRUE
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
	rows, err := q.db.QueryContext(ctx, `
		WITH target AS (
			SELECT genres FROM media_items WHERE id = $1
		)
		SELECT mi.id, COALESCE(mi.parent_id,''), mi.title, mi.type, 0, '',
		       (SELECT COUNT(*) FROM jsonb_array_elements_text(mi.genres) g
		        WHERE g IN (SELECT jsonb_array_elements_text(genres) FROM target)) AS score
		FROM media_items mi, target
		WHERE mi.id != $1
		  AND mi.type IN ('movie', 'series')
		  AND mi.id NOT IN (
		      SELECT media_item_id FROM watch_progress WHERE profile_id = $2 AND completed = TRUE
		  )
		  AND (SELECT COUNT(*) FROM jsonb_array_elements_text(mi.genres) g
		       WHERE g IN (SELECT jsonb_array_elements_text(genres) FROM target)) >= 1
		ORDER BY score DESC, mi.community_rating DESC NULLS LAST
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
		if err := rows.Scan(&item.MediaItemID, &item.ParentID, &item.Title, &item.Type, &item.PositionMs, &item.AddedAt); err != nil {
			return nil, err
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
