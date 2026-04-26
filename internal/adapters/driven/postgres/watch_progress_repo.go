package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO watch_progress (id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (profile_id, media_item_id) DO UPDATE SET
		   position_ms = EXCLUDED.position_ms,
		   completed = EXCLUDED.completed,
		   play_count = EXCLUDED.play_count,
		   last_played_at = EXCLUDED.last_played_at`,
		wp.ID, wp.ProfileID, wp.MediaItemID, wp.Position.Milliseconds(), wp.Completed, wp.PlayCount, wp.LastPlayedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("WatchProgressRepo.Upsert: %w", err)
	}
	return nil
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
	err := r.db.QueryRowContext(ctx,
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
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, position_ms, completed, play_count, last_played_at
		 FROM watch_progress WHERE profile_id = $1
		 ORDER BY last_played_at DESC LIMIT $2 OFFSET $3`, profileID, limit, offset)
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

	rows, err := r.db.QueryContext(ctx, query, args...)
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
	rows, err := r.db.QueryContext(ctx,
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
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT ON (mi.parent_id)
		   COALESCE(wp.id, ''), $1, mi.id, COALESCE(wp.position_ms, 0),
		   COALESCE(wp.completed, FALSE), COALESCE(wp.play_count, 0), COALESCE(wp.last_played_at, NOW())
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
