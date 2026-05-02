package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// registerArtworkAdminRoutes mounts the three artwork-cache admin endpoints:
//
//   GET  /api/v1/admin/artworks/stats
//   POST /api/v1/admin/artworks/download-all
//   GET  /api/v1/admin/artworks/download-all/{jobId}
//
// dataDir is the server data directory (e.g. "/data"); it is used to derive
// local storage paths for downloaded artworks.
func registerArtworkAdminRoutes(
	mux *http.ServeMux,
	metaCmd *appmetadata.CommandService,
	tokenGen TokenVerifier,
	serverCtx context.Context,
	dataDir string,
) {
	// Ensure the CommandService knows where to store artworks.
	// auto-download on scan is disabled by default; the toggle is surfaced via
	// the server-config endpoint rather than wired here.
	if dataDir != "" {
		metaCmd.SetAutoDownload(dataDir, false)
	}
	admin := httpAdminMiddleware(tokenGen)

	// GET /api/v1/admin/artworks/stats
	mux.HandleFunc("GET /api/v1/admin/artworks/stats", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.admin.ArtworkStats")
		defer span.End()

		stats, err := metaCmd.ArtworkStats(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			slog.Warn("artwork stats", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		resp := map[string]any{
			"totalRecords":     stats.TotalRecords,
			"withLocalPath":    stats.WithLocalPath,
			"missingLocalPath": stats.MissingLocalPath,
			"totalSizeBytes":   stats.TotalSizeBytes,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("artwork stats: json encode", slog.Any("err", err))
		}
	}))

	// POST /api/v1/admin/artworks/download-all — starts background bulk download.
	// Returns immediately with jobId + totalToDownload.
	mux.HandleFunc("POST /api/v1/admin/artworks/download-all", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.admin.ArtworkDownloadAll")
		defer span.End()

		// listing uses request ctx; background goroutine uses server ctx
		jobID, total, err := metaCmd.StartBulkDownload(ctx, serverCtx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			slog.Warn("artwork download-all: start", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		resp := map[string]any{
			"jobId":           jobID,
			"totalToDownload": total,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("artwork download-all: json encode", slog.Any("err", err))
		}
	}))

	// GET /api/v1/admin/artworks/download-all/{jobId} — poll job progress.
	mux.HandleFunc("GET /api/v1/admin/artworks/download-all/{jobId}", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.admin.ArtworkDownloadAllStatus")
		defer span.End()
		r = r.WithContext(ctx)

		jobID := r.PathValue("jobId")
		job := metaCmd.GetDownloadJob(jobID)
		if job == nil {
			span.SetStatus(otelcodes.Error, "job not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		status := "running"
		if job.Done.Load() {
			status = "done"
		}
		processed := job.Processed.Load()
		errs := job.ListErrors()
		if errs == nil {
			errs = []string{}
		}
		resp := map[string]any{
			"status":    status,
			"processed": processed,
			"total":     job.Total,
			"errors":    errs,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("artwork download-all status: json encode", slog.Any("err", err))
		}
	}))
}
