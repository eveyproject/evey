package httpapi

// Serving the preview clip an operator watches to judge a desync.
//
// It is a plain HTTP route rather than a gRPC method because it answers with a
// video, and the browser has to be able to hand it straight to a <video> tag.
// It lives in its own file, with its own register function, so it can be wired
// without touching the metadata route group.

import (
	"net/http"
	"strconv"

	otelcodes "go.opentelemetry.io/otel/codes"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// syncPreviewMaxBytes caps what this route will send. A 45 s clip at 854px
// lands around 2 MB; anything an order of magnitude past that means the
// renderer misbehaved and the browser should not be handed it.
const syncPreviewMaxBytes = 64 << 20

// registerSyncPreviewRoute wires GET /api/v1/admin/media/{id}/sync-preview.
// Rendering a clip runs ffmpeg, so it goes behind the heavy limiter.
func registerSyncPreviewRoute(mux *http.ServeMux, seekIndex *appmaintenance.SeekIndexService, tokenGen domainuser.TokenGenerator, heavyRL *ratelimit.Limiter) {
	if seekIndex == nil {
		return
	}
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/admin/media/{id}/sync-preview",
		ratelimit.MiddlewareFunc(heavyRL, admin(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracing.StartSpan(r.Context(), "http.admin.SyncPreview")
			defer span.End()

			q := r.URL.Query()
			req := appmaintenance.SyncPreviewRequest{
				AtSec:    floatQuery(q.Get("at"), 600),
				LenSec:   floatQuery(q.Get("len"), 20),
				Track:    int(floatQuery(q.Get("track"), 0)),
				ShiftSec: floatQuery(q.Get("shift"), 0),
			}
			clip, err := seekIndex.SyncPreviewOfMedia(ctx, r.PathValue("id"), req)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, err.Error())
				messageError(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			if len(clip) > syncPreviewMaxBytes {
				http.Error(w, `{"message":"preview too large"}`, http.StatusInsufficientStorage)
				return
			}
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", strconv.Itoa(len(clip)))
			// Every render is a fresh judgement of a candidate offset: a cached
			// clip would answer the previous question.
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(clip)
		})))
}

// floatQuery reads a numeric query parameter, falling back to def. A control
// the operator drags can send anything; the service clamps the rest.
func floatQuery(raw string, def float64) float64 {
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return v
}
