package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PUT /api/v1/admin/media/{id}/segments
// Body: {"type":"intro","startMs":45000,"endMs":60000}
// Sets a manual segment. Idempotent on (media_id, type) - replaces auto detection.
func setSegmentHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	admin := httpAdminMiddleware(tokenGen)
	return admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.segments.SetManual")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		if mediaID == "" {
			http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
			return
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.SetStatus(codes.Error, "media not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		claims := HTTPClaimsFromContext(r.Context())
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		var body struct {
			Type    string `json:"type"`
			StartMs int64  `json:"startMs"`
			EndMs   int64  `json:"endMs"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
		if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
			http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if err := svc.SetManualSegment(r.Context(), mediaID, body.Type, body.StartMs, body.EndMs); err != nil {
			span.RecordError(err)
			slog.WarnContext(ctx, "set manual segment failed", slog.String("media_id", mediaID), slog.Any("err", err))
			messageError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

// DELETE /api/v1/admin/media/{id}/segments?type=intro
func deleteSegmentHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	admin := httpAdminMiddleware(tokenGen)
	return admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.segments.Delete")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		segType := strings.TrimSpace(r.URL.Query().Get("type"))
		if mediaID == "" || segType == "" {
			http.Error(w, `{"message":"missing media id or type"}`, http.StatusBadRequest)
			return
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		claims := HTTPClaimsFromContext(r.Context())
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		if err := svc.DeleteSegment(r.Context(), mediaID, segType); err != nil {
			span.RecordError(err)
			logInternal(r.Context(), "segments: delete", err)
			http.Error(w, `{"message":"delete failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

// POST /api/v1/admin/media/{id}/segments/propagate?type=intro
// Returns: {"propagatedTo": ["id1","id2",...], "count": N}
func propagateSegmentHandler(tokenGen TokenVerifier, svc IntroDetectionTrigger, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	admin := httpAdminMiddleware(tokenGen)
	return admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.segments.Propagate")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		segType := strings.TrimSpace(r.URL.Query().Get("type"))
		if mediaID == "" || segType == "" {
			http.Error(w, `{"message":"missing media id or type"}`, http.StatusBadRequest)
			return
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		claims := HTTPClaimsFromContext(r.Context())
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		ids, err := svc.PropagateSegment(r.Context(), mediaID, segType)
		if err != nil {
			span.RecordError(err)
			slog.WarnContext(ctx, "propagate failed", slog.String("media_id", mediaID), slog.Any("err", err))
			messageError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]any{
			"propagatedTo": ids,
			"count":        len(ids),
		}); encErr != nil {
			slog.WarnContext(ctx, "encode propagate response", slog.Any("err", encErr))
		}
	})
}
