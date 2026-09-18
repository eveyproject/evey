package ffmpeg

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

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

// noteExpressibility records the builder's own verdict on a GPU plan (the
// SHADOW of its refusal): the cell it checked and whether hwUsable found a
// pipeline for it. A refusal means the builder is about to emit a software
// encode under a GPU-billed session - a planner/builder disagreement, counted
// and logged, never silent. Once every reachable cell is covered with zero
// disagreements the fallback becomes a refusal (ADR-0030).
func noteExpressibility(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string, ok bool) {
	if opts.HWAccel == "" || opts.HWAccel == transcoding.None {
		return // a software plan: nothing to check
	}
	cell := builderCell(opts, burnIn, toneMap)
	recordExpressibility(transcoding.ExpressibilitySiteBuilder, cell, !ok)
	if ok {
		return
	}
	metrics.HLSVODRouteNotExpressible.Inc()
	slog.Warn("hls: resolved GPU route not expressible by the builder - software fallback (planner/builder disagreement, ADR-0030)",
		slog.String("accel", string(opts.HWAccel)), slog.String("device", opts.HWDevice), slog.String("topology", cell.Topology), slog.String("needs", cell.Needs),
		slog.Bool("tone_map", toneMap != ""), slog.Bool("hw_tonemap", opts.HWTonemap), slog.Any("features", opts.HWFeatures))
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
