package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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

func (r *PlaySessionStatsReader) GetUserStats(ctx context.Context, profileID shared.ProfileID) (*activity.UserStats, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_session_stats.GetUserStats")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	stats := &activity.UserStats{
		ProfileID:      profileID,
		GenreBreakdown: make(map[string]activity.GenreStats),
		PlayByMonth:    make(map[string]activity.MonthlyStats),
	}

	// Single round-trip: totals + genres + most-played + monthly breakdown
	// "Temps de visionnage" = content actually viewed, measured as the playhead
	// position (COALESCE(position_ms, duration_ms, 0): the content offset, with
	// the session wall-clock as fallback when no position was reported - e.g.
	// sessions closed as stale, which never set duration_ms and previously
	// counted as 0). For per-(profile,media) totals we take MAX(position) so
	// resuming a title doesn't double-count the rewatched span. Cross-cutting
	// rollups (genres, monthly) sum the per-session figure: the same column
	// choice fixes the stale-session undercount and the cross-stat
	// inconsistency; a small resume double-count there is an accepted
	// approximation (true de-dup needs watched-interval tracking we don't keep).
	const q = `
	WITH totals AS (
		SELECT (SELECT COALESCE(SUM(viewed), 0) FROM (
		            SELECT MAX(COALESCE(position_ms, duration_ms, 0)) AS viewed
		            FROM play_sessions WHERE profile_id = $1 GROUP BY media_item_id
		        ) v) AS total_ms,
		       COUNT(*) AS total_count,
		       COUNT(DISTINCT media_item_id) AS unique_items
		FROM play_sessions WHERE profile_id = $1
	),
	genres AS (
		SELECT g.genre,
		       COALESCE(SUM(COALESCE(ps.position_ms, ps.duration_ms, 0)), 0) AS total_ms,
		       COUNT(*) AS play_count
		FROM play_sessions ps
		INNER JOIN media_catalog mc ON mc.media_item_id = ps.media_item_id,
		jsonb_array_elements_text(mc.genres) AS g(genre)
		WHERE ps.profile_id = $1
		GROUP BY g.genre
		ORDER BY total_ms DESC
	),
	most_played AS (
		SELECT ps.media_item_id, mi.title,
		       COUNT(*) AS play_count,
		       MAX(COALESCE(ps.position_ms, ps.duration_ms, 0)) AS total_ms
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		WHERE ps.profile_id = $1
		GROUP BY ps.media_item_id, mi.title
		ORDER BY play_count DESC
		LIMIT 10
	),
	monthly AS (
		SELECT TO_CHAR(started_at, 'YYYY-MM') AS month,
		       COUNT(*) AS cnt,
		       COALESCE(SUM(COALESCE(position_ms, duration_ms, 0)), 0) AS total_ms
		FROM play_sessions WHERE profile_id = $1
		GROUP BY month
		ORDER BY month
	)
	SELECT 'T' AS kind, total_ms::TEXT, total_count::TEXT, unique_items::TEXT FROM totals
	UNION ALL
	SELECT 'G', genre, total_ms::TEXT, play_count::TEXT FROM genres
	UNION ALL
	SELECT 'P', media_item_id, title, play_count::TEXT || ':' || total_ms::TEXT FROM most_played
	UNION ALL
	SELECT 'M', month, cnt::TEXT, total_ms::TEXT FROM monthly`

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var kind, col1, col2, col3 string
		if err := rows.Scan(&kind, &col1, &col2, &col3); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		switch kind {
		case "T":
			fmt.Sscan(col1, &stats.TotalWatchTimeMs)
			fmt.Sscan(col2, &stats.TotalPlayCount)
			fmt.Sscan(col3, &stats.UniqueItemsWatched)
		case "G":
			var ms int64
			var cnt int
			fmt.Sscan(col2, &ms)
			fmt.Sscan(col3, &cnt)
			stats.GenreBreakdown[col1] = activity.GenreStats{TotalTimeMs: ms, PlayCount: cnt}
		case "P":
			var cnt int
			var totalMs int64
			parts := strings.SplitN(col3, ":", 2)
			fmt.Sscan(parts[0], &cnt)
			if len(parts) > 1 {
				fmt.Sscan(parts[1], &totalMs)
			}
			stats.MostPlayedItems = append(stats.MostPlayedItems, activity.MediaPlayCount{
				MediaItemID: shared.MediaItemID(col1),
				Title:       col2,
				PlayCount:   cnt,
				TotalTimeMs: totalMs,
			})
		case "M":
			var cnt int
			var totalMs int64
			fmt.Sscan(col2, &cnt)
			fmt.Sscan(col3, &totalMs)
			stats.PlayByMonth[col1] = activity.MonthlyStats{PlayCount: cnt, TotalTimeMs: totalMs}
		}
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return stats, nil
}

