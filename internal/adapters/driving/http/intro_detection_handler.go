package httpapi

import (
	"context"
	"encoding/json"
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

		// Dry-run: run detection synchronously and return the segments it WOULD
		// store, without persisting. Used by the offline eval harness to score the
		// detector against a known ground truth without clobbering it via upsert.
		if r.URL.Query().Get("dry_run") == "true" {
			segs, err := svc.DetectIntroDryRun(r.Context(), mediaID)
			if err != nil {
				slog.Warn("intro detection dry-run failed",
					slog.String("media_id", mediaID), slog.Any("err", err))
				http.Error(w, `{"message":"detection failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if encErr := json.NewEncoder(w).Encode(segs); encErr != nil {
				slog.Warn("intro detection dry-run encode failed",
					slog.String("media_id", mediaID), slog.Any("err", encErr))
			}
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

// libraryIntroBackfillHandler handles POST /api/v1/admin/libraries/{id}/detect-intros.
// Admin-only. Re-runs intro/segment detection across EVERY season of the library
// (not just seasons missing segments) asynchronously, returning 202 Accepted.
// This is the supported way to force a re-detection after a detection quality
// improvement + an introDetectionAlgoVersion bump.
func libraryIntroBackfillHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger) http.HandlerFunc {
	admin := httpAdminMiddleware(tokenGen)
	return admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.intro.BackfillLibrary")
		defer span.End()
		r = r.WithContext(ctx)
		libraryID := r.PathValue("id")
		if libraryID == "" {
			http.Error(w, `{"message":"missing library id"}`, http.StatusBadRequest)
			return
		}
		detached := context.WithoutCancel(r.Context())
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("panic in library intro backfill goroutine",
						slog.String("library_id", libraryID),
						slog.Any("recover", rec),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}()
			n, err := svc.BackfillLibrary(detached, libraryID)
			if err != nil {
				slog.Warn("library intro backfill failed",
					slog.String("library_id", libraryID), slog.Any("err", err))
				return
			}
			slog.Info("library intro backfill scheduled",
				slog.String("library_id", libraryID), slog.Int("seasons", n))
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"started"}`))
	})
}
