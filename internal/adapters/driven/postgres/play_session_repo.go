package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/playback"
)

type PlaySessionRepository struct {
	db *sql.DB
}

func NewPlaySessionRepository(db *sql.DB) *PlaySessionRepository {
	return &PlaySessionRepository{db: db}
}

func (r *PlaySessionRepository) Create(ctx context.Context, s *playback.PlaySession) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO play_sessions (id, profile_id, media_item_id, device_name, device_type, client_ip,
		 play_method, video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		s.ID, s.ProfileID, s.MediaItemID, s.DeviceName, s.DeviceType, s.ClientIP,
		s.PlayMethod, s.VideoCodec, s.AudioCodec, s.Resolution, s.BandwidthBps,
		s.StartedAt, s.EndedAt, s.DurationMs, s.PositionMs,
	)
	if err != nil {
		return fmt.Errorf("PlaySessionRepo.Create: %w", err)
	}
	return nil
}

// AcquireUserStreamLock takes a tx-scoped Postgres advisory lock keyed by
// hashtext(userID). Serializes the MaxStreams quota check+Create pair so
// concurrent StartStream calls can't race past the cap. Outside a tx the
// lock is released at statement end — so caller MUST run inside TxUnit.
func (r *PlaySessionRepository) AcquireUserStreamLock(ctx context.Context, userID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1))`, userID,
	); err != nil {
		return fmt.Errorf("PlaySessionRepo.AcquireUserStreamLock: %w", err)
	}
	return nil
}

func (r *PlaySessionRepository) Update(ctx context.Context, s *playback.PlaySession) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE play_sessions SET device_name=$1, device_type=$2, client_ip=$3, play_method=$4,
		 video_codec=$5, audio_codec=$6, resolution=$7, bandwidth_bps=$8, ended_at=$9, duration_ms=$10, position_ms=$11,
		 is_paused=$12, updated_at=NOW()
		 WHERE id=$13`,
		s.DeviceName, s.DeviceType, s.ClientIP, s.PlayMethod,
		s.VideoCodec, s.AudioCodec, s.Resolution, s.BandwidthBps,
		s.EndedAt, s.DurationMs, s.PositionMs, s.IsPaused, s.ID,
	)
	if err != nil {
		return fmt.Errorf("PlaySessionRepo.Update: %w", err)
	}
	return nil
}

func (r *PlaySessionRepository) GetByID(ctx context.Context, id string) (*playback.PlaySession, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	s := &playback.PlaySession{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms, is_paused
		 FROM play_sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
		&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
		&s.StartedAt, &s.EndedAt, &s.DurationMs, &s.PositionMs, &s.IsPaused)
	if err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.GetByID: %w", err)
	}
	return s, nil
}

func (r *PlaySessionRepository) ListActiveSessions(ctx context.Context) ([]*playback.ActiveSessionInfo, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT ps.id, ps.profile_id, p.name, ps.media_item_id, mi.title, mi.type,
		        COALESCE(mf.duration_ms, 0), COALESCE(ps.position_ms, 0),
		        ps.started_at, ps.device_name, ps.device_type, ps.client_ip,
		        ps.play_method, ps.video_codec, ps.audio_codec, ps.resolution, ps.bandwidth_bps,
		        COALESCE(series.title, ''), COALESCE(season.index_number, 0), COALESCE(mi.index_number, 0),
		        ps.is_paused
		 FROM play_sessions ps
		 INNER JOIN profiles p ON p.id = ps.profile_id
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
		 LEFT JOIN media_files mf ON mf.media_item_id = mi.id
		 LEFT JOIN media_items season ON mi.parent_id = season.id AND mi.type = 'episode'
		 LEFT JOIN media_items series ON season.parent_id = series.id AND season.type = 'season'
		 WHERE ps.ended_at IS NULL
		   AND ps.updated_at > NOW() - INTERVAL '5 minutes'
		 ORDER BY ps.started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.ListActiveSessions: %w", err)
	}
	defer rows.Close()

	var result []*playback.ActiveSessionInfo
	for rows.Next() {
		s := &playback.ActiveSessionInfo{}
		if err := rows.Scan(
			&s.SessionID, &s.ProfileID, &s.ProfileName,
			&s.MediaItemID, &s.MediaTitle, &s.MediaType,
			&s.DurationMs, &s.PositionMs,
			&s.StartedAt, &s.DeviceName, &s.DeviceType, &s.ClientIP,
			&s.PlayMethod, &s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
			&s.SeriesTitle, &s.SeasonNumber, &s.EpisodeNumber, &s.IsPaused,
		); err != nil {
			return nil, fmt.Errorf("PlaySessionRepo.ListActiveSessions scan: %w", err)
		}
		if s.PlayMethod == string(playback.Transcode) {
			info := fmt.Sprintf("%s -> %s", s.VideoCodec, "h264")
			s.TranscodeInfo = &info
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.ListActiveSessions rows: %w", err)
	}
	return result, nil
}

