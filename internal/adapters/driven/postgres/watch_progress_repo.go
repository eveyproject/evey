package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// scanWatchProgressPosition converts the position_ms column (NOT NULL,
// see migration 015) into the domain shared.Duration. A negative stored
// value would indicate row corruption; we log and clamp to zero rather
// than failing the whole read so the rest of the watch history stays
// useful for the caller.
func scanWatchProgressPosition(wpID string, raw int64) shared.Duration {
	d, err := shared.NewDurationMs(raw)
	if err != nil {
		slog.Warn("invalid position_ms in watch_progress row", slog.String("watchProgressID", wpID), slog.Int64("ms", raw), slog.Any("err", err))
		return shared.Duration{}
	}
	return d
}

type WatchProgressRepository struct {
	db *sql.DB
}

func NewWatchProgressRepository(db *sql.DB) *WatchProgressRepository {
	return &WatchProgressRepository{db: db}
}

func (r *WatchProgressRepository) Upsert(ctx context.Context, wp *playback.WatchProgress) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.Upsert")
	defer span.End()
	span.SetAttributes(
		attribute.String("profile.id", string(wp.ProfileID)),
		attribute.String("media_item.id", string(wp.MediaItemID)),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Completed is monotonic-true on conflict (OR with existing) so a
	// background progress tick can't undo an explicit MarkAsWatched or a
	// previous auto-completion at 90%. MarkAsUnwatched goes through Delete,
	// not Upsert, when it needs to clear the flag.
	//
	// PlayCount semantic: increments only on the false→true completion
	// transition (so re-watching a finished episode bumps the count). For
	// any other update (mid-stream progress tick that doesn't flip the
	// flag), it takes max(existing, incoming) so callers passing
	// PlayCount=0 (the common ReportProgress* path) never regress the
	// counter. Callers wanting to set an absolute value still can - the
	// incoming PlayCount wins when it's higher.
	//
	// position_ms/last_played_at are guarded on FRESHNESS, not on position
	// value: a tick can only move the stored position if its last_played_at
	// is at least as recent as what's already stored. This lets a user
	// rewind on purpose (a fresher tick reporting a lower position still
	// wins) while rejecting a stale write - a late-arriving retry, or a
	// second device whose tick was in flight - that would otherwise drag an
	// already-more-advanced position backwards. last_played_at itself only
	// ever moves forward (GREATEST) so a late write can't even reset the
	// freshness clock for the next comparison.
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO watch_progress (id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (profile_id, media_item_id) DO UPDATE SET
		   position_ms = CASE
		     WHEN EXCLUDED.last_played_at >= watch_progress.last_played_at THEN EXCLUDED.position_ms
		     ELSE watch_progress.position_ms
		   END,
		   completed = (watch_progress.completed OR EXCLUDED.completed),
		   play_count = CASE
		     WHEN EXCLUDED.completed AND NOT watch_progress.completed THEN watch_progress.play_count + 1
		     ELSE GREATEST(watch_progress.play_count, EXCLUDED.play_count)
		   END,
		   last_played_at = GREATEST(watch_progress.last_played_at, EXCLUDED.last_played_at)`,
		wp.ID, wp.ProfileID, wp.MediaItemID, wp.Position.Milliseconds(), wp.Completed, wp.PlayCount, wp.LastPlayedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("WatchProgressRepo.Upsert: %w", err)
	}
	return nil
}

// UpsertMany writes a whole flush in ONE statement. Same conflict clauses as
// Upsert - the two must never drift, they implement the same contract.
func (r *WatchProgressRepository) UpsertMany(ctx context.Context, entries []*playback.WatchProgress) error {
	if len(entries) == 0 {
		return nil
	}
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.UpsertMany")
	defer span.End()
	span.SetAttributes(attribute.Int("entries", len(entries)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	ids := make([]string, len(entries))
	profiles := make([]string, len(entries))
	medias := make([]string, len(entries))
	positions := make([]int64, len(entries))
	completed := make([]bool, len(entries))
	playCounts := make([]int64, len(entries))
	lastPlayed := make([]time.Time, len(entries))
	for i, wp := range entries {
		ids[i] = wp.ID
		profiles[i] = string(wp.ProfileID)
		medias[i] = string(wp.MediaItemID)
		positions[i] = wp.Position.Milliseconds()
		completed[i] = wp.Completed
		playCounts[i] = int64(wp.PlayCount)
		lastPlayed[i] = wp.LastPlayedAt
	}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO watch_progress (id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at)
		 SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::bigint[], $5::boolean[], $6::bigint[], $7::timestamptz[])
		 ON CONFLICT (profile_id, media_item_id) DO UPDATE SET
		   position_ms = CASE
		     WHEN EXCLUDED.last_played_at >= watch_progress.last_played_at THEN EXCLUDED.position_ms
		     ELSE watch_progress.position_ms
		   END,
		   completed = (watch_progress.completed OR EXCLUDED.completed),
		   play_count = CASE
		     WHEN EXCLUDED.completed AND NOT watch_progress.completed THEN watch_progress.play_count + 1
		     ELSE GREATEST(watch_progress.play_count, EXCLUDED.play_count)
		   END,
		   last_played_at = GREATEST(watch_progress.last_played_at, EXCLUDED.last_played_at)`,
		pq.Array(ids), pq.Array(profiles), pq.Array(medias), pq.Array(positions),
		pq.BoolArray(completed), pq.Array(playCounts), pq.Array(lastPlayed),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("WatchProgressRepo.UpsertMany: %w", err)
	}
	return nil
}

