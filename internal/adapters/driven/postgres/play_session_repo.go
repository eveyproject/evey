package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// positionMsArg adapts a *shared.Duration to the int64-or-NULL value the
// position_ms column accepts. Returns nil for a nil position.
func positionMsArg(p *shared.Duration) any {
	if p == nil {
		return nil
	}
	return p.Milliseconds()
}

// scanPositionMs converts a nullable int64 read from position_ms into the
// domain *shared.Duration. A negative stored value would be a corrupt row;
// log and drop it (returning nil) rather than failing the whole read.
func scanPositionMs(sessionID string, raw *int64) *shared.Duration {
	if raw == nil {
		return nil
	}
	d, err := shared.NewDurationMs(*raw)
	if err != nil {
		slog.Warn("invalid position_ms in play_sessions row", slog.String("sessionID", sessionID), slog.Int64("ms", *raw), slog.Any("err", err))
		return nil
	}
	return &d
}

type PlaySessionRepository struct {
	db *sql.DB
}

func NewPlaySessionRepository(db *sql.DB) *PlaySessionRepository {
	return &PlaySessionRepository{db: db}
}

func (r *PlaySessionRepository) Create(ctx context.Context, s *playback.PlaySession) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.Create")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", s.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO play_sessions (id, profile_id, media_item_id, device_name, device_type, client_ip,
		 play_method, video_codec, audio_codec, resolution, video_decision, audio_decision, bandwidth_bps, started_at, ended_at, duration_ms, position_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		s.ID, s.ProfileID, s.MediaItemID, s.DeviceName, s.DeviceType, s.ClientIP,
		s.PlayMethod, s.VideoCodec, s.AudioCodec, s.Resolution, s.VideoDecision, s.AudioDecision, s.BandwidthBps,
		s.StartedAt, s.EndedAt, durationMsArg(s.Duration), positionMsArg(s.Position),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaySessionRepo.Create: %w", err)
	}
	return nil
}

// SetSessionAudioDecision rewrites the live audio_decision of an ACTIVE
// session - the cast receiver's audio_mode beacon feeds it. Same ended_at
// guard as UpdateProgress: a finished session is never rewritten.
func (r *PlaySessionRepository) SetSessionAudioDecision(ctx context.Context, sessionID, decision string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.SetSessionAudioDecision")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET audio_decision = $2 WHERE id = $1 AND ended_at IS NULL`,
		sessionID, decision)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("PlaySessionRepo.SetSessionAudioDecision: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetSessionServing records the served path on an ACTIVE session (master
// render time); same ended_at guard as SetSessionAudioDecision.
func (r *PlaySessionRepository) SetSessionServing(ctx context.Context, sessionID string, p playback.ServingPath) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.SetSessionServing")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET video_decision = $2, audio_decision = $3, video_engine = $4, video_backend = $5, audio_engine = $6
		 WHERE id = $1 AND ended_at IS NULL`,
		sessionID, p.VideoDecision, p.AudioDecision, p.VideoEngine, p.VideoBackend, p.AudioEngine)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("PlaySessionRepo.SetSessionServing: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AcquireUserStreamLock takes a tx-scoped Postgres advisory lock keyed by
// hashtext(userID). Serializes the MaxStreams quota check+Create pair so
// concurrent StartStream calls can't race past the cap. Outside a tx the
// lock is released at statement end - so caller MUST run inside TxUnit.
func (r *PlaySessionRepository) AcquireUserStreamLock(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.AcquireUserStreamLock")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1))`, userID,
	); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaySessionRepo.AcquireUserStreamLock: %w", err)
	}
	return nil
}