func (r *PlaySessionRepository) GetActiveByProfile(ctx context.Context, profileID string) ([]*playback.PlaySession, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms
		 FROM play_sessions WHERE profile_id = $1 AND ended_at IS NULL
		 ORDER BY started_at DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.GetActiveByProfile: %w", err)
	}
	defer rows.Close()
	return r.scanPlaySessionRows(rows)
}

func (r *PlaySessionRepository) GetHistory(ctx context.Context, f playback.PlaySessionFilter) ([]*playback.PlaySession, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var conditions []string
	var args []any
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
	}

	query := `SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms
		 FROM play_sessions`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY started_at DESC"

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.GetHistory: %w", err)
	}
	defer rows.Close()
	return r.scanPlaySessionRows(rows)
}

func (r *PlaySessionRepository) CloseStale(ctx context.Context, olderThan time.Duration) (int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	cutoff := time.Now().Add(-olderThan)
	res, err := r.db.ExecContext(ctx,
		`UPDATE play_sessions SET ended_at = NOW()
		 WHERE ended_at IS NULL AND updated_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("PlaySessionRepo.CloseStale: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	return int(n), nil
}

func (r *PlaySessionRepository) SetRemoteCommand(ctx context.Context, sessionID, command string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE play_sessions SET remote_command = $1 WHERE id = $2 AND ended_at IS NULL`, command, sessionID)
	if err != nil {
		return fmt.Errorf("PlaySessionRepo.SetRemoteCommand: %w", err)
	}
	return nil
}

func (r *PlaySessionRepository) ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var cmd string
	err := r.db.QueryRowContext(ctx,
		`WITH old AS (SELECT remote_command FROM play_sessions WHERE id = $1 AND remote_command != '')
		 UPDATE play_sessions SET remote_command = '' WHERE id = $1 AND remote_command != ''
		 RETURNING (SELECT remote_command FROM old)`, sessionID).Scan(&cmd)
	if err != nil {
		return "", nil
	}
	return cmd, nil
}

func (r *PlaySessionRepository) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM play_sessions ps
		 INNER JOIN profiles p ON p.id = ps.profile_id
		 WHERE p.user_id = $1 AND ps.ended_at IS NULL`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("PlaySessionRepo.CountActiveByUser: %w", err)
	}
	return count, nil
}

func (r *PlaySessionRepository) scanPlaySessionRows(rows *sql.Rows) ([]*playback.PlaySession, error) {
	var result []*playback.PlaySession
	for rows.Next() {
		s := &playback.PlaySession{}
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
			&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
			&s.StartedAt, &s.EndedAt, &s.DurationMs, &s.PositionMs); err != nil {
			return nil, fmt.Errorf("PlaySessionRepo.scanRows: %w", err)
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.scanRows: %w", err)
	}
	return result, nil
}

var _ playback.PlaySessionRepository = (*PlaySessionRepository)(nil)
