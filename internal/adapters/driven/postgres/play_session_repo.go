package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
		 play_method, video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		s.ID, s.ProfileID, s.MediaItemID, s.DeviceName, s.DeviceType, s.ClientIP,
		s.PlayMethod, s.VideoCodec, s.AudioCodec, s.Resolution, s.BandwidthBps,
		s.StartedAt, s.EndedAt, durationMsArg(s.Duration), positionMsArg(s.Position),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PlaySessionRepo.Create: %w", err)
	}
	return nil
}

// SetServed records what the server sent on an ACTIVE session, with the
// summary columns the history and the play-method counts read, in one
// statement: the summary is derived from the served path, never written on
// its own.
func (r *PlaySessionRepository) SetServed(ctx context.Context, sessionID string, served playback.Served) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.SetServed")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	raw, err := json.Marshal(served)
	if err != nil {
		return false, fmt.Errorf("PlaySessionRepo.SetServed: %w", err)
	}
	method, videoCodec, audioCodec, resolution := served.Summary()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET served = $2, play_method = $3, video_codec = $4, audio_codec = $5, resolution = $6
		 WHERE id = $1 AND ended_at IS NULL`,
		sessionID, raw, method, videoCodec, audioCodec, resolution)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("PlaySessionRepo.SetServed: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ResetWitness empties what an ACTIVE session was seen served and playing.
func (r *PlaySessionRepository) ResetWitness(ctx context.Context, sessionID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.ResetWitness")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET served = '{}'::jsonb, played = NULL, play_method = '', video_codec = '', audio_codec = '', resolution = ''
		 WHERE id = $1 AND ended_at IS NULL`, sessionID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("PlaySessionRepo.ResetWitness: %w", err)
	}
	return nil
}