func (r *PlaySessionRepository) Update(ctx context.Context, s *playback.PlaySession) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.Update")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", s.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET device_name=$1, device_type=$2, client_ip=$3, play_method=$4,
		 video_codec=$5, audio_codec=$6, resolution=$7, video_decision=$8, audio_decision=$9, bandwidth_bps=$10, ended_at=$11, duration_ms=$12, position_ms=$13,
		 is_paused=$14, cast_device_name=$15, updated_at=NOW()
		 WHERE id=$16`,
		s.DeviceName, s.DeviceType, s.ClientIP, s.PlayMethod,
		s.VideoCodec, s.AudioCodec, s.Resolution, s.VideoDecision, s.AudioDecision, s.BandwidthBps,
		s.EndedAt, durationMsArg(s.Duration), positionMsArg(s.Position), s.IsPaused, s.CastDeviceName, s.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaySessionRepo.Update: %w", err)
	}
	return nil
}

func (r *PlaySessionRepository) UpdateProgress(ctx context.Context, sessionID string, positionMs int64, isPaused bool, castDeviceName string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.UpdateProgress")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// ended_at IS NULL guard: a progress tick that commits AFTER a concurrent
	// StopStream must not touch the row (it would otherwise leave ended_at set
	// but bump updated_at, keeping the stopped session in the 45s active
	// window). Only the live-playback columns are written - ended_at is never
	// in this statement, so it can never be cleared.
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET position_ms=$2, is_paused=$3, cast_device_name=$4, updated_at=NOW()
		 WHERE id=$1 AND ended_at IS NULL`,
		sessionID, positionMs, isPaused, castDeviceName,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("PlaySessionRepo.UpdateProgress: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("PlaySessionRepo.UpdateProgress: rows affected: %w", err)
	}
	return n > 0, nil
}

func (r *PlaySessionRepository) GetByID(ctx context.Context, id string) (*playback.PlaySession, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	s := &playback.PlaySession{}
	var positionMs *int64
	var durationMs *int64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, video_decision, audio_decision, bandwidth_bps, started_at, ended_at, duration_ms, position_ms, is_paused, cast_device_name
		 FROM play_sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
		&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.VideoDecision, &s.AudioDecision, &s.BandwidthBps,
		&s.StartedAt, &s.EndedAt, &durationMs, &positionMs, &s.IsPaused, &s.CastDeviceName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaySessionRepo.GetByID: %w", err)
	}
	s.Duration = scanDurationMs(s.ID, durationMs)
	s.Position = scanPositionMs(s.ID, positionMs)
	return s, nil
}

func (r *PlaySessionRepository) ListActiveSessions(ctx context.Context) ([]*playback.ActiveSessionInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.ListActiveSessions")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// "Now playing" = a session that is both unfinished AND heartbeated recently.
	// The client beats every 3s (≥~9s under error backoff), so a 45s window keeps
	// live sessions shown (including paused, or playing in a background tab) while a
	// closed player drops off the admin list in ≤45s. The previous 5-min window made
	// a player whose stop POST never landed - e.g. an access token that expired during
	// a long film, 401 silently dropped client-side - linger as "playing" for minutes.
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT ps.id, ps.profile_id, p.name, p.user_id, COALESCE(u.username, ''), ps.media_item_id, mi.title, mi.type,
		        COALESCE(mf.duration_ms, 0), COALESCE(ps.position_ms, 0),
		        ps.started_at, ps.device_name, ps.device_type, ps.client_ip,
		        ps.play_method, ps.video_codec, ps.audio_codec, ps.resolution, ps.video_decision, ps.audio_decision, ps.bandwidth_bps,
		        COALESCE(series.title, ''), COALESCE(season.index_number, 0), COALESCE(mi.index_number, 0),
		        ps.is_paused, ps.cast_device_name, ps.video_engine, ps.video_backend, ps.audio_engine
		 FROM play_sessions ps
		 INNER JOIN profiles p ON p.id = ps.profile_id
		 LEFT JOIN users u ON u.id = p.user_id
		 INNER JOIN media_items mi ON mi.id = ps.media_item_id
		 LEFT JOIN media_files mf ON mf.media_item_id = mi.id
		 LEFT JOIN media_items season ON mi.parent_id = season.id AND mi.type = 'episode'
		 LEFT JOIN media_items series ON season.parent_id = series.id AND season.type = 'season'
		 WHERE ps.ended_at IS NULL
		   AND ps.updated_at > NOW() - INTERVAL '45 seconds'
		 ORDER BY ps.started_at DESC`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaySessionRepo.ListActiveSessions: %w", err)
	}
	defer rows.Close()

	var result []*playback.ActiveSessionInfo
	for rows.Next() {
		s := &playback.ActiveSessionInfo{}
		var rawDurationMs, rawPositionMs int64
		if err := rows.Scan(
			&s.SessionID, &s.ProfileID, &s.ProfileName, &s.UserID, &s.Username,
			&s.MediaItemID, &s.MediaTitle, &s.MediaType,
			&rawDurationMs, &rawPositionMs,
			&s.StartedAt, &s.DeviceName, &s.DeviceType, &s.ClientIP,
			&s.PlayMethod, &s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.VideoDecision, &s.AudioDecision, &s.BandwidthBps,
			&s.SeriesTitle, &s.SeasonNumber, &s.EpisodeNumber, &s.IsPaused, &s.CastDeviceName,
			&s.VideoEngine, &s.VideoBackend, &s.AudioEngine,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("PlaySessionRepo.ListActiveSessions scan: %w", err)
		}
		if d, err := shared.NewDurationMs(rawDurationMs); err == nil {
			s.Duration = d
		}
		if d, err := shared.NewDurationMs(rawPositionMs); err == nil {
			s.Position = d
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaySessionRepo.ListActiveSessions rows: %w", err)
	}
	return result, nil
}

func (r *PlaySessionRepository) GetActiveByProfile(ctx context.Context, profileID string) ([]*playback.PlaySession, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.GetActiveByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, video_decision, audio_decision, bandwidth_bps, started_at, ended_at, duration_ms, position_ms, is_paused, cast_device_name
		 FROM play_sessions WHERE profile_id = $1 AND ended_at IS NULL
		 ORDER BY started_at DESC`, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaySessionRepo.GetActiveByProfile: %w", err)
	}
	defer rows.Close()
	sessions, err := r.scanPlaySessionRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return sessions, nil
}

