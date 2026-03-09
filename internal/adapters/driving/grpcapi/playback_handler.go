package grpcapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playbackHandler struct {
	eveyv1.UnimplementedPlaybackServiceServer
	cmd        *appplayback.CommandService
	query      *appplayback.QueryService
	librarySvc *applibrary.QueryService
}

func (h *playbackHandler) GetStreamInfo(ctx context.Context, req *eveyv1.GetStreamInfoRequest) (*eveyv1.StreamInfo, error) {
	item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	resp := &eveyv1.StreamInfo{
		MediaItemId:   req.MediaItemId,
		PlayMethod:    eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DirectPlayUrl: "/api/v1/media/" + req.MediaItemId + "/stream",
		Container:     item.ContainerFormat,
	}
	if streams, err := h.librarySvc.GetMediaStreams(ctx, req.MediaItemId); err == nil {
		for _, s := range streams {
			resp.MediaStreams = append(resp.MediaStreams, mediaStreamToProto(s))
		}
	}
	return resp, nil
}

func (h *playbackHandler) StartStream(ctx context.Context, req *eveyv1.StartStreamRequest) (*eveyv1.PlaySession, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	profileID := req.ProfileId
	if profileID == "" {
		profileID = claims.ProfileID
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
		slog.Error("start stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return playSessionToProto(session), nil
}

func (h *playbackHandler) StopStream(ctx context.Context, req *eveyv1.StopStreamRequest) (*eveyv1.StopStreamResponse, error) {
	if err := h.cmd.StopStream(ctx, req.SessionId); err != nil {
		slog.Error("stop stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.StopStreamResponse{}, nil
}

func (h *playbackHandler) ReportProgress(ctx context.Context, req *eveyv1.ReportProgressRequest) (*eveyv1.ReportProgressResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	if err := h.cmd.ReportProgressBySession(ctx, req.SessionId, req.PositionMs); err != nil {
		slog.Error("report progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.ReportProgressResponse{}, nil
}

func (h *playbackHandler) GetWatchProgress(ctx context.Context, req *eveyv1.GetWatchProgressRequest) (*eveyv1.WatchProgress, error) {
	profileID := req.ProfileId
	if profileID == "" {
		if claims := interceptors.ClaimsFromContext(ctx); claims != nil {
			profileID = claims.ProfileID
		}
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
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := 0
	if req.Pagination != nil {
		offset = int((req.Pagination.Page - 1) * req.Pagination.PageSize)
		limit = int(req.Pagination.PageSize)
	}
	items, err := h.query.GetWatchHistory(ctx, req.ProfileId, limit, offset)
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
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	items, err := h.query.GetResumeItems(ctx, req.ProfileId, limit)
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
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	progresses, err := h.query.GetNextUp(ctx, req.ProfileId, limit)
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
	if err := h.cmd.MarkAsWatched(ctx, req.ProfileId, req.MediaItemId); err != nil {
		slog.Error("mark as watched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsWatchedResponse{}, nil
}

func (h *playbackHandler) MarkAsUnwatched(ctx context.Context, req *eveyv1.MarkAsUnwatchedRequest) (*eveyv1.MarkAsUnwatchedResponse, error) {
	if err := h.cmd.MarkAsUnwatched(ctx, req.ProfileId, req.MediaItemId); err != nil {
		slog.Error("mark as unwatched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsUnwatchedResponse{}, nil
}
