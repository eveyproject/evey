package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	appconfig "github.com/evey-project/evey/internal/app/config"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
)

// configKeyArtworkAutoDownload toggles fire-and-forget artwork download on
// scan-time metadata enrichment. Default is ON — without this 99% of users
// see broken posters after a scan because the metadata enricher creates
// artwork rows with external source URLs but never fetches them, and the
// browser refuses to display cross-origin images via Opaque Response
// Blocking. Power users wanting to save API quota / bandwidth can flip the
// key to "false" via the server-config endpoint and restart.
const configKeyArtworkAutoDownload = "artwork.auto_download"

// resolveAutoDownloadDefault reads the server-config toggle. Absent key →
// true (the new default). Explicit "false" → false. Anything else → true
// (conservative parse: don't silently fall through if the admin set
// something non-bool).
func resolveAutoDownloadDefault(ctx context.Context, cfg *appconfig.Service) bool {
	if cfg == nil {
		return true
	}
	entry, err := cfg.Get(ctx, configKeyArtworkAutoDownload)
	if err != nil {
		if !errors.Is(err, shared.ErrNotFound) {
			slog.Warn("read artwork.auto_download config", slog.Any("err", err))
		}
		return true
	}
	if entry == nil {
		return true
	}
	return entry.Value != "false"
}

// registerArtworkAdminRoutes mounts the three artwork-cache admin endpoints:
//
//	GET  /api/v1/admin/artworks/stats
//	POST /api/v1/admin/artworks/download-all
//	GET  /api/v1/admin/artworks/download-all/{jobId}
//
// dataDir is the server data directory (e.g. "/data"); it is used to derive
// local storage paths for downloaded artworks.
func registerArtworkAdminRoutes(
	mux *http.ServeMux,
	metaCmd *appmetadata.CommandService,
	tokenGen TokenVerifier,
	serverCtx context.Context,
	dataDir string,
	configSvc *appconfig.Service,
) {
	// Ensure the CommandService knows where to store artworks. Default ON
	// (see configKeyArtworkAutoDownload). Admins can override via the
	// server-config endpoint; takes effect on next restart.
	if dataDir != "" {
		metaCmd.SetAutoDownload(dataDir, resolveAutoDownloadDefault(serverCtx, configSvc))
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

	// POST /api/v1/admin/artworks/retry-missing — one-shot best-effort retry
	// of artwork rows that still don't have a LocalPath. Same logic as the
	// hourly RetryMissingArtworksLoop, exposed as an explicit admin button
	// for when an operator just toggled auto-download back on or fixed a
	// broken provider key and wants the cache populated now rather than
	// waiting for the next tick.
	mux.HandleFunc("POST /api/v1/admin/artworks/retry-missing", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.ArtworkRetryMissing")
		defer span.End()

		// Detach: RetryMissingArtworks lists every artwork without a local path
		// and re-downloads them across 4 workers — minutes for a large catalogue.
		// Blocking the request ctx 504s behind a proxy and cancels the work
		// mid-pass. Run it in the background (serverCtx) and return 202; the
		// pass result lands in the logs (same shape as download-all).
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("recovered panic in artwork retry-missing", slog.Any("error", rec))
				}
			}()
			succeeded, failed := metaCmd.RetryMissingArtworks(serverCtx)
			slog.Info("artwork retry-missing complete", slog.Int("succeeded", succeeded), slog.Int("failed", failed))
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(map[string]any{"started": true}); err != nil {
			slog.Warn("artwork retry-missing: json encode", slog.Any("err", err))
		}
	}))

	// POST /api/v1/admin/artworks/backfill-blurhashes — recompute placeholder
	// hashes for every artwork row that has a LocalPath on disk but no
	// blurhash. Lets admins retro-fit the field onto artworks ingested before
	// migration 000095 without re-downloading. Runs synchronously: decode is
	// CPU-bound at ~50 ms per image with 2 workers, so a 10k-row catalogue
	// finishes in ~5 minutes; same shape as retry-missing for symmetry.
	mux.HandleFunc("POST /api/v1/admin/artworks/backfill-blurhashes", admin(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.ArtworkBackfillBlurhashes")
		defer span.End()

		// Detach: BackfillBlurhashes decodes every artwork lacking a hash across
		// 2 workers — self-documented ~5 min per 10k catalogue, well past the
		// 60s default proxy read-timeout. Blocking 504s and cancels the work.
		// Run it in the background (serverCtx) and return 202.
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("recovered panic in artwork backfill-blurhashes", slog.Any("error", rec))
				}
			}()
			succeeded, failed := metaCmd.BackfillBlurhashes(serverCtx)
			slog.Info("artwork backfill-blurhashes complete", slog.Int("succeeded", succeeded), slog.Int("failed", failed))
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(map[string]any{"started": true}); err != nil {
			slog.Warn("artwork backfill-blurhashes: json encode", slog.Any("err", err))
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