func (r *WatchProgressRepository) Delete(ctx context.Context, profileID, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.Delete")
	defer span.End()
	span.SetAttributes(
		attribute.String("profile.id", profileID),
		attribute.String("media_item.id", mediaItemID),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM watch_progress WHERE profile_id = $1 AND media_item_id = $2`,
		profileID, mediaItemID,
	); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("WatchProgressRepo.Delete: %w", err)
	}
	return nil
}

// DeleteByProfile removes every watch-progress row for the profile (the
// "clear watch history" action). Returns how many rows were deleted.
func (r *WatchProgressRepository) DeleteByProfile(ctx context.Context, profileID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.DeleteByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM watch_progress WHERE profile_id = $1`, profileID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("WatchProgressRepo.DeleteByProfile: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *WatchProgressRepository) GetByProfileAndMedia(ctx context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.GetByProfileAndMedia")
	defer span.End()
	span.SetAttributes(
		attribute.String("profile.id", profileID),
		attribute.String("media_item.id", mediaItemID),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	wp := &playback.WatchProgress{}
	var positionMs int64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at
		 FROM watch_progress WHERE profile_id = $1 AND media_item_id = $2`, profileID, mediaItemID,
	).Scan(&wp.ID, &wp.ProfileID, &wp.MediaItemID, &positionMs, &wp.Completed, &wp.PlayCount, &wp.LastPlayedAt)
	if errors.Is(err, sql.ErrNoRows) {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("WatchProgressRepo.GetByProfileAndMedia: %w", err)
	}
	wp.Position = scanWatchProgressPosition(wp.ID, positionMs)
	return wp, nil
}

func (r *WatchProgressRepository) GetByProfile(ctx context.Context, profileID string, limit, offset int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.GetByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at
		 FROM watch_progress WHERE profile_id = $1
		 ORDER BY last_played_at DESC, id DESC LIMIT $2 OFFSET $3`, profileID, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("WatchProgressRepo.GetByProfile: %w", err)
	}
	defer rows.Close()
	progress, err := r.scanWatchProgressRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return progress, nil
}

func (r *WatchProgressRepository) GetByProfileCursor(ctx context.Context, profileID string, cursor *playback.WatchHistoryCursor, limit int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.GetByProfileCursor")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query := `SELECT id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at
		 FROM watch_progress WHERE profile_id = $1`
	args := []any{profileID}
	i := 2
	if cursor != nil {
		query += fmt.Sprintf(" AND (last_played_at, media_item_id) < ($%d, $%d)", i, i+1)
		args = append(args, cursor.LastPlayedAt, cursor.MediaItemID)
		i += 2
	}
	query += fmt.Sprintf(" ORDER BY last_played_at DESC, media_item_id DESC LIMIT $%d", i)
	args = append(args, limit)

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("WatchProgressRepo.GetByProfileCursor: %w", err)
	}
	defer rows.Close()
	progress, err := r.scanWatchProgressRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return progress, nil
}

func (r *WatchProgressRepository) GetResumeItems(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.GetResumeItems")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at
		 FROM watch_progress WHERE profile_id = $1 AND completed = FALSE AND position_ms > 0
		 ORDER BY last_played_at DESC LIMIT $2`, profileID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("WatchProgressRepo.GetResumeItems: %w", err)
	}
	defer rows.Close()
	progress, err := r.scanWatchProgressRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return progress, nil
}

