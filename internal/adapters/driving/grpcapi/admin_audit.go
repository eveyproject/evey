package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type adminSessionHandler struct {
	eveyv1.UnimplementedAdminSessionServiceServer
	activitySvc *appactivity.QueryService
	playbackCmd *appplayback.CommandService
	playbackQry *appplayback.QueryService
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
		return nil, status.Errorf(codes.Internal, "get audit log: %v", err)
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
		return nil, status.Errorf(codes.Internal, "list active sessions: %v", err)
	}
	resp := &eveyv1.ListActiveSessionsResponse{}
	for _, s := range sessions {
		as := &eveyv1.ActiveSession{
			SessionId:    s.SessionID,
			ProfileName:  s.ProfileName,
			MediaTitle:   s.MediaTitle,
			MediaType:    s.MediaType,
			MediaItemId:  s.MediaItemID,
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
			BandwidthBps: s.BandwidthBps,
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
	if err := d.playbackCmd.StopStream(ctx, req.SessionId); err != nil {
		return nil, status.Errorf(codes.Internal, "kill session: %v", err)
	}
	return &eveyv1.KillSessionResponse{}, nil
}
