package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PlaybackQoEReader serves the admin dashboard. Reads are append-only on a
// log-shaped table - no ORM, just hand-written aggregations that match the
// partial index on (ts DESC) WHERE buffer_starved = TRUE.
//
// All queries take a Window so the dashboard can pivot between "last 24h"
// (live ops view) and "last 7d" (per-title learning view).
type PlaybackQoEReader struct {
	db *sql.DB
}

func NewPlaybackQoEReader(db *sql.DB) *PlaybackQoEReader {
	return &PlaybackQoEReader{db: db}
}

// QoESummary captures the headline numbers for the dashboard top bar.
type QoESummary struct {
	WindowLabel    string // "24h" / "7d"
	TotalEvents    int64
	StarvedEvents  int64
	StarvedRatio   float64 // 0..1
	UniqueSessions int64
	UniqueMedia    int64
}

// QoEStarvedTitle is one row of "top N titles where viewers struggled".
// Title + FilePath are JOINed in from media_items so the admin UI can
// label the row instead of just showing a UUID.
type QoEStarvedTitle struct {
	MediaItemID   string
	Title         string
	FilePath      string
	StarvedEvents int64
	TotalEvents   int64
	StarvedRatio  float64
	LastStarvedAt time.Time
}

// QoEByDevice splits the starvation rate by detected client class.
type QoEByDevice struct {
	DeviceClass    string
	TotalEvents    int64
	StarvedEvents  int64
	StarvedRatio   float64
	AvgBufferMs    float64
	AvgEncodedKbps float64
}

