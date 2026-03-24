package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type PlaySessionStatsReader struct {
	db *sql.DB

	serverStatsMu    sync.Mutex
	serverStatsCache *activity.ServerStatsResult
	serverStatsTTL   time.Time
}

func NewPlaySessionStatsReader(db *sql.DB) *PlaySessionStatsReader {
	return &PlaySessionStatsReader{db: db}
}

func (r *PlaySessionStatsReader) GetUserStats(ctx context.Context, profileID string) (*activity.UserStats, error) {
	stats := &activity.UserStats{
		ProfileID:        profileID,
		GenreBreakdown:   make(map[string]int64),
		PlayCountByMonth: make(map[string]int),
	}

	// Single round-trip: totals + genres + most-played + monthly breakdown
	const q = `
	WITH totals AS (
		SELECT COALESCE(SUM(duration_ms), 0) AS total_ms, COUNT(*) AS total_count
		FROM play_sessions WHERE profile_id = $1
	),
	genres AS (
		SELECT g.genre, COALESCE(SUM(ps.duration_ms), 0) AS total_ms
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id,
		jsonb_array_elements_text(mi.genres) AS g(genre)
		WHERE ps.profile_id = $1
		GROUP BY g.genre
		ORDER BY total_ms DESC
	),
	most_played AS (
		SELECT ps.media_item_id, mi.title, COUNT(*) AS play_count
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		WHERE ps.profile_id = $1
		GROUP BY ps.media_item_id, mi.title
		ORDER BY play_count DESC
		LIMIT 10
	),
	monthly AS (
		SELECT TO_CHAR(started_at, 'YYYY-MM') AS month, COUNT(*) AS cnt
		FROM play_sessions WHERE profile_id = $1
		GROUP BY month
		ORDER BY month
	)
	SELECT 'T' AS kind, total_ms::TEXT, total_count::TEXT, '' FROM totals
	UNION ALL
	SELECT 'G', genre, total_ms::TEXT, '' FROM genres
	UNION ALL
	SELECT 'P', media_item_id, title, play_count::TEXT FROM most_played
	UNION ALL
	SELECT 'M', month, cnt::TEXT, '' FROM monthly`

	rows, err := r.db.QueryContext(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var kind, col1, col2, col3 string
		if err := rows.Scan(&kind, &col1, &col2, &col3); err != nil {
			return nil, err
		}
		switch kind {
		case "T":
			fmt.Sscan(col1, &stats.TotalWatchTimeMs)
			fmt.Sscan(col2, &stats.TotalPlayCount)
		case "G":
			var ms int64
			fmt.Sscan(col2, &ms)
			stats.GenreBreakdown[col1] = ms
		case "P":
			var cnt int
			fmt.Sscan(col3, &cnt)
			stats.MostPlayedItems = append(stats.MostPlayedItems, activity.MediaPlayCount{
				MediaItemID: col1,
				Title:       col2,
				PlayCount:   cnt,
			})
		case "M":
			var cnt int
			fmt.Sscan(col2, &cnt)
			stats.PlayCountByMonth[col1] = cnt
		}
	}
	return stats, rows.Err()
}

const serverStatsCacheDuration = 5 * time.Minute

func (r *PlaySessionStatsReader) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	r.serverStatsMu.Lock()
	defer r.serverStatsMu.Unlock()

	if r.serverStatsCache != nil && time.Now().Before(r.serverStatsTTL) {
		cached := *r.serverStatsCache
		return &cached, nil
	}

	stats := &activity.ServerStatsResult{}
	err := r.db.QueryRowContext(ctx,
		`SELECT
			(SELECT COUNT(*) FROM play_sessions WHERE ended_at IS NULL),
			(SELECT COUNT(*) FROM transcode_sessions WHERE status IN ('queued', 'running')),
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM media_items),
			(SELECT COALESCE(SUM(duration_ms), 0) FROM play_sessions)`,
	).Scan(&stats.ActiveStreams, &stats.ActiveTranscodes, &stats.TotalUsers, &stats.TotalItems, &stats.TotalPlayTimeMs)
	if err != nil {
		return nil, err
	}

	r.serverStatsCache = stats
	r.serverStatsTTL = time.Now().Add(serverStatsCacheDuration)

	cached := *stats
	return &cached, nil
}

func (r *PlaySessionStatsReader) GetMostPopular(ctx context.Context, limit int) ([]activity.MediaPlayCount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ps.media_item_id, mi.title, COUNT(*) AS play_count
		 FROM play_sessions ps
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
		 WHERE mi.type IN ('movie', 'series')
		   AND ps.started_at >= NOW() - INTERVAL '90 days'
		 GROUP BY ps.media_item_id, mi.title
		 ORDER BY play_count DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []activity.MediaPlayCount
	for rows.Next() {
		var mpc activity.MediaPlayCount
		if err := rows.Scan(&mpc.MediaItemID, &mpc.Title, &mpc.PlayCount); err != nil {
			return nil, err
		}
		result = append(result, mpc)
	}
	return result, rows.Err()
}

