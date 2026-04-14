package grpcapi

import (
	"context"
	"log/slog"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// HLSSessionRemover can kill an active HLS transcode by media item ID.
type HLSSessionRemover interface {
	RemoveSession(mediaItemID string)
}

type adminSessionHandler struct {
	eveyv1.UnimplementedAdminSessionServiceServer
	activitySvc *appactivity.QueryService
	playbackCmd *appplayback.CommandService
	playbackQry *appplayback.QueryService
	hlsManager  HLSSessionRemover
}

func (d *adminSessionHandler) GetAuditLog(ctx context.Context, req *eveyv1.GetAuditLogRequest) (*eveyv1.GetAuditLogResponse, error) {
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
		return nil, internalError("get audit log", err)
	}
	resp := &eveyv1.GetAuditLogResponse{Total: int32(len(entries))}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, auditLogEntryToProto(e))
	}
	return resp, nil
}

func (d *adminSessionHandler) ListActiveSessions(ctx context.Context, _ *eveyv1.ListActiveSessionsRequest) (*eveyv1.ListActiveSessionsResponse, error) {
	sessions, err := d.playbackQry.GetActiveSessions(ctx)
	if err != nil {
		return nil, internalError("list active sessions", err)
	}
	resp := &eveyv1.ListActiveSessionsResponse{}
	for _, s := range sessions {
		as := &eveyv1.ActiveSession{
			SessionId:    s.SessionID,
			ProfileId:    string(s.ProfileID),
			ProfileName:  s.ProfileName,
			MediaTitle:   s.MediaTitle,
			MediaType:    s.MediaType,
			MediaItemId:  string(s.MediaItemID),
			Progress:     s.ProgressPercent(),
			DurationMs:   s.DurationMs,
			PositionMs:   s.PositionMs,
			StartedAt:    timestamppb.New(s.StartedAt),
			DeviceName:   s.DeviceName,
			DeviceType:   s.DeviceType,
			ClientIp:     s.ClientIP,
			PlayMethod:   s.PlayMethod,
			VideoCodec:   s.VideoCodec,
			AudioCodec:   s.AudioCodec,
			Resolution:   s.Resolution,
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
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id required")
	}
	session, err := d.playbackCmd.GetSession(ctx, req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "session not found: %v", err)
	}
	// Send stop command so the player closes gracefully
	if err := d.playbackCmd.SetRemoteCommand(ctx, req.SessionId, "stop"); err != nil {
		slog.Warn("set remote command failed", slog.String("session_id", req.SessionId), slog.Any("err", err))
	}
	// Defer the actual kill to give the player time to receive the command
	mediaItemID := session.MediaItemID
	sessionID := req.SessionId
	hlsMgr := d.hlsManager
	cmdSvc := d.playbackCmd
	go func() {
		time.Sleep(5 * time.Second)
		if err := cmdSvc.StopStream(context.Background(), sessionID); err != nil {
			slog.Warn("stop stream failed", slog.String("session_id", sessionID), slog.Any("err", err))
		}
		if hlsMgr != nil {
			hlsMgr.RemoveSession(string(mediaItemID))
		}
	}()
	return &eveyv1.KillSessionResponse{}, nil
}

func (d *adminSessionHandler) SendSessionCommand(ctx context.Context, req *eveyv1.SendSessionCommandRequest) (*eveyv1.SendSessionCommandResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin required")
	}
	if req.SessionId == "" || req.Command == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id and command required")
	}
	if err := d.playbackCmd.SetRemoteCommand(ctx, req.SessionId, req.Command); err != nil {
		return nil, internalError("send command", err)
	}
	return &eveyv1.SendSessionCommandResponse{}, nil
}
