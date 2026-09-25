package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type PlayHistoryRepository struct {
	db *sql.DB
}

func NewPlayHistoryRepository(db *sql.DB) *PlayHistoryRepository {
	return &PlayHistoryRepository{db: db}
}

func (r *PlayHistoryRepository) List(ctx context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_history.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// The profile/user joins stay LEFT: a session outlives the profile that
	// created it only until the ON DELETE CASCADE fires, but history read
	// inside that window must still list the row rather than drop it.
	query := `
		SELECT ps.id, ps.profile_id, ps.media_item_id,
		       CASE WHEN mi.type = 'episode' THEN
		         COALESCE(series.title, '') || ' - S' ||
		         LPAD(COALESCE(season.index_number, 0)::text, 2, '0') || 'E' ||
		         LPAD(COALESCE(mi.index_number, 0)::text, 2, '0')
		       ELSE mi.title END,
		       mi.type,
		       COALESCE(mf.duration_ms, 0), COALESCE(ps.position_ms, 0),
		       COALESCE(wp.completed, false),
		       ps.device_name, ps.device_type, ps.play_method,
		       ps.started_at, ps.ended_at,
		       COALESCE(p.name, ''), COALESCE(p.user_id, ''), COALESCE(u.username, '')
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		-- An episode reads as its series and code, as in the audit log.
		LEFT JOIN media_items season ON season.id = mi.parent_id AND mi.type = 'episode'
		LEFT JOIN media_items series ON series.id = season.parent_id AND mi.type = 'episode'
		LEFT JOIN media_files mf ON mf.media_item_id = mi.id
		LEFT JOIN watch_progress wp ON wp.profile_id = ps.profile_id AND wp.media_item_id = ps.media_item_id
		LEFT JOIN profiles p ON p.id = ps.profile_id
		LEFT JOIN users u ON u.id = p.user_id
		WHERE 1=1`

	args := []any{}
	i := 1

	if filter.ProfileID != "" {
		query += fmt.Sprintf(" AND ps.profile_id = $%d", i)
		args = append(args, filter.ProfileID)
		i++
	}
	if filter.UserID != "" {
		query += fmt.Sprintf(" AND p.user_id = $%d", i)
		args = append(args, filter.UserID)
		i++
	}
	if filter.MediaItemID != "" {
		query += fmt.Sprintf(" AND ps.media_item_id = $%d", i)
		args = append(args, filter.MediaItemID)
		i++
	}
	if filter.From != nil {
		query += fmt.Sprintf(" AND ps.started_at >= $%d", i)
		args = append(args, *filter.From)
		i++
	}
	if filter.To != nil {
		query += fmt.Sprintf(" AND ps.started_at <= $%d", i)
		args = append(args, *filter.To)
		i++
	}
	if filter.Cursor != nil {
		query += fmt.Sprintf(" AND (ps.started_at, ps.id) < ($%d, $%d)", i, i+1)
		args = append(args, filter.Cursor.StartedAt, filter.Cursor.SessionID)
		i += 2
	}

	query += " ORDER BY ps.started_at DESC, ps.id DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT $%d", i)
	args = append(args, limit)
	i++
	if filter.Cursor == nil && filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", i)
		args = append(args, filter.Offset)
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*activity.PlayHistoryEntry
	for rows.Next() {
		e := &activity.PlayHistoryEntry{}
		var rawDurationMs, rawPositionMs int64
		if err := rows.Scan(
			&e.ID, &e.ProfileID, &e.MediaItemID,
			&e.MediaTitle, &e.MediaType,
			&rawDurationMs, &rawPositionMs,
			&e.Completed,
			&e.DeviceName, &e.ClientName, &e.PlayMethod,
			&e.StartedAt, &e.EndedAt,
			&e.ProfileName, &e.UserID, &e.Username,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if d, err := shared.NewDurationMs(rawPositionMs); err == nil {
			e.Position = d
		}
		if d, err := shared.NewDurationMs(rawDurationMs); err == nil {
			e.Duration = d
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// Count returns the total sessions matching the filter predicates (same INNER
// JOIN as List so the total matches what List can return), ignoring
// Limit/Offset/Cursor.
func (r *PlayHistoryRepository) Count(ctx context.Context, filter activity.PlayHistoryFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.play_history.Count")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	query := `SELECT COUNT(*) FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		LEFT JOIN profiles p ON p.id = ps.profile_id
		WHERE 1=1`
	args := []any{}
	i := 1
	if filter.ProfileID != "" {
		query += fmt.Sprintf(" AND ps.profile_id = $%d", i)
		args = append(args, filter.ProfileID)
		i++
	}
	if filter.UserID != "" {
		query += fmt.Sprintf(" AND p.user_id = $%d", i)
		args = append(args, filter.UserID)
		i++
	}
	if filter.MediaItemID != "" {
		query += fmt.Sprintf(" AND ps.media_item_id = $%d", i)
		args = append(args, filter.MediaItemID)
		i++
	}
	if filter.From != nil {
		query += fmt.Sprintf(" AND ps.started_at >= $%d", i)
		args = append(args, *filter.From)
		i++
	}
	if filter.To != nil {
		query += fmt.Sprintf(" AND ps.started_at <= $%d", i)
		args = append(args, *filter.To)
	}
	var n int
	if err := txFromCtx(ctx, r.db).QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return n, nil
}

var _ activity.PlayHistoryRepository = (*PlayHistoryRepository)(nil)