const serverStatsCacheDuration = 5 * time.Minute

func (r *PlaySessionStatsReader) GetServerStats(ctx context.Context) (*activity.ServerStatsResult, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_session_stats.GetServerStats")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	r.serverStatsMu.Lock()
	defer r.serverStatsMu.Unlock()

	if r.serverStatsCache != nil && time.Now().Before(r.serverStatsTTL) {
		cached := *r.serverStatsCache
		// The rollups below are worth a 5-minute cache, the two live counters
		// are not: the dashboard polls every 3 s and showed "0 active streams"
		// beside two Now Playing cards, and the demo reset reads the same
		// count to decide whether someone is watching.
		if err := r.liveCounters(ctx, &cached); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		return &cached, nil
	}

	stats := &activity.ServerStatsResult{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT
			(SELECT COUNT(*) FROM play_sessions WHERE ended_at IS NULL),
			(SELECT COUNT(*) FROM transcode_sessions WHERE status IN ('queued', 'running')),
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM media_items),
			(SELECT COALESCE(SUM(viewed), 0) FROM (
				SELECT MAX(COALESCE(position_ms, duration_ms, 0)) AS viewed
				FROM play_sessions GROUP BY profile_id, media_item_id) v),
			(SELECT COUNT(*) FROM play_sessions),
			(SELECT COALESCE(SUM(file_size), 0) FROM media_files)`,
	).Scan(&stats.ActiveStreams, &stats.ActiveTranscodes, &stats.TotalUsers, &stats.TotalItems,
		&stats.TotalPlayTimeMs, &stats.TotalPlays, &stats.TotalStorageBytes)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if stats.PopularMedia, err = r.popularMediaRollup(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if stats.ActiveUsers, err = r.activeUsersRollup(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	r.serverStatsCache = stats
	r.serverStatsTTL = time.Now().Add(serverStatsCacheDuration)

	cached := *stats
	return &cached, nil
}

// liveCounters re-reads the running playbacks and transcodes, the two figures
// of the server stats that must never be served from the rollup cache.
func (r *PlaySessionStatsReader) liveCounters(ctx context.Context, stats *activity.ServerStatsResult) error {
	return txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT
			(SELECT COUNT(*) FROM play_sessions WHERE ended_at IS NULL),
			(SELECT COUNT(*) FROM transcode_sessions WHERE status IN ('queued', 'running'))`,
	).Scan(&stats.ActiveStreams, &stats.ActiveTranscodes)
}

