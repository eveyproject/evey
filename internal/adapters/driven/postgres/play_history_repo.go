package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type PlayHistoryRepository struct {
	db *sql.DB
}

func NewPlayHistoryRepository(db *sql.DB) *PlayHistoryRepository {
	return &PlayHistoryRepository{db: db}
}

func (r *PlayHistoryRepository) List(ctx context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	query := `
		SELECT ps.id, ps.profile_id, ps.media_item_id,
		       mi.title, mi.type,
		       COALESCE(mi.duration_ms, 0), COALESCE(ps.position_ms, 0),
		       COALESCE(wp.completed, false),
		       ps.device_name, ps.device_type, ps.play_method,
		       ps.started_at, ps.ended_at
		FROM play_sessions ps
		INNER JOIN media_items mi ON mi.id = ps.media_item_id
		LEFT JOIN watch_progress wp ON wp.profile_id = ps.profile_id AND wp.media_item_id = ps.media_item_id
		WHERE 1=1`

	args := []any{}
	i := 1

	if filter.ProfileID != "" {
		query += fmt.Sprintf(" AND ps.profile_id = $%d", i)
		args = append(args, filter.ProfileID)
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

	query += " ORDER BY ps.started_at DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT $%d", i)
	args = append(args, limit)
	i++
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", i)
		args = append(args, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*activity.PlayHistoryEntry
	for rows.Next() {
		e := &activity.PlayHistoryEntry{}
		if err := rows.Scan(
			&e.ID, &e.ProfileID, &e.MediaItemID,
			&e.MediaTitle, &e.MediaType,
			&e.DurationMs, &e.PositionMs,
			&e.Completed,
			&e.DeviceName, &e.ClientName, &e.PlayMethod,
			&e.StartedAt, &e.EndedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

var _ activity.PlayHistoryRepository = (*PlayHistoryRepository)(nil)
