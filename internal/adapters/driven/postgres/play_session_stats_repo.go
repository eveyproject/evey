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
	const q = `
	WITH totals AS (
		SELECT COALESCE(SUM(duration_ms), 0) AS total_ms,
		       COUNT(*) AS total_count,
		       COUNT(DISTINCT media_item_id) AS unique_items
		FROM play_sessions WHERE profile_id = $1
	),
	genres AS (
		SELECT g.genre,
		       COALESCE(SUM(ps.duration_ms), 0) AS total_ms,
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
		       COALESCE(SUM(ps.duration_ms), 0) AS total_ms
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
		       COALESCE(SUM(duration_ms), 0) AS total_ms
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

	rows, err := r.db.QueryContext(ctx, q, profileID)
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	r.serverStatsCache = stats
	r.serverStatsTTL = time.Now().Add(serverStatsCacheDuration)

	cached := *stats
	return &cached, nil
}

func (r *PlaySessionStatsReader) GetMostPopular(ctx context.Context, limit int) ([]activity.MediaPlayCount, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_session_stats.GetMostPopular")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
