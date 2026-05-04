package grpcapi

import (
	"context"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IntroCacheInvalidator is the port the handler needs. The postgres repo
// implements it directly; other implementations (e.g. fakes in tests) can
// also satisfy it.
type IntroCacheInvalidator interface {
	Invalidate(ctx context.Context, tmdbID, season, episode int) (int64, error)
}

type adminIntroHandler struct {
	eveyv1.UnimplementedAdminIntroServiceServer
	cache IntroCacheInvalidator
}

func (h *adminIntroHandler) InvalidateIntroCache(ctx context.Context, req *eveyv1.InvalidateIntroCacheRequest) (*eveyv1.InvalidateIntroCacheResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.InvalidateIntroCache")
	defer span.End()

	if req.TmdbId <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tmdb_id must be positive")
	}

	n, err := h.cache.Invalidate(ctx, int(req.TmdbId), int(req.Season), int(req.Episode))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "invalidate cache: %v", err)
	}

	return &eveyv1.InvalidateIntroCacheResponse{RowsDeleted: n}, nil
}
