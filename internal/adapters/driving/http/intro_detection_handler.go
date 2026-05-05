package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

// introDetectionHandler handles POST /api/v1/admin/media/{id}/detect-segments.
// Admin-only. Triggers detection asynchronously and returns 202 Accepted immediately.
func introDetectionHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger) http.HandlerFunc {
	admin := httpAdminMiddleware(tokenGen)
	return admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.intro.DetectSegments")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		if mediaID == "" {
			http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
			return
		}

		// Detach from the request context so the goroutine outlives the HTTP response.
		detached := context.WithoutCancel(r.Context())
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("panic in intro detection goroutine",
						slog.String("media_id", mediaID),
						slog.Any("recover", rec),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}()
			if err := svc.DetectIntro(detached, mediaID); err != nil {
				slog.Warn("intro detection failed",
					slog.String("media_id", mediaID),
					slog.Any("err", err),
				)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"started"}`))
	})
}
