package grpcapi

import (
	"context"
	"log/slog"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/activity"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// HLSSessionRemover can kill an active HLS transcode by media item ID.
type HLSSessionRemover interface {
	RemoveSession(mediaItemID string)
}

// hlsSessionManager is what the gRPC wiring needs from the HLS producer layer:
// kill a media's sessions AND report the live count. Both the hardware
// orchestrator and the fan-out ProducerSelector satisfy it, so wiring the
// selector here makes RemoveSession and ActiveSessionCount span software + HW.
type hlsSessionManager interface {
	HLSSessionRemover
	ActiveSessionCount() int
}

type adminSessionHandler struct {
	eveyv1.UnimplementedAdminSessionServiceServer
	activitySvc *appactivity.QueryService
	playbackCmd *appplayback.CommandService
	playbackQry *appplayback.QueryService
	hlsManager  HLSSessionRemover
	musicNow    *appmusic.NowPlayingRegistry // may be nil; live music sessions
	musicSvc    *appmusic.Service            // may be nil; enriches a music session's track
	// userQuery resolves a music session's profile id to a profile name and an
	// owning account. Video sessions get both straight from SQL; the music
	// registry is in-memory and holds no profile repository. May be nil.
	userQuery  *appuser.QueryService
	translator *eveyi18n.Translator
}

// profileLabel is the display identity behind a profile id: which profile, and
// on whose account.
type profileLabel struct {
	name     string
	userID   string
	username string
}

// profileLabels indexes every profile by id in one read. Called once per
// ListActiveSessions (only when there are music sessions to label), never per
// session. A failure yields an empty map: an unlabeled session is better than
// a failed panel.
func (d *adminSessionHandler) profileLabels(ctx context.Context) map[string]profileLabel {
	if d.userQuery == nil {
		return nil
	}
	profiles, err := d.userQuery.ListAllProfiles(ctx)
	if err != nil {
		slog.WarnContext(ctx, "resolve profile labels", slog.Any("err", err))
		return nil
	}
	users, err := d.userQuery.ListUsers(ctx)
	if err != nil {
		slog.WarnContext(ctx, "resolve profile labels", slog.Any("err", err))
		users = nil
	}
	usernames := make(map[string]string, len(users))
	for _, u := range users {
		usernames[string(u.ID)] = u.Username
	}
	out := make(map[string]profileLabel, len(profiles))
	for _, p := range profiles {
		out[string(p.ID)] = profileLabel{
			name:     p.Name,
			userID:   string(p.UserID),
			username: usernames[string(p.UserID)],
		}
	}
	return out
}

func (d *adminSessionHandler) GetAuditLog(ctx context.Context, req *eveyv1.GetAuditLogRequest) (*eveyv1.GetAuditLogResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetAuditLog")
	defer span.End()
	filter := activity.AuditLogFilter{
		Limit:  int(req.Limit),
		Offset: int(req.Offset),
	}
	if req.UserId != "" {
		filter.UserID = &req.UserId
	}
	if req.ProfileId != "" {
		filter.ProfileID = &req.ProfileId
	}
	if req.Action != "" {
		filter.Action = &req.Action
	}
	if req.ResourceType != "" {
		filter.ResourceType = &req.ResourceType
	}
	entries, err := d.activitySvc.GetAuditLog(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get audit log", err)
	}
	// Real total across all matching rows (a COUNT), so the client can paginate
	// past the first page - Total used to be len(entries), i.e. the page size.
	total, err := d.activitySvc.CountAuditLog(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "count audit log", err)
	}
	resp := &eveyv1.GetAuditLogResponse{Total: int32(total)} //nolint:gosec // count fits int32
	for _, e := range entries {
		resp.Entries = append(resp.Entries, auditLogEntryToProto(e))
	}
	return resp, nil
}

