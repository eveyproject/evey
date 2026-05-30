package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/codes"
)

// calibrationSvc is the narrow port the calibration HTTP handlers need
// — Recalibrate (the action) and Current (the snapshot for the GET
// endpoint). Lets tests inject a fake without spinning up the real
// CalibrationService + BenchmarkRunner combo.
type calibrationSvc interface {
	Recalibrate(ctx context.Context) error
	Current() domaintranscoding.EncodeCapability
}

// capOverrideLister is the narrow port the overrides GET handler needs.
// Same atomic snapshot the HLS-VOD request path reads.
type capOverrideLister interface {
	ListActive() []domaintranscoding.CapOverride
}

// transcodeCalibrationGetHandler returns the current encode capability
// (read-only). Used by the Server Config admin page to surface "last
// calibrated 3 days ago" + the actual caps the host is enforcing.
func transcodeCalibrationGetHandler(svc calibrationSvc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.transcode.calibration.Get")
		defer span.End()

		if svc == nil {
			http.Error(w, `{"message":"calibration service not wired"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache") // freshness wins on an admin-only path
		if err := json.NewEncoder(w).Encode(svc.Current()); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "encode capability")
			slog.Warn("calibration: GET encode failed", slog.Any("err", err))
		}
	}
}

// transcodeOverridesGetHandler returns the active CMCD-feedback
// adaptive overrides (Phase C). Read-only — surfaces what the worker
// has computed from playback_qoe_events. The admin uses this to see
// "why is the 4K bucket capped to 1080p?" and the worker is the only
// thing that writes it; the UI has no force-reset button by design
// (KISS for Phase C MVP).
func transcodeOverridesGetHandler(lister capOverrideLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.transcode.calibration.OverridesGet")
		defer span.End()

		if lister == nil {
			http.Error(w, `{"message":"override service not wired"}`, http.StatusServiceUnavailable)
			return
		}
		overrides := lister.ListActive()
		if overrides == nil {
			overrides = []domaintranscoding.CapOverride{}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"overrides": overrides,
		}); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "encode overrides")
			slog.Warn("calibration: GET overrides encode failed", slog.Any("err", err))
		}
	}
}

// transcodeRecalibrateHandler triggers a fresh benchmark. Synchronous —
// the admin UI shows a spinner for the ~20 s the bench runs. Returns
// 409 when another recalibration is already in flight so two admins
// clicking the button at once don't queue work.
//
// Long ops detached from the request context: the bench can take
// ~20-60 s on a weak host and the browser may give up before that.
// The result still lands in persistence + in-memory snapshot.
func transcodeRecalibrateHandler(svc calibrationSvc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.transcode.calibration.Recalibrate")
		defer span.End()

		if svc == nil {
			http.Error(w, `{"message":"calibration service not wired"}`, http.StatusServiceUnavailable)
			return
		}
		runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := svc.Recalibrate(runCtx); err != nil {
			if errors.Is(err, apptranscoding.ErrBenchmarkBusy) {
				http.Error(w, `{"message":"recalibration already in progress"}`, http.StatusConflict)
				return
			}
			span.RecordError(err)
			span.SetStatus(codes.Error, "recalibrate failed")
			slog.Warn("calibration: recalibrate failed", slog.Any("err", err))
			http.Error(w, `{"message":"recalibration failed"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(svc.Current())
	}
}
