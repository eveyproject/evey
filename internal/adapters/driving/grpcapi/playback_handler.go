package grpcapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"github.com/gravity-zero/evey/internal/pkg/ratings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playbackHandler struct {
	eveyv1.UnimplementedPlaybackServiceServer
	cmd         *appplayback.CommandService
	query       *appplayback.QueryService
	librarySvc  *applibrary.QueryService
	activitySvc *appactivity.CommandService
	profiles    user.ProfileRepository
}

func (h *playbackHandler) audit(ctx context.Context, userID, action, resourceType string, resourceID *string, details map[string]any) {
	if h.activitySvc == nil {
		return
	}
	e := &activity.AuditLogEntry{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	if ip := interceptors.ClientIPFromCtx(ctx); ip != "" {
		e.ClientIP = &ip
	}
	if ua := interceptors.UserAgentFromCtx(ctx); ua != "" {
		e.UserAgent = &ua
	}
	if dn := interceptors.DeviceNameFromCtx(ctx); dn != "" {
		e.DeviceName = &dn
	}
	if err := h.activitySvc.CreateAuditEntry(ctx, e); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

func (h *playbackHandler) GetStreamInfo(ctx context.Context, req *eveyv1.GetStreamInfoRequest) (*eveyv1.StreamInfo, error) {
	item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	resp := &eveyv1.StreamInfo{
		MediaItemId: req.MediaItemId,
		Container:   item.ContainerFormat,
	}
	streams, err := h.librarySvc.GetMediaStreams(ctx, req.MediaItemId)
	if err != nil {
		slog.Warn("get media streams", slog.String("media_item_id", req.MediaItemId), slog.Any("err", err))
	}
	for _, s := range streams {
		resp.MediaStreams = append(resp.MediaStreams, mediaStreamToProto(s))
	}

	client := transcoding.DefaultWebProfile
	if len(req.SupportedCodecs) > 0 {
		client.VideoCodecs = req.SupportedCodecs
	}
	if len(req.SupportedAudioCodecs) > 0 {
		client.AudioCodecs = req.SupportedAudioCodecs
	}
	if len(req.SupportedContainers) > 0 {
		client.Containers = req.SupportedContainers
	}
	if req.MaxWidth > 0 {
		client.MaxWidth = int(req.MaxWidth)
	}
	if req.MaxHeight > 0 {
		client.MaxHeight = int(req.MaxHeight)
	}

	var maxBitrateKbps int
	if req.MaxBitrate > 0 {
		maxBitrateKbps = int(req.MaxBitrate / 1000)
	}

	decision := apptranscoding.MakePlaybackDecision(streams, item.ContainerFormat, client, maxBitrateKbps)
	resp.TranscodeReasons = decision.TranscodeReasons
	resp.VideoDecision = decision.VideoDecision
	resp.AudioDecision = decision.AudioDecision

	switch decision.PlayMethod {
	case "direct_play":
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY
		resp.DirectPlayUrl = "/api/v1/media/" + req.MediaItemId + "/stream"
	case "direct_stream":
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM
		resp.DirectPlayUrl = "/api/v1/media/" + req.MediaItemId + "/stream"
	default:
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE
		resp.TranscodeUrl = "/api/v1/media/" + req.MediaItemId + "/transcode"
	}

	return resp, nil
}

func (h *playbackHandler) StartStream(ctx context.Context, req *eveyv1.StartStreamRequest) (*eveyv1.PlaySession, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	if h.profiles != nil {
		if blocked, reason := h.isBlockedByParental(ctx, profileID, req.MediaItemId); blocked {
			return nil, status.Errorf(codes.PermissionDenied, "parental control: %s", reason)
		}
	}

	session := &playback.PlaySession{
		ID:          id.New("ps-"),
		ProfileID:   profileID,
		MediaItemID: req.MediaItemId,
		PlayMethod:  playMethodFromProto[req.PlayMethod],
		DeviceName:  req.DeviceName,
		DeviceType:  req.ClientName,
	}
	if err := h.cmd.StartStream(ctx, session, claims.UserID); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, status.Errorf(codes.NotFound, "profile not found")
		}
		slog.Error("start stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	details := map[string]any{"media_id": req.MediaItemId, "method": req.PlayMethod.String()}
	if item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId); err == nil {
		details["title"] = item.Title
	}
	h.audit(ctx, claims.UserID, "start_stream", "media", &req.MediaItemId, details)
	return playSessionToProto(session), nil
}

func (h *playbackHandler) StopStream(ctx context.Context, req *eveyv1.StopStreamRequest) (*eveyv1.StopStreamResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		session, err := h.cmd.GetSession(ctx, req.SessionId)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "session not found")
		}
		if session.ProfileID != claims.ProfileID {
			return nil, status.Error(codes.PermissionDenied, "cannot stop another user's session")
		}
	}
	if err := h.cmd.StopStream(ctx, req.SessionId); err != nil {
		slog.Error("stop stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	sid := req.SessionId
	h.audit(ctx, claims.UserID, "stop_stream", "session", &sid, nil)
	return &eveyv1.StopStreamResponse{}, nil
}

func (h *playbackHandler) ReportProgress(ctx context.Context, req *eveyv1.ReportProgressRequest) (*eveyv1.ReportProgressResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		session, err := h.cmd.GetSession(ctx, req.SessionId)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "session not found")
		}
		if session.ProfileID != claims.ProfileID {
			return nil, status.Error(codes.PermissionDenied, "cannot report progress for another user's session")
		}
	}
	if err := h.cmd.ReportProgressBySession(ctx, req.SessionId, req.PositionMs); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, status.Errorf(codes.NotFound, "profile not found")
		}
		slog.Error("report progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.ReportProgressResponse{}, nil
}