// SetPlayed records what the player reports it decodes on an ACTIVE session.
func (r *PlaySessionRepository) SetPlayed(ctx context.Context, sessionID string, played playback.Played) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.SetPlayed")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	raw, err := json.Marshal(played)
	if err != nil {
		return false, fmt.Errorf("PlaySessionRepo.SetPlayed: %w", err)
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET played = $2 WHERE id = $1 AND ended_at IS NULL`, sessionID, raw)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("PlaySessionRepo.SetPlayed: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// scanServed decodes the served column; a row that cannot be read shows no
// served path rather than failing the whole listing.
func scanServed(sessionID string, raw []byte) playback.Served {
	var out playback.Served
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		slog.Warn("invalid served in play_sessions row", slog.String("sessionID", sessionID), slog.Any("err", err))
		return playback.Served{}
	}
	return out
}

// scanPlayed decodes the played column; nil when the player never reported.
func scanPlayed(sessionID string, raw []byte) *playback.Played {
	if len(raw) == 0 {
		return nil
	}
	var out playback.Played
	if err := json.Unmarshal(raw, &out); err != nil {
		slog.Warn("invalid played in play_sessions row", slog.String("sessionID", sessionID), slog.Any("err", err))
		return nil
	}
	return &out
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
		 video_codec=$5, audio_codec=$6, resolution=$7, bandwidth_bps=$8, ended_at=$9, duration_ms=$10, position_ms=$11,
		 is_paused=$12, cast_device_name=$13, ended_by_sweep=false, updated_at=NOW()
		 WHERE id=$14`,
		s.DeviceName, s.DeviceType, s.ClientIP, s.PlayMethod,
		s.VideoCodec, s.AudioCodec, s.Resolution, s.BandwidthBps,
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
	var servedRaw, playedRaw []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, profile_id, media_item_id, device_name, device_type, client_ip, play_method,
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms, is_paused, cast_device_name,
		 served, played
		 FROM play_sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
		&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
		&s.StartedAt, &s.EndedAt, &durationMs, &positionMs, &s.IsPaused, &s.CastDeviceName,
		&servedRaw, &playedRaw)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaySessionRepo.GetByID: %w", err)
	}
	s.Duration = scanDurationMs(s.ID, durationMs)
	s.Position = scanPositionMs(s.ID, positionMs)
	s.Served, s.Played = scanServed(s.ID, servedRaw), scanPlayed(s.ID, playedRaw)
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
		        ps.play_method, ps.video_codec, ps.audio_codec, ps.resolution, ps.bandwidth_bps,
		        COALESCE(series.title, ''), COALESCE(season.index_number, 0), COALESCE(mi.index_number, 0),
		        ps.is_paused, ps.cast_device_name, ps.served, ps.played
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
		var servedRaw, playedRaw []byte
		if err := rows.Scan(
			&s.SessionID, &s.ProfileID, &s.ProfileName, &s.UserID, &s.Username,
			&s.MediaItemID, &s.MediaTitle, &s.MediaType,
			&rawDurationMs, &rawPositionMs,
			&s.StartedAt, &s.DeviceName, &s.DeviceType, &s.ClientIP,
			&s.PlayMethod, &s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
			&s.SeriesTitle, &s.SeasonNumber, &s.EpisodeNumber, &s.IsPaused, &s.CastDeviceName,
			&servedRaw, &playedRaw,
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
		s.Served, s.Played = scanServed(s.SessionID, servedRaw), scanPlayed(s.SessionID, playedRaw)
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
		 video_codec, audio_codec, resolution, bandwidth_bps, started_at, ended_at, duration_ms, position_ms, is_paused, cast_device_name,
		 served, played
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
		`UPDATE play_sessions SET ended_at = NOW(), ended_by_sweep = true
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

// CloseStaleReturning ends the sessions without a heartbeat since the cutoff
// at their last heartbeat, with the duration measured to it, and returns them
// so the caller can say they stopped.
func (r *PlaySessionRepository) CloseStaleReturning(ctx context.Context, olderThan time.Duration) ([]*playback.PlaySession, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.CloseStaleReturning")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	cutoff := time.Now().Add(-olderThan)
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`UPDATE play_sessions
		 SET ended_at = updated_at,
		     ended_by_sweep = true,
		     duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (updated_at - started_at)) * 1000)::bigint)
		 WHERE ended_at IS NULL AND updated_at < $1
		 RETURNING id, profile_id, media_item_id, started_at, ended_at, duration_ms, position_ms`, cutoff)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("PlaySessionRepo.CloseStaleReturning: %w", err)
	}
	defer rows.Close()
	var ended []*playback.PlaySession
	for rows.Next() {
		s := &playback.PlaySession{}
		var endedAt sql.NullTime
		var durationMs, positionMs sql.NullInt64
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.StartedAt, &endedAt, &durationMs, &positionMs); err != nil {
			return nil, fmt.Errorf("PlaySessionRepo.CloseStaleReturning: %w", err)
		}
		if endedAt.Valid {
			t := endedAt.Time
			s.EndedAt = &t
		}
		if durationMs.Valid {
			if d, dErr := shared.NewDurationMs(durationMs.Int64); dErr == nil {
				s.Duration = &d
			}
		}
		if positionMs.Valid {
			if p, pErr := shared.NewDurationMs(positionMs.Int64); pErr == nil {
				s.Position = &p
			}
		}
		ended = append(ended, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.CloseStaleReturning: %w", err)
	}
	return ended, nil
}

func (r *PlaySessionRepository) ReopenNotStopped(ctx context.Context, sessionID string, before time.Time, positionMs int64, isPaused bool, castDeviceName string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.playsession.ReopenNotStopped")
	defer span.End()
	span.SetAttributes(attribute.String("play_session.id", sessionID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions SET ended_at=NULL, ended_by_sweep=false, position_ms=$3, is_paused=$4, cast_device_name=$5, updated_at=NOW()
		 WHERE id=$1 AND ended_at IS NOT NULL AND (ended_at < $2 OR ended_by_sweep)`,
		sessionID, before, positionMs, isPaused, castDeviceName,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("PlaySessionRepo.ReopenNotStopped: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("PlaySessionRepo.ReopenNotStopped: rows affected: %w", err)
	}
	return n > 0, nil
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
	// The duration is stored with the end: a session closed by the shutdown
	// otherwise has none, and the stats read it.
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE play_sessions
		 SET ended_at = NOW(),
		     duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint)
		 WHERE ended_at IS NULL`)
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
		var servedRaw, playedRaw []byte
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.MediaItemID, &s.DeviceName, &s.DeviceType, &s.ClientIP, &s.PlayMethod,
			&s.VideoCodec, &s.AudioCodec, &s.Resolution, &s.BandwidthBps,
			&s.StartedAt, &s.EndedAt, &durationMs, &positionMs, &s.IsPaused, &s.CastDeviceName,
			&servedRaw, &playedRaw); err != nil {
			return nil, fmt.Errorf("PlaySessionRepo.scanRows: %w", err)
		}
		s.Duration = scanDurationMs(s.ID, durationMs)
		s.Position = scanPositionMs(s.ID, positionMs)
		s.Served, s.Played = scanServed(s.ID, servedRaw), scanPlayed(s.ID, playedRaw)
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PlaySessionRepo.scanRows: %w", err)
	}
	return result, nil
}

var _ playback.PlaySessionRepository = (*PlaySessionRepository)(nil)
