package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

func snapshotHandler(frameExt FrameExtractor, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, baseURL string, parental ParentalChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spanCtx, span := tracing.StartSpan(r.Context(), "http.snapshot.Capture")
		defer span.End()
		r = r.WithContext(spanCtx)
		mediaID := r.PathValue("id")
		span.SetAttributes(attribute.String("media_item_id", mediaID))
		claims := HTTPClaimsFromContext(r.Context())

		// A stream token is scoped to its one media item - reject replay against
		// another id (the other stream-authed handlers all enforce this). Without
		// it a leaked MOVIE1 token could pull frames of any same-library MOVIE2.
		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(otelcodes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}

		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}
		if item.File == nil || item.File.Path == "" {
			span.SetStatus(otelcodes.Error, "no file")
			http.Error(w, `{"message":"no file"}`, http.StatusNotFound)
			return
		}

		posMs, _ := strconv.ParseInt(r.URL.Query().Get("position_ms"), 10, 64)
		posSec := float64(posMs) / 1000.0
		if posSec < 0 {
			posSec = 0
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		data, err := frameExt.ExtractFrame(ctx, item.File.Path, posSec, 640)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "frame extraction failed")
			logInternal(r.Context(), "snapshot: extract frame", err)
			http.Error(w, `{"message":"frame extraction failed"}`, http.StatusInternalServerError)
			return
		}

		setCORSHeaders(w, r, baseURL)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=5")
		w.Write(data)
	}
}
