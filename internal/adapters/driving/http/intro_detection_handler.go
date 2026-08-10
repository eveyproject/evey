package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	applibrary "github.com/evey-project/evey/internal/app/library"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// introDetectionHandler handles POST /api/v1/admin/media/{id}/detect-segments.
// Admin-only. Triggers detection asynchronously and returns 202 Accepted immediately.
func introDetectionHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger, libSvc MediaStreamsGetter, libraryAccess LibraryAccessChecker) http.HandlerFunc {
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
		// Resolve the item before detaching any work: an unknown id must be a
		// 404, not a fire-and-forget goroutine that fails later in the logs.
		// The access check is defense-in-depth (the route is admin-gated, and
		// checkLibraryAccess short-circuits for admins) so this handler stays
		// safe if it is ever re-homed on a non-admin route.
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, HTTPClaimsFromContext(r.Context()), item.LibraryID, libraryAccess) {
			return
		}

		isContainer := item.Type == domainlibrary.Season || item.Type == domainlibrary.Series

		// Dry-run: run detection synchronously and return the segments it WOULD
		// store, without persisting. Used by the offline eval harness and the
		// admin intro-test tool to score the detector without clobbering the
		// stored truth via upsert.
		if r.URL.Query().Get("dry_run") == "true" {
			// A container has no file to analyse; the caller samples a
			// concrete episode itself. 400 beats a misleading empty result.
			if isContainer {
				http.Error(w, `{"message":"dry-run needs an episode or movie id"}`, http.StatusBadRequest)
				return
			}
			segs, err := svc.DetectIntroDryRun(r.Context(), mediaID)
			if err != nil {
				slog.WarnContext(ctx, "intro detection dry-run failed",
					slog.String("media_id", mediaID), slog.Any("err", err))
				http.Error(w, `{"message":"detection failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if encErr := json.NewEncoder(w).Encode(toSegmentDTOs(segs)); encErr != nil {
				slog.WarnContext(ctx, "intro detection dry-run encode failed",
					slog.String("media_id", mediaID), slog.Any("err", encErr))
			}
			return
		}

		// Detach from the request context so the goroutine outlives the HTTP
		// response. Containers run the season pipeline (cluster included) -
		// the tool's "apply" granularity; file media keeps the per-episode path.
		detached := context.WithoutCancel(r.Context())
		itemType := item.Type
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.ErrorContext(detached, "panic in intro detection goroutine",
						slog.String("media_id", mediaID),
						slog.Any("recover", rec),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}()
			var err error
			switch itemType {
			case domainlibrary.Season:
				err = svc.DetectSeason(detached, mediaID)
			case domainlibrary.Series:
				_, err = svc.DetectSeries(detached, mediaID)
			default:
				err = svc.DetectIntro(detached, mediaID)
			}
			if err != nil {
				slog.WarnContext(detached, "intro detection failed",
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
func libraryIntroBackfillHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger, libraryAccess LibraryAccessChecker) http.HandlerFunc {
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
		// Defense-in-depth: admin short-circuits inside checkLibraryAccess, but
		// the guard keeps this handler safe if it ever leaves the admin group.
		if !checkLibraryAccess(r.Context(), w, HTTPClaimsFromContext(r.Context()), shared.LibraryID(libraryID), libraryAccess) {
			return
		}
		detached := context.WithoutCancel(r.Context())
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.ErrorContext(detached, "panic in library intro backfill goroutine",
						slog.String("library_id", libraryID),
						slog.Any("recover", rec),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}()
			n, err := svc.BackfillLibrary(detached, libraryID)
			if err != nil {
				// A duplicate trigger is already logged by the service as an
				// ignored no-op; only real failures deserve a warning.
				if !errors.Is(err, applibrary.ErrIntroBackfillAlreadyRunning) {
					slog.WarnContext(detached, "library intro backfill failed",
						slog.String("library_id", libraryID), slog.Any("err", err))
				}
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

// segmentDTO is the wire shape of a detected segment.
//
// The domain type was encoded directly here, and it carries no struct tags, so
// the JSON keys were bare Go field names ("ID", "MediaItemID") - unlike every
// other endpoint. That also meant renaming a domain field silently changed this
// payload with nothing to catch it: the offline eval harness that consumes
// ?dry_run=true would just stop scoring, with no compile error anywhere.
type segmentDTO struct {
	ID          string  `json:"id"`
	MediaItemID string  `json:"mediaItemId"`
	Type        string  `json:"type"`
	StartMs     int64   `json:"startMs"`
	EndMs       int64   `json:"endMs"`
	Confidence  float64 `json:"confidence"`
	Source      string  `json:"source"`
}

func toSegmentDTOs(segs []domainlibrary.MediaSegment) []segmentDTO {
	// Non-nil so an empty result encodes as [] rather than null - a harness
	// iterating the response should not have to special-case it.
	out := make([]segmentDTO, 0, len(segs))
	for _, s := range segs {
		out = append(out, segmentDTO{
			ID:          s.ID,
			MediaItemID: string(s.MediaItemID),
			Type:        s.Type,
			StartMs:     s.StartMs,
			EndMs:       s.EndMs,
			Confidence:  s.Confidence,
			Source:      s.Source,
		})
	}
	return out
}