func (d *adminSessionHandler) ListActiveSessions(ctx context.Context, _ *eveyv1.ListActiveSessionsRequest) (*eveyv1.ListActiveSessionsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ListActiveSessions")
	defer span.End()
	sessions, err := d.playbackQry.GetActiveSessions(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list active sessions", err)
	}
	resp := &eveyv1.ListActiveSessionsResponse{}
	for _, s := range sessions {
		as := &eveyv1.ActiveSession{
			SessionId:      s.SessionID,
			ProfileId:      string(s.ProfileID),
			ProfileName:    s.ProfileName,
			MediaTitle:     s.MediaTitle,
			MediaType:      s.MediaType,
			MediaItemId:    string(s.MediaItemID),
			Progress:       s.ProgressPercent(),
			DurationMs:     s.Duration.Milliseconds(),
			PositionMs:     s.Position.Milliseconds(),
			StartedAt:      timestamppb.New(s.StartedAt),
			DeviceName:     s.DeviceName,
			DeviceType:     s.DeviceType,
			ClientIp:       s.ClientIP,
			PlayMethod:     s.PlayMethod,
			VideoCodec:     s.VideoCodec,
			AudioCodec:     s.AudioCodec,
			Resolution:     s.Resolution,
			VideoDecision:  s.VideoDecision,
			AudioDecision:  s.AudioDecision,
			BandwidthBps:   s.BandwidthBps,
			SeriesTitle:    s.SeriesTitle,
			SeasonNumber:   int32(s.SeasonNumber),
			EpisodeNumber:  int32(s.EpisodeNumber),
			IsPaused:       s.IsPaused,
			CastDeviceName: s.CastDeviceName,
			UserId:         string(s.UserID),
			Username:       s.Username,
		}
		if s.TranscodeInfo != nil {
			as.TranscodeInfo = *s.TranscodeInfo
		}
		resp.Sessions = append(resp.Sessions, as)
	}
	d.appendMusicSessions(ctx, resp)
	return resp, nil
}

// appendMusicSessions folds the live in-memory music sessions into the response,
// enriching each with its track's display metadata. A session whose track can no
// longer be resolved (deleted, or the music service is unwired) is skipped rather
// than shown as a blank card.
func (d *adminSessionHandler) appendMusicSessions(ctx context.Context, resp *eveyv1.ListActiveSessionsResponse) {
	if d.musicNow == nil || d.musicSvc == nil {
		return
	}
	live := d.musicNow.List()
	if len(live) == 0 {
		return
	}
	labels := d.profileLabels(ctx)
	for _, ms := range live {
		track, err := d.musicSvc.GetTrack(ctx, ms.TrackID)
		if err != nil || track == nil {
			continue
		}
		label := labels[ms.ProfileID]
		// The registry only knows the account name; fall back to it so a
		// session whose profile was deleted mid-play is still identified.
		profileName := label.name
		if profileName == "" {
			profileName = ms.Username
		}
		username := label.username
		if username == "" {
			username = ms.Username
		}
		title := track.Title
		if track.Artist != "" {
			title = track.Artist + " - " + track.Title
		}
		durationMs := int64(track.Duration * 1000)
		var progress float64
		if durationMs > 0 {
			progress = float64(ms.PositionMs) / float64(durationMs) * 100
		}
		resp.Sessions = append(resp.Sessions, &eveyv1.ActiveSession{
			SessionId:   ms.SessionID,
			ProfileId:   ms.ProfileID,
			ProfileName: profileName,
			UserId:      label.userID,
			Username:    username,
			MediaTitle:  title,
			MediaType:   "track",
			MediaItemId: ms.TrackID,
			Progress:    progress,
			DurationMs:  durationMs,
			PositionMs:  ms.PositionMs,
			StartedAt:   timestamppb.New(ms.StartedAt),
			DeviceName:  ms.DeviceName,
			PlayMethod:  "DirectStream",
			AudioCodec:  track.MimeType,
			IsPaused:    ms.IsPaused,
			CoverArtUrl: track.CoverArtURL,
		})
	}
}

