package grpcapi

import (
	"context"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type activityHandler struct {
	eveyv1.UnimplementedActivityServiceServer
	svc *appactivity.QueryService
}

func (h *activityHandler) GetPlayHistory(ctx context.Context, req *eveyv1.GetPlayHistoryRequest) (*eveyv1.GetPlayHistoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	filter := activity.PlayHistoryFilter{
		ProfileID:   profileID,
		MediaItemID: req.MediaItemId,
		From:        fromTimestamppbPtr(req.From),
		To:          fromTimestamppbPtr(req.To),
	}
	if req.Pagination != nil {
		filter.Limit = int(req.Pagination.PageSize)
		filter.Offset = int((req.Pagination.Page - 1) * req.Pagination.PageSize)
	}
	entries, err := h.svc.GetPlayHistory(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get play history: %v", err)
	}
	resp := &eveyv1.GetPlayHistoryResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, playHistoryEntryToProto(e))
	}
	return resp, nil
}

func (h *activityHandler) GetUserStats(ctx context.Context, req *eveyv1.GetUserStatsRequest) (*eveyv1.UserStats, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, err
	}
	stats, err := h.svc.GetUserStats(ctx, profileID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get user stats: %v", err)
	}
	pb := &eveyv1.UserStats{
		ProfileId:        stats.ProfileID,
		TotalWatchTimeMs: stats.TotalWatchTimeMs,
		TotalPlays:       int32(stats.TotalPlayCount),
	}
	for genre, ms := range stats.GenreBreakdown {
		pb.Genres = append(pb.Genres, &eveyv1.GenreBreakdown{
			Genre:       genre,
			TotalTimeMs: ms,
		})
	}
	for _, mp := range stats.MostPlayedItems {
		pb.MostPlayed = append(pb.MostPlayed, &eveyv1.MostPlayed{
			MediaItemId: mp.MediaItemID,
			Title:       mp.Title,
			PlayCount:   int32(mp.PlayCount),
		})
	}
	return pb, nil
}

func (h *activityHandler) GetServerStats(ctx context.Context, _ *eveyv1.GetServerStatsRequest) (*eveyv1.ServerStats, error) {
	stats, err := h.svc.GetServerStats(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get server stats: %v", err)
	}
	return &eveyv1.ServerStats{
		TotalUsers:      int32(stats.TotalUsers),
		TotalMediaItems: int32(stats.TotalItems),
	}, nil
}

func (h *activityHandler) GetLoginHistory(ctx context.Context, req *eveyv1.GetLoginHistoryRequest) (*eveyv1.GetLoginHistoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	// Non-admin can only view their own login history
	userID := req.UserId
	if userID == "" {
		userID = claims.UserID
	} else if userID != claims.UserID && !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "cannot access another user's login history")
	}
	limit := 50
	offset := 0
	if req.Pagination != nil {
		limit = int(req.Pagination.PageSize)
		offset = int((req.Pagination.Page - 1) * req.Pagination.PageSize)
	}
	entries, err := h.svc.GetLoginHistory(ctx, userID, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get login history: %v", err)
	}
	resp := &eveyv1.GetLoginHistoryResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, loginEntryToProto(e))
	}
	return resp, nil
}

func (h *activityHandler) ListServerMetrics(ctx context.Context, req *eveyv1.ListServerMetricsRequest) (*eveyv1.ListServerMetricsResponse, error) {
	from := fromTimestamppb(req.From)
	to := fromTimestamppb(req.To)
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	metrics, err := h.svc.GetServerMetrics(ctx, from, to)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get metrics: %v", err)
	}
	resp := &eveyv1.ListServerMetricsResponse{}
	for _, m := range metrics {
		resp.Metrics = append(resp.Metrics, serverMetricToProto(m))
	}
	return resp, nil
}
