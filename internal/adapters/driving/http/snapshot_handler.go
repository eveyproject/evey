package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

func snapshotHandler(tokenGen TokenVerifier, frameExt FrameExtractor, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spanCtx, span := tracing.StartSpan(r.Context(), "http.snapshot.Capture")
		defer span.End()
		r = r.WithContext(spanCtx)
		mediaID := r.PathValue("id")
		span.SetAttributes(attribute.String("media_item_id", mediaID))
		claims := HTTPClaimsFromContext(r.Context())

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
			http.Error(w, `{"message":"frame extraction failed"}`, http.StatusInternalServerError)
			return
		}

		setCORSHeaders(w, r, baseURL)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=5")
		w.Write(data)
	}
}
