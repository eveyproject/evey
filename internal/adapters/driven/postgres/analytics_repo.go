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
		dailyPlays     []activity.DailyPlayCount
		playMethods    []activity.PlayMethodCount
		hourlyPlays    []activity.HourlyPlayCount
		err            error
	}

	results := make([]result, 8)
	var wg sync.WaitGroup
	wg.Add(8)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics top media", "error", r, "stack", debug.Stack())
			}
		}()
		rows, err := r.db.QueryContext(ctx, `
			SELECT ps.media_item_id,
			       CASE
			         WHEN mi.type = 'episode' THEN
			           COALESCE(series.title, '') || ' - S' ||
			           LPAD(COALESCE(season.index_number, 0)::text, 2, '0') || 'E' ||
			           LPAD(COALESCE(mi.index_number, 0)::text, 2, '0') ||
			           CASE WHEN mi.title != '' THEN ' - ' || mi.title ELSE '' END
			         ELSE mi.title
			       END AS display_title,
			       COUNT(*) AS play_count,
			       COALESCE(SUM(ps.duration_ms)::bigint / 1000, 0) AS total_duration_seconds
			FROM play_sessions ps
			INNER JOIN media_items mi ON mi.id = ps.media_item_id
			LEFT JOIN media_items season ON season.id = mi.parent_id AND mi.type = 'episode'
			LEFT JOIN media_items series ON series.id = season.parent_id AND mi.type = 'episode'
			WHERE ps.started_at >= NOW() - INTERVAL '30 days'
			GROUP BY ps.media_item_id, mi.title, mi.type, mi.index_number, mi.parent_id, season.index_number, season.parent_id, series.title
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

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics daily plays", "error", r, "stack", debug.Stack())
			}
		}()
		rows, err := r.db.QueryContext(ctx, `
			SELECT d::date::text AS day,
			       COUNT(ps.id) AS play_count,
			       COALESCE(SUM(ps.duration_ms)::bigint / 1000, 0) AS watch_time_seconds
			FROM generate_series(CURRENT_DATE - INTERVAL '13 days', CURRENT_DATE, '1 day') AS d
			LEFT JOIN play_sessions ps ON ps.started_at::date = d::date
			GROUP BY d
			ORDER BY d`)
		if err != nil {
			results[5].err = err
			return
		}
		defer rows.Close()
		var items []activity.DailyPlayCount
		for rows.Next() {
			var item activity.DailyPlayCount
			if scanErr := rows.Scan(&item.Date, &item.PlayCount, &item.WatchTimeSeconds); scanErr != nil {
				results[5].err = scanErr
				return
			}
			items = append(items, item)
		}
		results[5].err = rows.Err()
		results[5].dailyPlays = items
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics play methods", "error", r, "stack", debug.Stack())
			}
		}()
		rows, err := r.db.QueryContext(ctx, `
			SELECT play_method, COUNT(*) AS cnt
			FROM play_sessions
			WHERE started_at >= NOW() - INTERVAL '30 days'
			GROUP BY play_method
			ORDER BY cnt DESC`)
		if err != nil {
			results[6].err = err
			return
		}
		defer rows.Close()
		var items []activity.PlayMethodCount
		for rows.Next() {
			var item activity.PlayMethodCount
			if scanErr := rows.Scan(&item.Method, &item.Count); scanErr != nil {
				results[6].err = scanErr
				return
			}
			items = append(items, item)
		}
		results[6].err = rows.Err()
		results[6].playMethods = items
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in analytics hourly plays", "error", r, "stack", debug.Stack())
			}
		}()
		rows, err := r.db.QueryContext(ctx, `
			SELECT EXTRACT(HOUR FROM started_at)::int AS hour, COUNT(*) AS cnt
			FROM play_sessions
			WHERE started_at >= NOW() - INTERVAL '30 days'
			GROUP BY hour
			ORDER BY hour`)
		if err != nil {
			results[7].err = err
			return
		}
		defer rows.Close()
		var items []activity.HourlyPlayCount
		for rows.Next() {
			var item activity.HourlyPlayCount
			if scanErr := rows.Scan(&item.Hour, &item.Count); scanErr != nil {
				results[7].err = scanErr
				return
			}
			items = append(items, item)
		}
		results[7].err = rows.Err()
		results[7].hourlyPlays = items
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

	daily := results[5].dailyPlays
	if daily == nil {
		daily = []activity.DailyPlayCount{}
	}
	methods := results[6].playMethods
	if methods == nil {
		methods = []activity.PlayMethodCount{}
	}
	hourly := results[7].hourlyPlays
	if hourly == nil {
		hourly = []activity.HourlyPlayCount{}
	}

	return &activity.AnalyticsData{
		TopMedia:           top,
		RecentActivity:     recent,
		TotalPlaysToday:    results[2].playsToday,
		TotalPlaysThisWeek: results[3].playsWeek,
		ActiveUsersToday:   results[4].activeToday,
		DailyPlays:         daily,
		PlayMethods:        methods,
		HourlyPlays:        hourly,
	}, nil
}

var _ activity.AnalyticsReader = (*AnalyticsReader)(nil)