func (r *PlaySessionRepository) CloseStale(ctx context.Context, olderThan time.Duration) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.CloseStale")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	cutoff := time.Now().Add(-olderThan)
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET ended_at = NOW()
		 WHERE ended_at IS NULL AND updated_at < $1`, cutoff)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("PlaySessionRepo.CloseStale: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	return int(n), nil
}

// CloseAllActive ends every open session. Same single statement as CloseStale
// without the age predicate: a draining server has no session worth keeping,
// and doing it in one UPDATE keeps it inside the shutdown budget however many
// viewers were connected.
func (r *PlaySessionRepository) CloseAllActive(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.CloseAllActive")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET ended_at = NOW() WHERE ended_at IS NULL`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("PlaySessionRepo.CloseAllActive: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	return int(n), nil
}

func (r *PlaySessionRepository) PurgeEndedBefore(ctx context.Context, cutoff time.Time) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.PurgeEndedBefore")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM play_sessions WHERE ended_at IS NOT NULL AND ended_at < $1`, cutoff)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("PlaySessionRepo.PurgeEndedBefore: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	return int(n), nil
}

func (r *PlaySessionRepository) SetRemoteCommand(ctx context.Context, sessionID, command string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.SetRemoteCommand")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET remote_command = $1 WHERE id = $2 AND ended_at IS NULL`, command, sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaySessionRepo.SetRemoteCommand: %w", err)
	}
	return nil
}

func (r *PlaySessionRepository) ConsumeRemoteCommand(ctx context.Context, sessionID string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.ConsumeRemoteCommand")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var cmd string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`WITH old AS (SELECT remote_command FROM play_sessions WHERE id = $1 AND remote_command != '')
		 UPDATE play_sessions SET remote_command = '' WHERE id = $1 AND remote_command != ''
		 RETURNING (SELECT remote_command FROM old)`, sessionID).Scan(&cmd)
	if err != nil {
		return "", nil
	}
	return cmd, nil
}

func (r *PlaySessionRepository) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.CountActiveByUser")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM play_sessions ps
		 INNER JOIN profiles p ON p.id = ps.profile_id
		 WHERE p.user_id = $1 AND ps.ended_at IS NULL`, userID,
	).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("PlaySessionRepo.CountActiveByUser: %w", err)
	}
	return count, nil
}

func (r *PlaySessionRepository) scanPlaySessionRows(rows *sql.Rows) ([]*playback.PlaySession, error) {
	var result []*playback.PlaySession
	for rows.Next() {
		s := &playback.PlaySession{}
		var positionMs *int64
		var durationMs *int64
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
			&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.VideoDecision, &s.AudioDecision, &s.BandwidthBps,
			&s.StartedAt, &s.EndedAt, &durationMs, &positionMs, &s.IsPaused, &s.CastDeviceName); err != nil {
			return nil, fmt.Errorf("PlaySessionRepo.scanRows: %w", err)
		}
		s.Duration = scanDurationMs(s.ID, durationMs)
		s.Position = scanPositionMs(s.ID, positionMs)
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.scanRows: %w", err)
	}
	return result, nil
}

var _ playback.PlaySessionRepository = (*PlaySessionRepository)(nil)
