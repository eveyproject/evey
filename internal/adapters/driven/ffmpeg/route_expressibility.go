package ffmpeg

import (
	"log/slog"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// Expressibility answers, from the builder's own filter tables, whether a
// route topology can be emitted on an accelerator (ADR-0030). It is the
// single home of the rules hwUsable applies at build time: the resolver asks
// them here BEFORE resolving a route, so the builder never has to swap one
// pipeline for another behind the plan's back.
type Expressibility struct{}

var _ transcoding.RouteExpressibility = Expressibility{}

// Expressible reports why the topology cannot run on accel, or nil.
func (Expressibility) Expressible(t transcoding.Topology, accel transcoding.HWAccelType, device string, features []string, needs transcoding.RouteNeeds) error {
	if !t.HW() {
		return nil // the software chain expresses every shape it is given
	}
	refuse := func(why string) error { return transcoding.ErrRouteNotExpressible{Topology: t, Accel: accel, Why: why} }
	if accel == "" || accel == transcoding.None || !accel.IsValid() {
		return refuse("no accelerator")
	}
	// A burn-in composites on the GPU only with a probed compositor the
	// backend has wired; otherwise it is a CPU overlay no GPU chain carries
	// (prod 2026-09-09: one forced-subtitle flag sent a 4K HDR session to the CPU).
	if needs.BurnInBitmap && (!needs.GPUOverlay || hwOverlayFilter(accel) == "") {
		return refuse("bitmap burn-in without a GPU compositor")
	}
	if t.SoftwareDecode() {
		// The decoupled route needs an upload chain and a device to upload to.
		if hwUploadFilter(accel) == "" {
			return refuse("no upload chain for a software decode")
		}
		if len(hwDeviceOnlyArgs(accel, device)) == 0 {
			return refuse("no device to upload to")
		}
	}
	if t.ToneMap == transcoding.ToneMapGPU && hwTonemapFilter(accel, features) == "" {
		return refuse("no GPU tone-map filter")
	}
	return nil
}

// noteRouteNotExpressible records the SHADOW of the builder's refusal: the
// plan resolved a GPU route, hwUsable found no pipeline for it and the
// builder is about to emit a software encode under a GPU-billed session. The
// resolver consults Expressibility before resolving, so this is a
// planner/builder disagreement - counted and logged, never silent. Once the
// counter reads zero in production the fallback becomes a refusal (ADR-0030).
func noteRouteNotExpressible(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string) {
	if opts.HWAccel == "" || opts.HWAccel == transcoding.None {
		return // a software plan: nothing to disagree about
	}
	metrics.HLSVODRouteNotExpressible.Inc()
	slog.Warn("hls: resolved GPU route not expressible by the builder - software fallback (planner/builder disagreement, ADR-0030)",
		slog.String("accel", string(opts.HWAccel)), slog.String("device", opts.HWDevice),
		slog.Bool("burn_in", burnIn), slog.Bool("hw_overlay", opts.HWOverlay),
		slog.Bool("tone_map", toneMap != ""), slog.Bool("hw_tonemap", opts.HWTonemap),
		slog.Bool("software_decode", opts.HWSoftwareDecode), slog.Any("features", opts.HWFeatures))
}