func (d *adminSessionHandler) KillSession(ctx context.Context, req *eveyv1.KillSessionRequest) (*eveyv1.KillSessionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.KillSession")
	defer span.End()
	// Same guard as SendSessionCommand right below - this RPC stops ANY
	// viewer's playback from a session id alone, and it used to lean entirely
	// on the interceptor's MethodRoles entry. That entry is data in a map: one
	// deleted line opens this door to every authenticated user, and nothing
	// fails to say so. Defence in depth, not a second opinion.
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin required")
		return nil, d.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if req.SessionId == "" {
		span.SetStatus(otelcodes.Error, "session_id required")
		return nil, status.Error(codes.InvalidArgument, "session_id required")
	}
	session, err := d.playbackCmd.GetSession(ctx, req.SessionId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "session not found")
		slog.WarnContext(ctx, "grpc KillSession: session query failed", slog.Any("err", err))
		return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
	}
	// Send stop command so the player closes gracefully. This stops the
	// PLAYBACK session only - it deliberately does NOT revoke the owner's auth
	// tokens. An admin stopping a stream (free a transcode slot, end a forgotten
	// session) must not sign the user out of the app; banning a user is a
	// separate action (DisableUser, which bumps the token epoch).
	if err := d.playbackCmd.SetRemoteCommand(ctx, req.SessionId, "stop"); err != nil {
		slog.WarnContext(ctx, "set remote command failed", slog.String("session_id", req.SessionId), slog.Any("err", err))
	}
	// Defer the actual kill to give the player time to receive the command
	mediaItemID := session.MediaItemID
	sessionID := req.SessionId
	hlsMgr := d.hlsManager
	cmdSvc := d.playbackCmd
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic in deferred kill-session goroutine",
					slog.Any("err", r), slog.String("session_id", sessionID))
			}
		}()
		time.Sleep(5 * time.Second)
		// Bound the StopStream call so a hung DB or downstream service can't
		// leak this goroutine forever. context.Background() would inherit no
		// deadline at all; 30s is plenty for the player-close handshake.
		killCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := cmdSvc.StopStream(killCtx, sessionID); err != nil {
			slog.WarnContext(ctx, "stop stream failed", slog.String("session_id", sessionID), slog.Any("err", err))
		}
		if hlsMgr != nil {
			hlsMgr.RemoveSession(string(mediaItemID))
		}
	}()
	return &eveyv1.KillSessionResponse{}, nil
}

func (d *adminSessionHandler) SendSessionCommand(ctx context.Context, req *eveyv1.SendSessionCommandRequest) (*eveyv1.SendSessionCommandResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.SendSessionCommand")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin required")
		return nil, d.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if req.SessionId == "" || req.Command == "" {
		span.SetStatus(otelcodes.Error, "session_id and command required")
		return nil, status.Error(codes.InvalidArgument, "session_id and command required")
	}
	// A music session lives in the in-memory registry, not play_sessions: route
	// its command there. SetCommand reports whether the id was a live music
	// session, so a video session falls through to the DB path below.
	if d.musicNow != nil && d.musicNow.SetCommand(req.SessionId, req.Command) {
		return &eveyv1.SendSessionCommandResponse{}, nil
	}
	if err := d.playbackCmd.SetRemoteCommand(ctx, req.SessionId, req.Command); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "send command", err)
	}
	return &eveyv1.SendSessionCommandResponse{}, nil
}

// SendCastReceiverCommand reaches a Chromecast still beating with no play
// session left - a television that never really switches off keeps the receiver
// page loaded long after its session ended, and SendSessionCommand has no row
// to write on. Same admin guard and same vocabulary as SendSessionCommand: this
// controls somebody else's television, and it must not lean on the
// interceptor's role map alone.
func (d *adminSessionHandler) SendCastReceiverCommand(ctx context.Context, req *eveyv1.SendCastReceiverCommandRequest) (*eveyv1.SendCastReceiverCommandResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.SendCastReceiverCommand")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin required")
		return nil, d.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if req.ProfileId == "" || req.Command == "" {
		span.SetStatus(otelcodes.Error, "profile_id and command required")
		return nil, status.Error(codes.InvalidArgument, "profile_id and command required")
	}
	if err := d.playbackCmd.SetCastReceiverCommand(ctx, req.ProfileId, req.Command); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "send cast receiver command", err)
	}
	return &eveyv1.SendCastReceiverCommandResponse{}, nil
}
