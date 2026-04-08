package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ratingHandler struct {
	eveyv1.UnimplementedRatingServiceServer
	starSvc    *apprating.Service
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
}

func (h *ratingHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if claims.IsAdmin || h.userAccess == nil {
		return nil
	}
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "not found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, claims.UserID, item.LibraryID)
	if accessErr != nil || !ok {
		return status.Error(codes.NotFound, "not found")
	}
	return nil
}

func (h *ratingHandler) PutThumbRating(ctx context.Context, req *eveyv1.PutThumbRatingRequest) (*eveyv1.PutThumbRatingResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("ratingHandler.PutThumbRating: %w", err)
	}
	if req.Value < -1 || req.Value > 1 {
		return nil, status.Error(codes.InvalidArgument, "value must be -1, 0, or 1")
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("ratingHandler.PutThumbRating: %w", accessErr)
	}
	r := &domainlibrary.UserRating{
		ProfileID:   claims.ProfileID,
		MediaItemID: req.MediaItemId,
		Value:       int(req.Value),
	}
	if upsertErr := h.starSvc.UpsertThumb(ctx, r); upsertErr != nil {
		slog.Error("upsert thumb rating", slog.Any("err", upsertErr))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &eveyv1.PutThumbRatingResponse{Value: req.Value}, nil
}

func (h *ratingHandler) GetMyThumbRating(ctx context.Context, req *eveyv1.GetMyThumbRatingRequest) (*eveyv1.GetMyThumbRatingResponse, error) {
	_, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("ratingHandler.GetMyThumbRating: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("ratingHandler.GetMyThumbRating: %w", accessErr)
	}
	claims := interceptors.ClaimsFromContext(ctx)
	thumbRating, getErr := h.starSvc.GetThumb(ctx, claims.ProfileID, req.MediaItemId)
	if getErr != nil {
		slog.Debug("get thumb rating not found", slog.Any("err", getErr))
	}
	if thumbRating == nil {
		return &eveyv1.GetMyThumbRatingResponse{Value: 0}, nil
	}
	return &eveyv1.GetMyThumbRatingResponse{Value: int32(thumbRating.Value)}, nil
}

func (h *ratingHandler) PostStarRating(ctx context.Context, req *eveyv1.PostStarRatingRequest) (*eveyv1.PostStarRatingResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("ratingHandler.PostStarRating: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("ratingHandler.PostStarRating: %w", accessErr)
	}
	if rateErr := h.starSvc.Rate(ctx, req.MediaItemId, claims.ProfileID, int(req.Value)); rateErr != nil {
		slog.Error("star rating", slog.Any("err", rateErr))
		return nil, status.Error(codes.InvalidArgument, "rating must be between 1 and 10")
	}
	return &eveyv1.PostStarRatingResponse{Value: req.Value}, nil
}

func (h *ratingHandler) GetStarRating(ctx context.Context, req *eveyv1.GetStarRatingRequest) (*eveyv1.GetStarRatingResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("ratingHandler.GetStarRating: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("ratingHandler.GetStarRating: %w", accessErr)
	}
	rt, getErr := h.starSvc.GetRating(ctx, req.MediaItemId, claims.ProfileID)
	if getErr != nil {
		slog.Error("get star rating", slog.Any("err", getErr))
		return nil, status.Error(codes.Internal, "internal error")
	}
	if rt == nil {
		return &eveyv1.GetStarRatingResponse{Value: 0}, nil
	}
	return &eveyv1.GetStarRatingResponse{Value: int32(rt.Value.Int())}, nil
}

func (h *ratingHandler) DeleteStarRating(ctx context.Context, req *eveyv1.DeleteStarRatingRequest) (*emptypb.Empty, error) {
	_, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("ratingHandler.DeleteStarRating: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("ratingHandler.DeleteStarRating: %w", accessErr)
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if delErr := h.starSvc.DeleteRating(ctx, req.MediaItemId, claims.ProfileID); delErr != nil {
		slog.Error("delete star rating", slog.Any("err", delErr))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &emptypb.Empty{}, nil
}

func (h *ratingHandler) GetRatingAggregate(ctx context.Context, req *eveyv1.GetRatingAggregateRequest) (*eveyv1.GetRatingAggregateResponse, error) {
	_, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("ratingHandler.GetRatingAggregate: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("ratingHandler.GetRatingAggregate: %w", accessErr)
	}
	aggs, aggErr := h.starSvc.GetAggregates(ctx, []string{req.MediaItemId})
	if aggErr != nil {
		slog.Error("get rating aggregate", slog.Any("err", aggErr))
		return nil, status.Error(codes.Internal, "internal error")
	}
	agg := aggs[req.MediaItemId]
	return &eveyv1.GetRatingAggregateResponse{
		Average: agg.Average,
		Count:   int32(agg.Count),
	}, nil
}
