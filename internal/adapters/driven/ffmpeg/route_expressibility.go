package ffmpeg

import (
	"log/slog"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// Expressibility answers, from the builder's own filter tables, whether a
// route topology can be emitted on an accelerator (ADR-0030). It is the ONE
// home of those rules: the resolver's backend pick asks them per candidate
// before resolving a route, and the builder asks them again at spawn
// (routeRefusal) - to refuse, never to swap one pipeline for another.
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

// refuseRoute is the builder's answer to a GPU plan it cannot express: no
// arguments (the spawn refused the plan before asking; a probe or a test
// that asks anyway gets nothing, never a software encode in GPU clothing),
// the alert counter that must stay at zero, and an error-level log naming
// the plan. The pick filtered on the same port (ADR-0030), so reaching here
// means the plan named a topology or a need the pick did not.
func refuseRoute(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string, err error) []string {
	metrics.HLSVODRouteNotExpressible.Inc()
	slog.Error("hls: refused a GPU plan the builder cannot express - no software fallback (invariant, ADR-0030)",
		slog.String("accel", string(opts.HWAccel)), slog.String("device", opts.HWDevice),
		slog.Bool("burn_in", burnIn), slog.Bool("overlay", opts.HWOverlay), slog.Bool("deinterlace", opts.HWDeinterlace), slog.Bool("software_decode", opts.HWSoftwareDecode),
		slog.Bool("tone_map", toneMap != ""), slog.Bool("hw_tonemap", opts.HWTonemap), slog.Bool("hdr_passthrough", opts.HDRPassthrough),
		slog.Any("features", opts.HWFeatures), slog.Any("err", err))
	return nil
}