func (h *playbackHandler) GetWatchProgress(ctx context.Context, req *eveyv1.GetWatchProgressRequest) (*eveyv1.WatchProgress, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	wp, err := h.query.GetWatchProgress(ctx, profileID, req.MediaItemId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &eveyv1.WatchProgress{}, nil
		}
		slog.Error("get watch progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return watchProgressToProto(wp), nil
}

func (h *playbackHandler) GetWatchHistory(ctx context.Context, req *eveyv1.GetWatchHistoryRequest) (*eveyv1.GetWatchHistoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := 0
	if req.Pagination != nil {
		offset = int((req.Pagination.Page - 1) * req.Pagination.PageSize)
		limit = int(req.Pagination.PageSize)
	}
	items, err := h.query.GetWatchHistory(ctx, profileID, limit, offset)
	if err != nil {
		slog.Error("get watch history", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.GetWatchHistoryResponse{}
	for _, wp := range items {
		resp.Items = append(resp.Items, watchProgressToProto(wp))
	}
	return resp, nil
}

func (h *playbackHandler) GetResumeItems(ctx context.Context, req *eveyv1.GetResumeItemsRequest) (*eveyv1.GetResumeItemsResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	items, err := h.query.GetResumeItems(ctx, profileID, limit)
	if err != nil {
		slog.Error("get resume items", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.GetResumeItemsResponse{}
	for _, wp := range items {
		resp.Progress = append(resp.Progress, watchProgressToProto(wp))
	}
	return resp, nil
}

func (h *playbackHandler) GetNextUp(ctx context.Context, req *eveyv1.GetNextUpRequest) (*eveyv1.GetNextUpResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	progresses, err := h.query.GetNextUp(ctx, profileID, limit)
	if err != nil {
		slog.Error("get next up", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.GetNextUpResponse{}
	for _, wp := range progresses {
		item, err := h.librarySvc.GetMediaItem(ctx, wp.MediaItemID)
		if err != nil {
			continue
		}
		resp.Items = append(resp.Items, mediaItemToProto(item))
	}
	return resp, nil
}

func (h *playbackHandler) MarkAsWatched(ctx context.Context, req *eveyv1.MarkAsWatchedRequest) (*eveyv1.MarkAsWatchedResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	if err := h.cmd.MarkAsWatched(ctx, profileID, req.MediaItemId); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, status.Errorf(codes.NotFound, "profile not found")
		}
		slog.Error("mark as watched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsWatchedResponse{}, nil
}

func (h *playbackHandler) MarkAsUnwatched(ctx context.Context, req *eveyv1.MarkAsUnwatchedRequest) (*eveyv1.MarkAsUnwatchedResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	if err := h.cmd.MarkAsUnwatched(ctx, profileID, req.MediaItemId); err != nil {
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			return nil, status.Errorf(codes.NotFound, "profile not found")
		}
		slog.Error("mark as unwatched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsUnwatchedResponse{}, nil
}

func (h *playbackHandler) isBlockedByParental(ctx context.Context, profileID, mediaItemID string) (bool, string) {
	profile, err := h.profiles.GetByID(ctx, profileID)
	if err != nil || profile.Parental == nil || !profile.Parental.Enabled {
		return false, ""
	}
	ps := profile.Parental
	item, err := h.librarySvc.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return false, ""
	}
	if ps.MaxRating != "" && !ratings.IsAllowed(item.OfficialRating, ps.MaxRating) {
		return true, "content rating exceeds profile limit"
	}
	if ratings.HasBlockedTag(item.Tags, ps.BlockedTags) {
		return true, "content contains blocked tags"
	}
	return false, ""
}
