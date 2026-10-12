package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/codes"
)

// calibrationSvc is the narrow port the calibration HTTP handlers need
// - Recalibrate (the action) and Current (the snapshot for the GET
// endpoint). Lets tests inject a fake without spinning up the real
// CalibrationService + BenchmarkRunner combo.
type calibrationSvc interface {
	Recalibrate(ctx context.Context) error
	Current() domaintranscoding.HostCapabilitySnapshot
	// HWMode is the active HW-acceleration policy (auto/off/nvenc/vaapi/qsv) so
	// the admin card can show what the host is configured to do, distinct from
	// the active backend the bench actually selected.
	HWMode() domaintranscoding.HWMode
	// RecentFallbacks surfaces the level-2 HW-demotion events (newest last) so
	// the admin can see a degraded host without log diving.
	RecentFallbacks() []apptranscoding.HWFallbackEvent
	// Status is the calibration lifecycle (pending / benchmarking / provisional
	// / calibrated), so the card can say a bench is waiting for an idle host
	// instead of presenting the interim ceilings as final.
	Status() apptranscoding.CalibrationStatus
}

// capOverrideLister is the narrow port the overrides GET handler needs.
// Same atomic snapshot the HLS-VOD request path reads.
type capOverrideLister interface {
	ListActive() []domaintranscoding.CapOverride
}

// capOverrideResetter is the narrow port the override-reset endpoint
// needs. Defined separately from capOverrideLister so handler tests
// can mock only what they actually exercise.
type capOverrideResetter interface {
	ResetOverride(ctx context.Context, sourceClass string) (bool, error)
}

// publicEncodeCapability is the admin-API view of EncodeCapability. It shadows
// two host-internal fields so they don't leak over HTTP: HWDevice (the DRM
// render-node path, e.g. /dev/dri/renderD128) and CPUSignature (the host CPU
// identity hash). The admin calibration UI uses neither; every other field is
// promoted flat. The domain struct keeps its json tags (it's persisted as JSON
// to the capability store) - the redaction lives only at this boundary.
type publicEncodeCapability struct {
	domaintranscoding.EncodeCapability
	// Same json names as the embedded fields so these (zero-value) shallower
	// fields win the name conflict; omitempty then drops them from the output.
	// (json:"-" would NOT work - ignored fields don't shadow the embedded ones.)
	HWDevice     string `json:"hw_device,omitempty"`
	CPUSignature string `json:"cpu_signature,omitempty"`
}

// routeCostView is one measured route as the admission engine prices it, for
// the admin panel: the identity (backend, encoder, route class, rung), the warm
// single-session measurement (fps, CPU cores), and the derived per-pool cost
// (accelerator fraction + normalised CPU) at the source frame rate it was
// measured on.
type routeCostView struct {
	Backend   string  `json:"backend"`
	Encoder   string  `json:"encoder"`
	Route     string  `json:"route"`
	Height    int     `json:"height"`
	FPS       float64 `json:"fps"`
	CPUCores  float64 `json:"cpu_cores"`
	SourceFPS float64 `json:"source_fps"`
	AccelCost float64 `json:"accel_cost"`
	CPUCost   float64 `json:"cpu_cost"`
}

