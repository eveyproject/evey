package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type PlaySessionStatsReader struct {
	db *sql.DB
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

	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(duration_ms), 0), COUNT(*)
		 FROM play_sessions WHERE profile_id = $1`, profileID,
	).Scan(&stats.TotalWatchTimeMs, &stats.TotalPlayCount)
	if err != nil {
		return nil, err
	}

	genreRows, err := r.db.QueryContext(ctx,
		`SELECT g.genre, COALESCE(SUM(ps.duration_ms), 0) AS total_ms
		 FROM play_sessions ps
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id,
		 jsonb_array_elements_text(mi.genres) AS g(genre)
		 WHERE ps.profile_id = $1
		 GROUP BY g.genre
		 ORDER BY total_ms DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer genreRows.Close()
	for genreRows.Next() {
		var genre string
		var totalMs int64
		if err := genreRows.Scan(&genre, &totalMs); err != nil {
			return nil, err
		}
		stats.GenreBreakdown[genre] = totalMs
	}
	if err := genreRows.Err(); err != nil {
		return nil, err
	}

	mostPlayed, err := r.db.QueryContext(ctx,
		`SELECT ps.media_item_id, mi.title, COUNT(*) AS play_count
		 FROM play_sessions ps
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
		 WHERE ps.profile_id = $1
		 GROUP BY ps.media_item_id, mi.title
		 ORDER BY play_count DESC
		 LIMIT 10`, profileID)
	if err != nil {
		return nil, err
	}
	defer mostPlayed.Close()
	for mostPlayed.Next() {
		var mpc activity.MediaPlayCount
		if err := mostPlayed.Scan(&mpc.MediaItemID, &mpc.Title, &mpc.PlayCount); err != nil {
			return nil, err
		}
		stats.MostPlayedItems = append(stats.MostPlayedItems, mpc)
	}
	if err := mostPlayed.Err(); err != nil {
		return nil, err
	}

	monthRows, err := r.db.QueryContext(ctx,
		`SELECT TO_CHAR(started_at, 'YYYY-MM') AS month, COUNT(*) AS cnt
		 FROM play_sessions WHERE profile_id = $1
		 GROUP BY month
		 ORDER BY month`, profileID)
	if err != nil {
		return nil, err
	}
	defer monthRows.Close()
	for monthRows.Next() {
		var month string
		var cnt int
		if err := monthRows.Scan(&month, &cnt); err != nil {
			return nil, err
		}
		stats.PlayCountByMonth[month] = cnt
	}
	if err := monthRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *PlaySessionStatsReader) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	stats := &activity.ServerStatsResult{}

	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM play_sessions WHERE ended_at IS NULL`,
	).Scan(&stats.ActiveStreams)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transcode_sessions WHERE status IN ('queued', 'running')`,
	).Scan(&stats.ActiveTranscodes)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users`,
	).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_items`,
	).Scan(&stats.TotalItems)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(duration_ms), 0) FROM play_sessions`,
	).Scan(&stats.TotalPlayTimeMs)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *PlaySessionStatsReader) GetMostPopular(ctx context.Context, limit int) ([]activity.MediaPlayCount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ps.media_item_id, mi.title, COUNT(*) AS play_count
		 FROM play_sessions ps
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
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

// ensure json import is used
var _ = json.Marshal
