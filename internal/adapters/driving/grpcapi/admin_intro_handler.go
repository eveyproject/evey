package grpcapi

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"

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

// IntroBackfiller is the port for the admin-triggered backfill.
type IntroBackfiller interface {
	BackfillLibrary(ctx context.Context, libraryID string) (int, error)
}

type adminIntroHandler struct {
	eveyv1.UnimplementedAdminIntroServiceServer
	cache           IntroCacheInvalidator
	backfill        IntroBackfiller
	backfillMu      sync.Mutex
	backfillRunning bool
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

func (h *adminIntroHandler) BackfillIntroDetection(ctx context.Context, req *eveyv1.BackfillIntroDetectionRequest) (*eveyv1.BackfillIntroDetectionResponse, error) {
	_, span := tracing.StartSpan(ctx, "grpc.admin.BackfillIntroDetection")
	defer span.End()

	if h.backfill == nil {
		return nil, status.Errorf(codes.Unavailable, "intro detection not configured")
	}

	h.backfillMu.Lock()
	if h.backfillRunning {
		h.backfillMu.Unlock()
		// Return AlreadyExists so the caller knows the job is already running —
		// no need for a second one.
		return nil, status.Errorf(codes.AlreadyExists, "backfill already in progress")
	}
	h.backfillRunning = true
	h.backfillMu.Unlock()

	libraryID := req.LibraryId
	// ADR-0007: detached ctx + defer recover. No outer timeout — a full library
	// backfill can take hours on a cold catalogue and is bounded by the
	// service's own concurrency limiter, not by a wall-clock deadline.
	go func() {
		defer func() {
			h.backfillMu.Lock()
			h.backfillRunning = false
			h.backfillMu.Unlock()
			if r := recover(); r != nil {
				slog.Error("panic recovered in admin intro backfill",
					slog.Any("panic", r),
					slog.String("stack", string(debug.Stack())))
			}
		}()
		_, _ = h.backfill.BackfillLibrary(context.Background(), libraryID)
	}()

	// seasons_scheduled is not available synchronously in non-blocking mode —
	// the job has just been enqueued. The caller should watch server logs for
	// progress (slog entries from BackfillLibrary).
	return &eveyv1.BackfillIntroDetectionResponse{SeasonsScheduled: 0}, nil
}
