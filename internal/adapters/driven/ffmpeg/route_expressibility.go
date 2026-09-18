package ffmpeg

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

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

// noteExpressibility records one builder-site check the plan passed, on the
// coverage table and the per-cell counter (the shadow's exit device).
func noteExpressibility(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string) {
	recordExpressibility(transcoding.ExpressibilitySiteBuilder, builderCell(opts, burnIn, toneMap), false)
}

// refuseRoute is the builder's answer to a GPU plan it cannot express: no
// arguments (the spawn refused the plan before asking; a probe or a test
// that asks anyway gets nothing, never a software encode in GPU clothing),
// the alert counter that must stay at zero, and an error-level log naming
// the plan. The pick filtered on the same port (ADR-0030), so reaching here
// means the plan named a topology or a need the pick did not.
func refuseRoute(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string, err error) []string {
	cell := builderCell(opts, burnIn, toneMap)
	recordExpressibility(transcoding.ExpressibilitySiteBuilder, cell, true)
	metrics.HLSVODRouteNotExpressible.Inc()
	slog.Error("hls: refused a GPU plan the builder cannot express - no software fallback (invariant, ADR-0030)",
		slog.String("accel", string(opts.HWAccel)), slog.String("device", opts.HWDevice), slog.String("topology", cell.Topology), slog.String("needs", cell.Needs),
		slog.Bool("tone_map", toneMap != ""), slog.Bool("hw_tonemap", opts.HWTonemap), slog.Any("features", opts.HWFeatures), slog.Any("err", err))
	return nil
}

// builderCell reads the cell off the process options exactly as hwUsable
// reads them (the topology by the CostKey precedence, the needs by the
// burn-in / overlay / deinterlace flags).
func builderCell(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string) transcoding.ExpressibilityCell {
	t := transcoding.EncodeRoute{
		HWUsable: true, SoftwareDecode: opts.HWSoftwareDecode,
		Tonemap: toneMap != "", HWTonemap: opts.HWTonemap && toneMap != "", HDRPassthrough: opts.HDRPassthrough,
	}.Topology()
	return transcoding.ExpressibilityCell{
		Backend:  string(opts.HWAccel),
		Topology: t.Name(),
		Needs:    transcoding.RouteNeedsLabel(burnIn, opts.HWOverlay, opts.HWDeinterlace),
	}
}

// recordExpressibility feeds the coverage table and the per-cell counters for
// one check at the builder site (the resolver records its own site in the app).
func recordExpressibility(site transcoding.ExpressibilitySite, cell transcoding.ExpressibilityCell, disagreed bool) {
	transcoding.ExpressibilityCoverageTable.Record(site, cell, disagreed)
	labels := prometheus.Labels{"site": string(site), "backend": cell.Backend, "topology": cell.Topology, "needs": cell.Needs}
	metrics.HLSVODRouteExpressibilityChecks.With(labels).Inc()
	if disagreed {
		metrics.HLSVODRouteExpressibilityDisagreements.With(labels).Inc()
	}
}
