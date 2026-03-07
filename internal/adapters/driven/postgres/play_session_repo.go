package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/playback"
)

type PlaySessionRepository struct {
	db *sql.DB
}

func NewPlaySessionRepository(db *sql.DB) *PlaySessionRepository {
	return &PlaySessionRepository{db: db}
}

func (r *PlaySessionRepository) Create(ctx context.Context, s *playback.PlaySession) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO play_sessions (id, profile_id, media_item_id, device_name, device_type, client_ip,
		 play_method, video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		s.ID, s.ProfileID, s.MediaItemID, s.DeviceName, s.DeviceType, s.ClientIP,
		s.PlayMethod, s.VideoCodec, s.AudioCodec, s.Resolution, s.BandwidthBps,
		s.StartedAt, s.EndedAt, s.DurationMs, s.PositionMs,
	)
	return err
}

func (r *PlaySessionRepository) Update(ctx context.Context, s *playback.PlaySession) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE play_sessions SET device_name=$1, device_type=$2, client_ip=$3, play_method=$4,
		 video_codec=$5, audio_codec=$6, resolution=$7, bandwidth_bps=$8, ended_at=$9, duration_ms=$10, position_ms=$11
		 WHERE id=$12`,
		s.DeviceName, s.DeviceType, s.ClientIP, s.PlayMethod,
		s.VideoCodec, s.AudioCodec, s.Resolution, s.BandwidthBps,
		s.EndedAt, s.DurationMs, s.PositionMs, s.ID,
	)
	return err
}

func (r *PlaySessionRepository) GetByID(ctx context.Context, id string) (*playback.PlaySession, error) {
	s := &playback.PlaySession{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms
		 FROM play_sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
		&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
		&s.StartedAt, &s.EndedAt, &s.DurationMs, &s.PositionMs)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *PlaySessionRepository) GetActiveByProfile(ctx context.Context, profileID string) ([]*playback.PlaySession, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms
		 FROM play_sessions WHERE profile_id = $1 AND ended_at IS NULL
		 ORDER BY started_at DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanPlaySessionRows(rows)
}

func (r *PlaySessionRepository) GetHistory(ctx context.Context, f playback.PlaySessionFilter) ([]*playback.PlaySession, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if f.ProfileID != nil {
		conditions = append(conditions, fmt.Sprintf("profile_id = $%d", argIdx))
		args = append(args, *f.ProfileID)
		argIdx++
	}
	if f.StartFrom != nil {
		conditions = append(conditions, fmt.Sprintf("started_at >= $%d", argIdx))
		args = append(args, *f.StartFrom)
		argIdx++
	}
	if f.StartTo != nil {
		conditions = append(conditions, fmt.Sprintf("started_at <= $%d", argIdx))
		args = append(args, *f.StartTo)
		argIdx++
	}

	query := `SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms
		 FROM play_sessions`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY started_at DESC"

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanPlaySessionRows(rows)
}

func (r *PlaySessionRepository) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM play_sessions ps
		 INNER JOIN profiles p ON p.id = ps.profile_id
		 WHERE p.user_id = $1 AND ps.ended_at IS NULL`, userID,
	).Scan(&count)
	return count, err
}

func (r *PlaySessionRepository) scanPlaySessionRows(rows *sql.Rows) ([]*playback.PlaySession, error) {
	var result []*playback.PlaySession
	for rows.Next() {
		s := &playback.PlaySession{}
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
			&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
			&s.StartedAt, &s.EndedAt, &s.DurationMs, &s.PositionMs); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
