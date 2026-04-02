package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type AnalyticsReader struct {
	db *sql.DB
}

func NewAnalyticsReader(db *sql.DB) *AnalyticsReader {
	return &AnalyticsReader{db: db}
}

func (r *AnalyticsReader) GetAnalytics(ctx context.Context) (*activity.AnalyticsData, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	type result struct {
		topMedia       []activity.TopMediaItem
		recentActivity []activity.RecentActivityEntry
		playsToday     int
		playsWeek      int
		activeToday    int
		err            error
	}

	results := make([]result, 5)
	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics top media", "error", r, "stack", debug.Stack())
			}
		}()
		rows, err := r.db.QueryContext(ctx, `
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
			results[0].err = err
			return
		}
		defer rows.Close()
		var items []activity.TopMediaItem
		for rows.Next() {
			var item activity.TopMediaItem
			if scanErr := rows.Scan(&item.MediaItemID, &item.Title, &item.PlayCount, &item.TotalDurationSeconds); scanErr != nil {
				results[0].err = scanErr
				return
			}
			items = append(items, item)
		}
		results[0].err = rows.Err()
		results[0].topMedia = items
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics recent activity", "error", r, "stack", debug.Stack())
			}
		}()
		rows, err := r.db.QueryContext(ctx, `
			SELECT u.id, u.username, mi.title, ps.started_at
			FROM play_sessions ps
			INNER JOIN profiles p ON p.id = ps.profile_id
			INNER JOIN users u ON u.id = p.user_id
			INNER JOIN media_items mi ON mi.id = ps.media_item_id
			ORDER BY ps.started_at DESC
			LIMIT 20`)
		if err != nil {
			results[1].err = err
			return
		}
		defer rows.Close()
		var entries []activity.RecentActivityEntry
		for rows.Next() {
			var entry activity.RecentActivityEntry
			if scanErr := rows.Scan(&entry.UserID, &entry.Username, &entry.MediaTitle, &entry.PlayedAt); scanErr != nil {
				results[1].err = scanErr
				return
			}
			entries = append(entries, entry)
		}
		results[1].err = rows.Err()
		results[1].recentActivity = entries
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics plays today", "error", r, "stack", debug.Stack())
			}
		}()
		results[2].err = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM play_sessions WHERE started_at >= CURRENT_DATE`,
		).Scan(&results[2].playsToday)
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics plays week", "error", r, "stack", debug.Stack())
			}
		}()
		results[3].err = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM play_sessions WHERE started_at >= date_trunc('week', NOW())`,
		).Scan(&results[3].playsWeek)
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics active users", "error", r, "stack", debug.Stack())
			}
		}()
		results[4].err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT p.user_id)
			FROM play_sessions ps
			INNER JOIN profiles p ON p.id = ps.profile_id
			WHERE ps.started_at >= CURRENT_DATE`,
		).Scan(&results[4].activeToday)
	}()

	wg.Wait()

	for _, res := range results {
		if res.err != nil {
			return nil, res.err
		}
	}

	top := results[0].topMedia
	if top == nil {
		top = []activity.TopMediaItem{}
	}
	recent := results[1].recentActivity
	if recent == nil {
		recent = []activity.RecentActivityEntry{}
	}

	return &activity.AnalyticsData{
		TopMedia:           top,
		RecentActivity:     recent,
		TotalPlaysToday:    results[2].playsToday,
		TotalPlaysThisWeek: results[3].playsWeek,
		ActiveUsersToday:   results[4].activeToday,
	}, nil
}

var _ activity.AnalyticsReader = (*AnalyticsReader)(nil)