// routeCostViews prices every RouteSample through the same RouteCostModel the
// limiter uses, so the panel shows exactly what admission charges. Empty on a
// capability with no route evidence (a pre-v7 row).
func routeCostViews(cap domaintranscoding.EncodeCapability) []routeCostView {
	if len(cap.RouteSamples) == 0 {
		return nil
	}
	m := apptranscoding.NewRouteCostModel(cap.RouteSamples, sysinfo.CPUCapacity())
	out := make([]routeCostView, 0, len(cap.RouteSamples))
	for _, rs := range cap.RouteSamples {
		v := routeCostView{
			Backend: rs.Key.Backend, Encoder: rs.Key.Encoder, Route: string(rs.Key.Route),
			Height: rs.Height, FPS: rs.FPS, CPUCores: rs.CPUCores, SourceFPS: rs.SourceFPS,
		}
		if c, ok := m.Cost(rs.Key, rs.Height, rs.SourceFPS); ok {
			v.AccelCost, v.CPUCost = c.Accel, c.CPU
		}
		out = append(out, v)
	}
	return out
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
		// Embed the capability so its fields stay top-level (the frontend reads
		// them flat) and add the recent HW-fallback events alongside.
		cap := svc.Current()
		resp := struct {
			publicEncodeCapability
			HWMode      string                           `json:"hw_mode"`
			IsWSL       bool                             `json:"is_wsl"`
			HWFallbacks []apptranscoding.HWFallbackEvent `json:"hw_fallbacks"`
			// RouteCosts is what the ADMISSION cost model actually prices on: one
			// row per measured route (backend, encoder, route class, rung) with
			// its warm single-session fps + CPU cores and the derived accel/CPU
			// pool cost. The bench_samples table above stays a DIAGNOSTIC/fallback
			// (the cumulative bare-preset HW rung); these RouteCosts are the truth
			// the engine uses, so "hw 2160 = 55 fps" in that table is not mistaken
			// for the whole story.
			RouteCosts []routeCostView `json:"route_costs"`
			// Calibration is the lifecycle: a boot never benchmarks, so the
			// ceilings above may be interim until the first idle window.
			Calibration apptranscoding.CalibrationStatus `json:"calibration"`
		}{
			publicEncodeCapability: publicEncodeCapability{EncodeCapability: cap.EncodeCapability},
			HWMode:                 string(svc.HWMode()),
			Calibration:            svc.Status(),
			// WSL's d3d12 VA driver can't sustain Evey's multi-variant VAAPI/QSV
			// encode, so the admin UI greys those backends when this is true.
			IsWSL:       sysinfo.IsWSL(),
			HWFallbacks: svc.RecentFallbacks(),
			RouteCosts:  routeCostViews(cap.EncodeCapability),
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "encode capability")
			slog.WarnContext(r.Context(), "calibration: GET encode failed", slog.Any("err", err))
		}
	}
}

// transcodeOverridesGetHandler returns the active CMCD-feedback
// adaptive overrides (Phase C). Read-only - surfaces what the worker
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
			slog.WarnContext(r.Context(), "calibration: GET overrides encode failed", slog.Any("err", err))
		}
	}
}

// transcodeOverrideResetHandler removes a CMCD-feedback adaptive cap
// override for a source-class bucket. Admin can use it when they're
// confident the underlying limit has changed (HW added, encoder
// preset tuned) and they want to give the calibrated ceiling a fresh
// chance - instead of waiting for the 7-day TTL.
//
// Routes via `?source_class=2160p` (or `1440p`/`1080p`). Returns 204
// on success (idempotent - removing a non-existent override is
// fine), 400 on missing query.
func transcodeOverrideResetHandler(svc capOverrideResetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.transcode.calibration.OverrideReset")
		defer span.End()

		if svc == nil {
			http.Error(w, `{"message":"override service not wired"}`, http.StatusServiceUnavailable)
			return
		}
		sourceClass := r.URL.Query().Get("source_class")
		if sourceClass == "" {
			http.Error(w, `{"message":"missing source_class query"}`, http.StatusBadRequest)
			return
		}
		// Detach from the request context: the DELETE is fast (single
		// SQL row) but the cache refresh that follows hits the same
		// repo and we want both to complete even if the admin's
		// browser navigates away.
		runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := svc.ResetOverride(runCtx, sourceClass); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "reset override")
			slog.WarnContext(runCtx, "calibration: override reset failed",
				slog.String("source_class", sourceClass), slog.Any("err", err))
			http.Error(w, `{"message":"reset failed"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// transcodeRecalibrateHandler triggers a fresh benchmark. Synchronous -
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
			if errors.Is(err, apptranscoding.ErrBenchmarkBusyTranscoding) {
				// Calibrating under load measures contended throughput → caps the
				// host too low. Tell the operator to stop playback and retry.
				http.Error(w, `{"message":"transcodes active","code":"transcodes_active"}`, http.StatusConflict)
				return
			}
			span.RecordError(err)
			span.SetStatus(codes.Error, "recalibrate failed")
			slog.WarnContext(runCtx, "calibration: recalibrate failed", slog.Any("err", err))
			http.Error(w, `{"message":"recalibration failed"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(publicEncodeCapability{EncodeCapability: svc.Current().EncodeCapability}); err != nil {
			slog.WarnContext(r.Context(), "calibration: recalibrate response encode failed", slog.Any("err", err))
		}
	}
}
