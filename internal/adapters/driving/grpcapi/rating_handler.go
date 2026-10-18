package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ratingHandler struct {
	eveyv1.UnimplementedRatingServiceServer
	// thumbs is the port itself: a thumb is stored as given and read back as
	// stored, with no rule between the two for an app service to own.
	thumbs     domainlibrary.UserRatingRepository
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
	translator *eveyi18n.Translator
}

func (h *ratingHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	// Existence check runs for every caller (admin included) so rating on a
	// non-existent mediaItemID returns NotFound instead of falling through
	// to Rate() where FK violation would surface as a misleading
	// "rating must be between 1 and 10" InvalidArgument.
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.general.not_found")
	}
	if claims.IsAdmin {
		return nil
	}
	if h.userAccess == nil {
		return errUserAccessNotConfigured(ctx, "ratingHandler.requireMediaAccess")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accessErr != nil || !ok {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.general.not_found")
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
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.rating.invalid_value")
	}
	r := &domainlibrary.UserRating{
		ProfileID:   shared.ProfileID(claims.ProfileID),
		MediaItemID: shared.MediaItemID(req.MediaItemId),
		Value:       int(req.Value),
	}
	if upsertErr := h.thumbs.Upsert(ctx, r); upsertErr != nil {
		span.RecordError(upsertErr)
		span.SetStatus(otelcodes.Error, upsertErr.Error())
		slog.ErrorContext(ctx, "upsert thumb rating", slog.Any("err", upsertErr))
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
	thumbRating, getErr := h.thumbs.Get(ctx, claims.ProfileID, req.MediaItemId)
	if getErr != nil && !errors.Is(getErr, shared.ErrNotFound) {
		// Value: 0 is a neutral thumb, which is a STATEMENT about what the
		// viewer chose. On a failed read the server does not know what they
		// chose, and a client acting on that answer overwrites an opinion it
		// never saw. The repository already tells the two apart.
		span.RecordError(getErr)
		return nil, internalError(ctx, "get thumb rating", getErr)
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
	// here. Returns empty for proto/client compatibility - no frontend caller
	// today; a deliberate proto cleanup can drop this RPC later.
	return &eveyv1.GetRatingAggregateResponse{Average: 0, Count: 0}, nil
}
