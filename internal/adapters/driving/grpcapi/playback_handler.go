package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/domain/playback"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playbackHandler struct {
	eveyv1.UnimplementedPlaybackServiceServer
	svc *appplayback.Service
}

func (h *playbackHandler) GetStreamInfo(ctx context.Context, req *eveyv1.GetStreamInfoRequest) (*eveyv1.StreamInfo, error) {
	_ = req
	return &eveyv1.StreamInfo{
		MediaItemId: req.MediaItemId,
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	}, nil
}

func (h *playbackHandler) StartStream(ctx context.Context, req *eveyv1.StartStreamRequest) (*eveyv1.PlaySession, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	session := &playback.PlaySession{
		ID:          fmt.Sprintf("ps-%d", nowUnixNano()),
		ProfileID:   req.ProfileId,
		MediaItemID: req.MediaItemId,
		PlayMethod:  playMethodFromProto[req.PlayMethod],
		DeviceName:  req.DeviceName,
		DeviceType:  req.ClientName,
	}
	if err := h.svc.StartStream(ctx, session, claims.UserID); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return playSessionToProto(session), nil
}

func (h *playbackHandler) StopStream(ctx context.Context, req *eveyv1.StopStreamRequest) (*eveyv1.StopStreamResponse, error) {
	if err := h.svc.StopStream(ctx, req.SessionId); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &eveyv1.StopStreamResponse{}, nil
}

func (h *playbackHandler) ReportProgress(ctx context.Context, req *eveyv1.ReportProgressRequest) (*eveyv1.ReportProgressResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	if err := h.svc.ReportProgress(ctx, claims.ProfileID, req.SessionId, req.PositionMs, false); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &eveyv1.ReportProgressResponse{}, nil
}

func (h *playbackHandler) GetWatchProgress(ctx context.Context, req *eveyv1.GetWatchProgressRequest) (*eveyv1.WatchProgress, error) {
	wp, err := h.svc.GetWatchProgress(ctx, req.ProfileId, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
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
	items, err := h.svc.GetWatchHistory(ctx, req.ProfileId, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
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
	items, err := h.svc.GetResumeItems(ctx, req.ProfileId, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
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
	_, err := h.svc.GetNextUp(ctx, req.ProfileId, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &eveyv1.GetNextUpResponse{}, nil
}

func (h *playbackHandler) MarkAsWatched(ctx context.Context, req *eveyv1.MarkAsWatchedRequest) (*eveyv1.MarkAsWatchedResponse, error) {
	if err := h.svc.MarkAsWatched(ctx, req.ProfileId, req.MediaItemId); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &eveyv1.MarkAsWatchedResponse{}, nil
}

func (h *playbackHandler) MarkAsUnwatched(ctx context.Context, req *eveyv1.MarkAsUnwatchedRequest) (*eveyv1.MarkAsUnwatchedResponse, error) {
	if err := h.svc.MarkAsUnwatched(ctx, req.ProfileId, req.MediaItemId); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &eveyv1.MarkAsUnwatchedResponse{}, nil
}
