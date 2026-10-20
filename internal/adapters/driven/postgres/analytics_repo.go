package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type AnalyticsReader struct {
	db *sql.DB
}

func NewAnalyticsReader(db *sql.DB) *AnalyticsReader {
	return &AnalyticsReader{db: db}
}

func (r *AnalyticsReader) GetAnalytics(ctx context.Context) (*activity.AnalyticsData, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.analytics.GetAnalytics")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Detach any ambient transaction: the 8 queries below fan out across
	// goroutines, and database/sql forbids concurrent use of a single *sql.Tx.
	// After this, each helper's txFromCtx falls back to the pooled *sql.DB and
	// takes its own connection. This read-only aggregate never needs to
	// participate in a caller's transaction.
	ctx = detachTx(ctx)

	var (
		topMedia       []activity.TopMediaItem
		recentActivity []activity.RecentActivityEntry
		playsToday     int
		playsWeek      int
		activeToday    int
		dailyPlays     []activity.DailyPlayCount
		playMethods    []activity.PlayMethodCount
		hourlyPlays    []activity.HourlyPlayCount
		errs           [8]error
		wg             sync.WaitGroup
	)
	wg.Add(8)

	go r.safeRun(ctx, &wg, "top media", func() error {
		var err error
		topMedia, err = r.fetchTopMedia(ctx)
		return err
	}, &errs[0])

	go r.safeRun(ctx, &wg, "recent activity", func() error {
		var err error
		recentActivity, err = r.fetchRecentActivity(ctx)
		return err
	}, &errs[1])

	go r.safeRun(ctx, &wg, "plays today", func() error {
		var err error
		playsToday, err = r.fetchScalarCount(ctx, `SELECT COUNT(*) FROM play_sessions WHERE started_at >= CURRENT_DATE`)
		return err
	}, &errs[2])

	go r.safeRun(ctx, &wg, "plays week", func() error {
		var err error
		playsWeek, err = r.fetchScalarCount(ctx, `SELECT COUNT(*) FROM play_sessions WHERE started_at >= date_trunc('week', NOW())`)
		return err
	}, &errs[3])

	go r.safeRun(ctx, &wg, "active users", func() error {
		var err error
		activeToday, err = r.fetchScalarCount(ctx, `
			SELECT COUNT(DISTINCT p.user_id)
			FROM play_sessions ps
			INNER JOIN profiles p ON p.id = ps.profile_id
			WHERE ps.started_at >= CURRENT_DATE`)
		return err
	}, &errs[4])

	go r.safeRun(ctx, &wg, "daily plays", func() error {
		var err error
		dailyPlays, err = r.fetchDailyPlays(ctx)
		return err
	}, &errs[5])

	go r.safeRun(ctx, &wg, "play methods", func() error {
		var err error
		playMethods, err = r.fetchPlayMethods(ctx)
		return err
	}, &errs[6])

	go r.safeRun(ctx, &wg, "hourly plays", func() error {
		var err error
		hourlyPlays, err = r.fetchHourlyPlays(ctx)
		return err
	}, &errs[7])

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return &activity.AnalyticsData{
		TopMedia:           orEmpty(topMedia),
		RecentActivity:     orEmpty(recentActivity),
		TotalPlaysToday:    playsToday,
		TotalPlaysThisWeek: playsWeek,
		ActiveUsersToday:   activeToday,
		DailyPlays:         orEmpty(dailyPlays),
		PlayMethods:        orEmpty(playMethods),
		HourlyPlays:        orEmpty(hourlyPlays),
	}, nil
}

func (r *AnalyticsReader) safeRun(ctx context.Context, wg *sync.WaitGroup, label string, fn func() error, errOut *error) {
	defer wg.Done()
	defer func() {
		if rec := recover(); rec != nil {
			slog.ErrorContext(ctx, "panic recovered in analytics "+label, "error", rec, "stack", string(debug.Stack()))
		}
	}()
	*errOut = fn()
}