// popularMediaRollup returns the most-played items of the last 90 days with
// their distinct-viewer counts (admin stats tab).
func (r *PlaySessionStatsReader) popularMediaRollup(ctx context.Context) ([]activity.ServerPopularMedia, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT ps.media_item_id, mi.title, mi.type,
		        COUNT(*) AS plays, COUNT(DISTINCT ps.profile_id) AS viewers
		 FROM play_sessions ps
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
		 WHERE ps.started_at >= NOW() - INTERVAL '90 days'
		 GROUP BY ps.media_item_id, mi.title, mi.type
		 ORDER BY plays DESC, mi.title
		 LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activity.ServerPopularMedia
	for rows.Next() {
		var m activity.ServerPopularMedia
		if err := rows.Scan(&m.MediaItemID, &m.Title, &m.Type, &m.PlayCount, &m.UniqueViewers); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// activeUsersRollup returns the most active users of the last 30 days
// (admin stats tab). Watch time = "Temps de visionnage" (content viewed):
// the playhead position, falling back to the session wall-clock when no
// position was reported. Position-first matches the other watch-time stats
// (it previously preferred duration, disagreeing with the dashboard/analytics
// and zero-counting stale sessions that never set duration_ms).
func (r *PlaySessionStatsReader) activeUsersRollup(ctx context.Context) ([]activity.ServerActiveUser, error) {
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT u.id, u.username, COUNT(*) AS plays,
		        COALESCE(SUM(COALESCE(ps.position_ms, ps.duration_ms, 0)), 0) AS watch_ms,
		        MAX(ps.started_at) AS last_active
		 FROM play_sessions ps
		 INNER JOIN profiles p ON p.id = ps.profile_id
		 INNER JOIN users u ON u.id = p.user_id
		 WHERE ps.started_at >= NOW() - INTERVAL '30 days'
		 GROUP BY u.id, u.username
		 ORDER BY plays DESC, u.username
		 LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activity.ServerActiveUser
	for rows.Next() {
		var u activity.ServerActiveUser
		if err := rows.Scan(&u.UserID, &u.Username, &u.PlayCount, &u.TotalWatchTimeMs, &u.LastActiveAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetActiveSessionsBitrate sums the source bitrate of the active sessions -
// the dashboard's theoretical outgoing rate. Uncached on purpose: the active
// set is tiny and the dashboard polls every few seconds.
func (r *PlaySessionStatsReader) GetActiveSessionsBitrate(ctx context.Context) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_session_stats.GetActiveSessionsBitrate")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var bps int64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COALESCE(SUM(bandwidth_bps), 0) FROM play_sessions WHERE ended_at IS NULL`,
	).Scan(&bps)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return bps, nil
}

// GetMostPopular returns the top-N most-played items in the last 90 days.
// When profileID is non-empty, items the profile has fully watched are
// filtered out (movies via wp.completed; series only when EVERY current
// episode has a completed wp row - a freshly-added unwatched episode
// re-surfaces the series). Pass an empty profileID for unfiltered output.
func (r *PlaySessionStatsReader) GetMostPopular(ctx context.Context, profileID string, limit int) ([]activity.MediaPlayCount, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_session_stats.GetMostPopular")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT ps.media_item_id, mi.title, COUNT(*) AS play_count
		 FROM play_sessions ps
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
		 WHERE mi.type IN ('movie', 'series')
		   AND ps.started_at >= NOW() - INTERVAL '90 days'
		   AND (
			$2 = ''
			OR (mi.type = 'movie' AND NOT COALESCE(
				(SELECT completed FROM watch_progress wp WHERE wp.media_item_id = mi.id AND wp.profile_id = $2 LIMIT 1),
				FALSE
			))
			OR (mi.type = 'series' AND EXISTS (
				SELECT 1
				FROM media_items se
				JOIN media_items ep ON ep.parent_id = se.id
				WHERE se.parent_id = mi.id
				  AND se.type = 'season'
				  AND ep.type = 'episode'
				  AND NOT EXISTS (
					SELECT 1 FROM watch_progress wp
					WHERE wp.media_item_id = ep.id
					  AND wp.profile_id = $2
					  AND wp.completed = TRUE
				  )
			))
		   )
		 GROUP BY ps.media_item_id, mi.title
		 ORDER BY play_count DESC
		 LIMIT $1`, limit, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []activity.MediaPlayCount
	for rows.Next() {
		var mpc activity.MediaPlayCount
		if err := rows.Scan(&mpc.MediaItemID, &mpc.Title, &mpc.PlayCount); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, mpc)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ activity.PlaySessionStatsReader = (*PlaySessionStatsReader)(nil)
