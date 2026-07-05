package grpcapi

import (
	"context"
	"log/slog"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/activity"
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
		return nil, internalError("get audit log", err)
	}
	// Real total across all matching rows (a COUNT), so the client can paginate
	// past the first page — Total used to be len(entries), i.e. the page size.
	total, err := d.activitySvc.CountAuditLog(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("count audit log", err)
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
		return nil, internalError("list active sessions", err)
	}
	resp := &eveyv1.ListActiveSessionsResponse{}
	for _, s := range sessions {
		as := &eveyv1.ActiveSession{
			SessionId:     s.SessionID,
			ProfileId:     string(s.ProfileID),
			ProfileName:   s.ProfileName,
			MediaTitle:    s.MediaTitle,
			MediaType:     s.MediaType,
			MediaItemId:   string(s.MediaItemID),
			Progress:      s.ProgressPercent(),
			DurationMs:    s.Duration.Milliseconds(),
			PositionMs:    s.Position.Milliseconds(),
			StartedAt:     timestamppb.New(s.StartedAt),
			DeviceName:    s.DeviceName,
			DeviceType:    s.DeviceType,
			ClientIp:      s.ClientIP,
			PlayMethod:    s.PlayMethod,
			VideoCodec:    s.VideoCodec,
			AudioCodec:    s.AudioCodec,
			Resolution:    s.Resolution,
			BandwidthBps:  s.BandwidthBps,
			SeriesTitle:   s.SeriesTitle,
			SeasonNumber:  int32(s.SeasonNumber),
			EpisodeNumber: int32(s.EpisodeNumber),
			IsPaused:      s.IsPaused,
		}
		if s.TranscodeInfo != nil {
			as.TranscodeInfo = *s.TranscodeInfo
		}
		resp.Sessions = append(resp.Sessions, as)
	}
	return resp, nil
}

func (d *adminSessionHandler) KillSession(ctx context.Context, req *eveyv1.KillSessionRequest) (*eveyv1.KillSessionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.KillSession")
	defer span.End()
	if req.SessionId == "" {
		span.SetStatus(otelcodes.Error, "session_id required")
		return nil, status.Error(codes.InvalidArgument, "session_id required")
	}
	session, err := d.playbackCmd.GetSession(ctx, req.SessionId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "session not found")
		slog.Warn("grpc KillSession: session query failed", slog.Any("err", err))
		return nil, status.Error(codes.NotFound, "session not found")
	}
	// Send stop command so the player closes gracefully. This stops the
	// PLAYBACK session only — it deliberately does NOT revoke the owner's auth
	// tokens. An admin stopping a stream (free a transcode slot, end a forgotten
	// session) must not sign the user out of the app; banning a user is a
	// separate action (DisableUser, which bumps the token epoch).
	if err := d.playbackCmd.SetRemoteCommand(ctx, req.SessionId, "stop"); err != nil {
		slog.Warn("set remote command failed", slog.String("session_id", req.SessionId), slog.Any("err", err))
	}
	// Defer the actual kill to give the player time to receive the command
	mediaItemID := session.MediaItemID
	sessionID := req.SessionId
	hlsMgr := d.hlsManager
	cmdSvc := d.playbackCmd
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in deferred kill-session goroutine",
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
			slog.Warn("stop stream failed", slog.String("session_id", sessionID), slog.Any("err", err))
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
		return nil, status.Error(codes.PermissionDenied, "admin required")
	}
	if req.SessionId == "" || req.Command == "" {
		span.SetStatus(otelcodes.Error, "session_id and command required")
		return nil, status.Error(codes.InvalidArgument, "session_id and command required")
	}
	if err := d.playbackCmd.SetRemoteCommand(ctx, req.SessionId, req.Command); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("send command", err)
	}
	return &eveyv1.SendSessionCommandResponse{}, nil
}
