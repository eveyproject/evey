package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type AnalyticsReader struct {
	db *sql.DB
}

func NewAnalyticsReader(db *sql.DB) *AnalyticsReader {
	return &AnalyticsReader{db: db}
}

func (r *AnalyticsReader) GetAnalytics(ctx context.Context) (*activity.AnalyticsData, error) {
	data := &activity.AnalyticsData{
		TopMedia:       []activity.TopMediaItem{},
		RecentActivity: []activity.RecentActivityEntry{},
	}

	topRows, err := r.db.QueryContext(ctx, `
		SELECT ps.media_item_id, mi.title,
		       COUNT(*) AS play_count,
		       COALESCE(SUM(ps.duration_ms) / 1000, 0) AS total_duration_seconds
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		WHERE ps.started_at >= NOW() - INTERVAL '30 days'
		GROUP BY ps.media_item_id, mi.title
		ORDER BY play_count DESC
		LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer topRows.Close()
	for topRows.Next() {
		var item activity.TopMediaItem
		if scanErr := topRows.Scan(&item.MediaItemID, &item.Title, &item.PlayCount, &item.TotalDurationSeconds); scanErr != nil {
			return nil, scanErr
		}
		data.TopMedia = append(data.TopMedia, item)
	}
	if rowsErr := topRows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	recentRows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.username, mi.title, ps.started_at
		FROM play_sessions ps
		INNER JOIN profiles p ON p.id = ps.profile_id
		INNER JOIN users u ON u.id = p.user_id
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		ORDER BY ps.started_at DESC
		LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer recentRows.Close()
	for recentRows.Next() {
		var entry activity.RecentActivityEntry
		if err := recentRows.Scan(&entry.UserID, &entry.Username, &entry.MediaTitle, &entry.PlayedAt); err != nil {
			return nil, err
		}
		data.RecentActivity = append(data.RecentActivity, entry)
	}
	if err := recentRows.Err(); err != nil {
		return nil, err
	}

	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM play_sessions WHERE started_at >= CURRENT_DATE`,
	).Scan(&data.TotalPlaysToday); err != nil {
		return nil, err
	}

	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM play_sessions WHERE started_at >= date_trunc('week', NOW())`,
	).Scan(&data.TotalPlaysThisWeek); err != nil {
		return nil, err
	}

	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT p.user_id)
		FROM play_sessions ps
		INNER JOIN profiles p ON p.id = ps.profile_id
		WHERE ps.started_at >= CURRENT_DATE`,
	).Scan(&data.ActiveUsersToday); err != nil {
		return nil, err
	}

	return data, nil
}