// GetNextUp returns the next unwatched episode for each series the profile has been watching.
// It finds episodes where the previous episode (index_number - 1) was completed,
// but the current episode has not been completed yet.
func (r *WatchProgressRepository) GetNextUp(ctx context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.GetNextUp")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// DISTINCT ON (parent_id) must order by parent_id first, so its rows come out
	// in series-id order. Wrap it and re-order the de-duplicated set by how
	// recently each series was watched, applying LIMIT in the outer query so the
	// N most-recently-watched series surface (the inner LIMIT would instead keep
	// the N alphabetically-first series and could drop the one you just watched).
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT nu.id, nu.profile_id, nu.media_item_id, nu.position_ms,
		        nu.completed, nu.play_count, nu.last_played_at
		 FROM (
		   SELECT DISTINCT ON (mi.parent_id)
		     COALESCE(wp.id, '') AS id, $1 AS profile_id, mi.id AS media_item_id,
		     COALESCE(wp.position_ms, 0) AS position_ms,
		     COALESCE(wp.completed, FALSE) AS completed,
		     COALESCE(wp.play_count, 0) AS play_count,
		     COALESCE(wp.last_played_at, NOW()) AS last_played_at,
		     COALESCE(prev_wp.last_played_at, NOW()) AS prev_last_played
		   FROM media_items mi
		   INNER JOIN media_items prev ON prev.parent_id = mi.parent_id
		     AND prev.index_number = mi.index_number - 1
		   INNER JOIN watch_progress prev_wp ON prev_wp.profile_id = $1
		     AND prev_wp.media_item_id = prev.id
		     AND prev_wp.completed = TRUE
		   LEFT JOIN watch_progress wp ON wp.profile_id = $1 AND wp.media_item_id = mi.id
		   WHERE mi.type = 'episode' AND mi.index_number IS NOT NULL
		     AND (wp.completed IS NULL OR wp.completed = FALSE)
		   ORDER BY mi.parent_id, COALESCE(prev_wp.last_played_at, NOW()) DESC
		 ) nu
		 ORDER BY nu.prev_last_played DESC, nu.media_item_id
		 LIMIT $2`, profileID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("WatchProgressRepo.GetNextUp: %w", err)
	}
	defer rows.Close()
	progress, err := r.scanWatchProgressRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return progress, nil
}

// ListSeriesEpisodes returns every episode of a series in viewing order -
// season, then episode, specials (season 0) last - with the profile's
// progress on each; an episode never played has a zero LastPlayedAt.
func (r *WatchProgressRepository) ListSeriesEpisodes(ctx context.Context, profileID, seriesID string) ([]playback.SeriesEpisode, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.watchprogress.ListSeriesEpisodes")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.String("series.id", seriesID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.id, COALESCE(wp.position_ms, 0), COALESCE(wp.completed, FALSE), wp.last_played_at
		 FROM media_items sea
		 INNER JOIN media_items mi ON mi.parent_id = sea.id AND mi.type = 'episode'
		 LEFT JOIN watch_progress wp ON wp.profile_id = $1 AND wp.media_item_id = mi.id
		 WHERE sea.parent_id = $2 AND sea.type = 'season'
		 ORDER BY (sea.index_number = 0), sea.index_number NULLS LAST,
		          mi.index_number NULLS LAST, mi.title, mi.id`, profileID, seriesID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("WatchProgressRepo.ListSeriesEpisodes: %w", err)
	}
	defer rows.Close()
	var episodes []playback.SeriesEpisode
	for rows.Next() {
		var e playback.SeriesEpisode
		var positionMs int64
		var played sql.NullTime
		if err := rows.Scan(&e.MediaItemID, &positionMs, &e.Completed, &played); err != nil {
			return nil, fmt.Errorf("WatchProgressRepo.ListSeriesEpisodes: %w", err)
		}
		e.Position = scanWatchProgressPosition(string(e.MediaItemID), positionMs)
		if played.Valid {
			e.LastPlayedAt = played.Time
		}
		episodes = append(episodes, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("WatchProgressRepo.ListSeriesEpisodes: %w", err)
	}
	return episodes, nil
}

func (r *WatchProgressRepository) scanWatchProgressRows(rows *sql.Rows) ([]*playback.WatchProgress, error) {
	var result []*playback.WatchProgress
	for rows.Next() {
		wp := &playback.WatchProgress{}
		var positionMs int64
		if err := rows.Scan(&wp.ID, &wp.ProfileID, &wp.MediaItemID, &positionMs, &wp.Completed, &wp.PlayCount, &wp.LastPlayedAt); err != nil {
			return nil, fmt.Errorf("WatchProgressRepo.scanRows: %w", err)
		}
		wp.Position = scanWatchProgressPosition(wp.ID, positionMs)
		result = append(result, wp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("WatchProgressRepo.scanRows: %w", err)
	}
	return result, nil
}

var _ playback.ProgressCommandRepo = (*WatchProgressRepository)(nil)
var _ playback.ProgressQueryRepo = (*WatchProgressRepository)(nil)
