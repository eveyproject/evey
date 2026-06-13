package grpcapi

import (
	"context"
	"fmt"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	apprating "github.com/evey-project/evey/internal/app/rating"
	appuser "github.com/evey-project/evey/internal/app/user"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	// Existence check runs for every caller (admin included) so rating on a
	// non-existent mediaItemID returns NotFound instead of falling through
	// to Rate() where FK violation would surface as a misleading
	// "rating must be between 1 and 10" InvalidArgument.
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "not found")
	}
	if claims.IsAdmin || h.userAccess == nil {
		return nil
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(item.LibraryID))
	if accessErr != nil || !ok {
		return status.Error(codes.NotFound, "not found")
	}
	return nil
}

func (h *ratingHandler) PutThumbRating(ctx context.Context, req *eveyv1.PutThumbRatingRequest) (*eveyv1.PutThumbRatingResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.rating.PutThumbRating")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("ratingHandler.PutThumbRating: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		span.RecordError(accessErr)
		span.SetStatus(otelcodes.Error, accessErr.Error())
		return nil, fmt.Errorf("ratingHandler.PutThumbRating: %w", accessErr)
	}
	if req.Value < -1 || req.Value > 1 {
		span.SetStatus(otelcodes.Error, "value must be -1, 0, or 1")
		return nil, status.Error(codes.InvalidArgument, "value must be -1, 0, or 1")
	}
	r := &domainlibrary.UserRating{
		ProfileID:   shared.ProfileID(claims.ProfileID),
		MediaItemID: shared.MediaItemID(req.MediaItemId),
		Value:       int(req.Value),
	}
	if upsertErr := h.starSvc.UpsertThumb(ctx, r); upsertErr != nil {
		span.RecordError(upsertErr)
		span.SetStatus(otelcodes.Error, upsertErr.Error())
		slog.Error("upsert thumb rating", slog.Any("err", upsertErr))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &eveyv1.PutThumbRatingResponse{Value: req.Value}, nil
}

func (h *ratingHandler) GetMyThumbRating(ctx context.Context, req *eveyv1.GetMyThumbRatingRequest) (*eveyv1.GetMyThumbRatingResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.rating.GetMyThumbRating")
	defer span.End()
	_, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("ratingHandler.GetMyThumbRating: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		span.RecordError(accessErr)
		span.SetStatus(otelcodes.Error, accessErr.Error())
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


func (h *ratingHandler) GetRatingAggregate(ctx context.Context, req *eveyv1.GetRatingAggregateRequest) (*eveyv1.GetRatingAggregateResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.rating.GetRatingAggregate")
	defer span.End()
	_, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("ratingHandler.GetRatingAggregate: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		span.RecordError(accessErr)
		span.SetStatus(otelcodes.Error, accessErr.Error())
		return nil, fmt.Errorf("ratingHandler.GetRatingAggregate: %w", accessErr)
	}
	// The legacy 1-10 star-rating aggregate (the `ratings` table) was removed;
	// thumbs (like/dislike) are the single rating model and aren't aggregated
	// here. Returns empty for proto/client compatibility — no frontend caller
	// today; a deliberate proto cleanup can drop this RPC later.
	return &eveyv1.GetRatingAggregateResponse{Average: 0, Count: 0}, nil
}