func (r *AnalyticsReader) fetchTopMedia(ctx context.Context) ([]activity.TopMediaItem, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT ps.media_item_id,
		       mi.title, mi.type, COALESCE(series.title, ''), COALESCE(season.index_number, 0), COALESCE(mi.index_number, 0),
		       COUNT(*) AS play_count,
		       COALESCE(SUM(COALESCE(ps.position_ms, ps.duration_ms, 0))::bigint / 1000, 0) AS total_duration_seconds
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		LEFT JOIN media_items season ON season.id = mi.parent_id AND mi.type = 'episode'
		LEFT JOIN media_items series ON series.id = season.parent_id AND mi.type = 'episode'
		WHERE ps.started_at >= NOW() - INTERVAL '30 days'
		GROUP BY ps.media_item_id, mi.title, mi.type, mi.index_number, mi.parent_id, season.index_number, season.parent_id, series.title
		ORDER BY play_count DESC
		LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []activity.TopMediaItem
	for rows.Next() {
		var item activity.TopMediaItem
		var mediaType, seriesTitle string
		var seasonNum, episodeNum int
		if err := rows.Scan(&item.MediaItemID, &item.Title, &mediaType, &seriesTitle, &seasonNum, &episodeNum, &item.PlayCount, &item.TotalDurationSeconds); err != nil {
			return nil, err
		}
		if mediaType == string(library.Episode) {
			item.Title = library.EpisodeLabel(seriesTitle, seasonNum, episodeNum, item.Title)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AnalyticsReader) fetchRecentActivity(ctx context.Context) ([]activity.RecentActivityEntry, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT u.id, u.username, mi.title, ps.started_at,
		       mi.type, COALESCE(series.title, ''), COALESCE(season.index_number, 0), COALESCE(mi.index_number, 0)
		FROM play_sessions ps
		INNER JOIN profiles p ON p.id = ps.profile_id
		INNER JOIN users u ON u.id = p.user_id
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		LEFT JOIN media_items season ON season.id = mi.parent_id AND mi.type = 'episode'
		LEFT JOIN media_items series ON series.id = season.parent_id AND mi.type = 'episode'
		ORDER BY ps.started_at DESC
		LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []activity.RecentActivityEntry
	for rows.Next() {
		var entry activity.RecentActivityEntry
		var mediaType, seriesTitle string
		var seasonNum, episodeNum int
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.MediaTitle, &entry.PlayedAt, &mediaType, &seriesTitle, &seasonNum, &episodeNum); err != nil {
			return nil, err
		}
		if mediaType == string(library.Episode) {
			entry.MediaTitle = library.EpisodeLabel(seriesTitle, seasonNum, episodeNum, entry.MediaTitle)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *AnalyticsReader) fetchScalarCount(ctx context.Context, query string) (int, error) {
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

func (r *AnalyticsReader) fetchDailyPlays(ctx context.Context) ([]activity.DailyPlayCount, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT d::date::text AS day,
		       COUNT(ps.id) AS play_count,
		       COALESCE(SUM(COALESCE(ps.position_ms, ps.duration_ms, 0))::bigint / 1000, 0) AS watch_time_seconds
		FROM generate_series(CURRENT_DATE - INTERVAL '13 days', CURRENT_DATE, '1 day') AS d
		-- Range predicate (not ps.started_at::date = d::date): the cast wraps the
		-- indexed column and forces a full scan of all history. >= d AND < d+1day
		-- is the same calendar day but lets the planner use idx_play_sessions_started_at
		-- and only touch the 14-day window.
		LEFT JOIN play_sessions ps ON ps.started_at >= d AND ps.started_at < d + INTERVAL '1 day'
		GROUP BY d
		ORDER BY d`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []activity.DailyPlayCount
	for rows.Next() {
		var item activity.DailyPlayCount
		if err := rows.Scan(&item.Date, &item.PlayCount, &item.WatchTimeSeconds); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AnalyticsReader) fetchPlayMethods(ctx context.Context) ([]activity.PlayMethodCount, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT play_method, COUNT(*) AS cnt
		FROM play_sessions
		WHERE started_at >= NOW() - INTERVAL '30 days'
		GROUP BY play_method
		ORDER BY cnt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []activity.PlayMethodCount
	for rows.Next() {
		var item activity.PlayMethodCount
		if err := rows.Scan(&item.Method, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AnalyticsReader) fetchHourlyPlays(ctx context.Context) ([]activity.HourlyPlayCount, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT EXTRACT(HOUR FROM started_at)::int AS hour, COUNT(*) AS cnt
		FROM play_sessions
		WHERE started_at >= NOW() - INTERVAL '30 days'
		GROUP BY hour
		ORDER BY hour`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []activity.HourlyPlayCount
	for rows.Next() {
		var item activity.HourlyPlayCount
		if err := rows.Scan(&item.Hour, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

var _ activity.AnalyticsReader = (*AnalyticsReader)(nil)