// GetSummary returns the headline counters for the given window.
func (r *PlaybackQoEReader) GetSummary(ctx context.Context, window time.Duration) (*QoESummary, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.qoe.GetSummary")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `
		SELECT
			COUNT(*)                                       AS total,
			COUNT(*) FILTER (WHERE buffer_starved = TRUE)  AS starved,
			COUNT(DISTINCT NULLIF(session_id, ''))         AS sessions,
			COUNT(DISTINCT media_item_id)                  AS media
		FROM playback_qoe_events
		WHERE ts >= NOW() - $1::interval`
	out := &QoESummary{WindowLabel: humanWindow(window)}
	if err := txFromCtx(ctx, r.db).QueryRowContext(ctx, q, intervalArg(window)).Scan(
		&out.TotalEvents, &out.StarvedEvents, &out.UniqueSessions, &out.UniqueMedia,
	); err != nil {
		return nil, fmt.Errorf("qoe summary: %w", err)
	}
	if out.TotalEvents > 0 {
		out.StarvedRatio = float64(out.StarvedEvents) / float64(out.TotalEvents)
	}
	return out, nil
}

// GetTopStarvedTitles returns the top N media items by starved-event ratio.
// Min volume guard (HAVING COUNT(*) >= 50) prevents a 1-event movie at 100%
// from polluting the list.
func (r *PlaybackQoEReader) GetTopStarvedTitles(ctx context.Context, window time.Duration, limit int) ([]QoEStarvedTitle, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.qoe.GetTopStarvedTitles")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// LEFT JOIN media_items so a starved row with no matching media
	// (orphaned event after a library cleanup) still shows up with
	// empty title/path rather than disappearing - admin can still see
	// the ratio and act on it.
	const q = `
		WITH agg AS (
			SELECT
				media_item_id,
				COUNT(*) FILTER (WHERE buffer_starved = TRUE) AS starved,
				COUNT(*)                                       AS total,
				MAX(ts) FILTER (WHERE buffer_starved = TRUE)   AS last_starved_at
			FROM playback_qoe_events
			WHERE ts >= NOW() - $1::interval
			  AND media_item_id IS NOT NULL
			GROUP BY media_item_id
			HAVING COUNT(*) >= 50
			   AND COUNT(*) FILTER (WHERE buffer_starved = TRUE) > 0
		)
		SELECT
			a.media_item_id::text,
			COALESCE(mi.title, '') AS title,
			COALESCE(mf.file_path, '') AS file_path,
			a.starved,
			a.total,
			a.last_starved_at,
			COALESCE(mi.type, ''), COALESCE(series.title, ''),
			COALESCE(season.index_number, 0), COALESCE(mi.index_number, 0)
		FROM agg a
		LEFT JOIN media_items mi ON mi.id = a.media_item_id
		LEFT JOIN media_items season ON season.id = mi.parent_id AND mi.type = 'episode'
		LEFT JOIN media_items series ON series.id = season.parent_id AND mi.type = 'episode'
		LEFT JOIN media_files mf ON mf.media_item_id = a.media_item_id
		ORDER BY (a.starved)::float / NULLIF(a.total, 0) DESC, a.starved DESC
		LIMIT $2`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, intervalArg(window), limit)
	if err != nil {
		return nil, fmt.Errorf("qoe top starved: %w", err)
	}
	defer rows.Close()

	out := make([]QoEStarvedTitle, 0, limit)
	for rows.Next() {
		var r QoEStarvedTitle
		var lastStarvedAt sql.NullTime
		var mediaType, seriesTitle string
		var seasonNum, episodeNum int
		if err := rows.Scan(&r.MediaItemID, &r.Title, &r.FilePath, &r.StarvedEvents, &r.TotalEvents, &lastStarvedAt, &mediaType, &seriesTitle, &seasonNum, &episodeNum); err != nil {
			return nil, fmt.Errorf("qoe top starved scan: %w", err)
		}
		if mediaType == "episode" {
			r.Title = library.EpisodeLabel(seriesTitle, seasonNum, episodeNum, r.Title)
		}
		if lastStarvedAt.Valid {
			r.LastStarvedAt = lastStarvedAt.Time
		}
		if r.TotalEvents > 0 {
			r.StarvedRatio = float64(r.StarvedEvents) / float64(r.TotalEvents)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetByDeviceClass splits the same window by the device class we derive at
// ingest. Empty/unknown ends up in the "unknown" bucket so totals match.
func (r *PlaybackQoEReader) GetByDeviceClass(ctx context.Context, window time.Duration) ([]QoEByDevice, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.qoe.GetByDeviceClass")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	const q = `
		SELECT
			COALESCE(NULLIF(device_class, ''), 'unknown') AS device,
			COUNT(*)                                       AS total,
			COUNT(*) FILTER (WHERE buffer_starved = TRUE)  AS starved,
			COALESCE(AVG(buffer_length_ms), 0)             AS avg_buf_ms,
			COALESCE(AVG(encoded_kbps), 0)                 AS avg_kbps
		FROM playback_qoe_events
		WHERE ts >= NOW() - $1::interval
		GROUP BY device
		ORDER BY total DESC`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, intervalArg(window))
	if err != nil {
		return nil, fmt.Errorf("qoe by device: %w", err)
	}
	defer rows.Close()

	var out []QoEByDevice
	for rows.Next() {
		var r QoEByDevice
		if err := rows.Scan(&r.DeviceClass, &r.TotalEvents, &r.StarvedEvents, &r.AvgBufferMs, &r.AvgEncodedKbps); err != nil {
			return nil, fmt.Errorf("qoe by device scan: %w", err)
		}
		if r.TotalEvents > 0 {
			r.StarvedRatio = float64(r.StarvedEvents) / float64(r.TotalEvents)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// intervalArg converts a Go Duration into the Postgres-compatible string for
// `interval` cast. We can't bind a Duration directly - pg's interval parser
// only takes strings or numeric seconds.
func intervalArg(d time.Duration) string {
	if d <= 0 {
		d = 24 * time.Hour
	}
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}

func humanWindow(d time.Duration) string {
	switch {
	case d >= 7*24*time.Hour:
		return "7d"
	case d >= 24*time.Hour:
		return "24h"
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}
