package transcoding

import (
	"math"
	"slices"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// route_cost_model.go - RouteCostModel (ADR-0029) prices a session on the per-route,
// single-session, warm evidence the calibration measured (transcoding.RouteSample),
// instead of the cumulative bare-preset HW rung VideoCostModel reads. It is a
// SEPARATE model, consulted first by the limiter for a route it has evidence
// for; VideoCostModel (untouched, the software ladder table) stays the fallback.
//
// It answers two costs, because a route spends two pools: a GPU route costs
// its accelerator budget AND a slice of the CPU pool (demux/mux/AAC/driver, or
// the whole software decode of a decoupled/bridged session), which the old
// single "hw" cost pretended was free.

// RouteCost is what one session of a route costs, per pool.
type RouteCost struct {
	// Accel is the fraction of the accelerator pool the session takes (0 for a
	// pure-CPU route). CPU is the fraction of the CPU pool it takes (never 0:
	// even a zero-copy GPU encode burns a fraction of a core).
	Accel float64
	CPU   float64
	// FromRoute is true when this came from measured route evidence, false
	// when the model had none for the key and the caller should fall back.
	FromRoute bool
}

// RouteCostModel is built from a capability's RouteSamples. Nil (no evidence)
// prices nothing - the limiter then uses VideoCostModel as before.
//
// WHAT THE PRICE IS. Both pools are priced as the session's STEADY-STATE,
// paced consumption - a budget of averages, not of instantaneous peaks:
//
//   - a software session is input-paced at SoftwareReadRate x realtime for
//     its whole life (`-readrate`), so its cores at that rate are its cost;
//   - a hardware session is NOT paced by ffmpeg: it bursts at full speed
//     until it is ThrottlePauseAhead segments ahead of the viewer, is then
//     SIGSTOPped, and thereafter runs at the viewer's pace. The burst is
//     bounded (ThrottlePauseAhead x SegmentSeconds of content, ~30 s, so a
//     few wall-clock seconds on a fast route) and happens once per start or
//     seek; in steady state the session costs what the paced price says.
//
// So the CPU pool is a capacity guarantee for the steady state and NOT a
// guard against several sessions bursting at the same instant: N viewers
// pressing play together can saturate the CPU for a few seconds (each burst
// then runs slower and the throttle parks it sooner). That transient is
// accepted by design - pacing the HW path would trade it for a slower start
// and seek on every session - and bounded by the global session caps
// (MaxActiveSessions, MaxRafalesPerHash); it is not what this model prices.
type RouteCostModel struct {
	// cpuCapacity is the CPU time this process may consume, in cores, exactly
	// (sysinfo.CPUCapacity - the cgroup quota, not the rounded-up worker
	// count: a 0.75-core route is half of a 1.5-CPU container).
	cpuCapacity float64
	// byKey[key] holds that route's rungs, ascending by height.
	byKey map[transcoding.RouteCostKey][]transcoding.RouteSample
}

// NewRouteCostModel groups the samples by key. Returns nil when there are none
// (a pre-evidence capability, a software-only host that measured no route),
// so callers treat nil as "no route evidence, price the old way".
func NewRouteCostModel(samples []transcoding.RouteSample, cpuCapacity float64) *RouteCostModel {
	if len(samples) == 0 {
		return nil
	}
	if cpuCapacity <= 0 {
		cpuCapacity = 1
	}
	m := &RouteCostModel{cpuCapacity: cpuCapacity, byKey: map[transcoding.RouteCostKey][]transcoding.RouteSample{}}
	for _, s := range samples {
		if s.FPS <= 0 {
			continue
		}
		m.byKey[s.Key] = append(m.byKey[s.Key], s)
	}
	if len(m.byKey) == 0 {
		return nil
	}
	for k, rs := range m.byKey {
		slices.SortFunc(rs, func(a, b transcoding.RouteSample) int { return a.Height - b.Height })
		m.byKey[k] = rs
	}
	return m
}

// rungFor returns the measured rung that bounds height h for key: the smallest
// rung >= h (a 1080 sample bounds a 720 request - conservative downwards).
// ok=false when the key was never measured OR h exceeds the tallest rung that
// actually produced: a 2160 point the calibration tried and that failed is
// absent here, and pricing that request as the 1080 rung would admit a
// session the host never proved. Fail-closed; admission then steps down a
// rung (the shaper walks the ladder) or a backend.
func (m *RouteCostModel) rungFor(key transcoding.RouteCostKey, h int) (transcoding.RouteSample, bool) {
	if m == nil {
		return transcoding.RouteSample{}, false
	}
	rs, ok := m.byKey[key]
	if !ok || len(rs) == 0 {
		return transcoding.RouteSample{}, false
	}
	for _, r := range rs {
		if r.Height >= h {
			return r, true
		}
	}
	return transcoding.RouteSample{}, false
}

// TallestRung is the tallest rung the route named by key actually produced -
// the highest quality the calibration PROVED for it, whatever the backend's
// generic encode ceiling says. ok=false when the key was never measured.
func (m *RouteCostModel) TallestRung(key transcoding.RouteCostKey) (int, bool) {
	if m == nil {
		return 0, false
	}
	rs, ok := m.byKey[key]
	if !ok || len(rs) == 0 {
		return 0, false
	}
	return rs[len(rs)-1].Height, true
}

// Cost prices a session of the route named by key, topping out at height h, on
// a source of sourceFPS. Both pools, at the session's steady-state pace (see
// RouteCostModel for what that means and does not mean):
//
//	required = sourceFPS x headroom
//	pace     = required                        (a hardware route: viewer-paced)
//	         = max(required, sourceFPS x SoftwareReadRate)  (a software route)
//	accel    = required / measuredFPS          (0 for a CPU encoder)
//	cpu      = measuredCPUcores x pace / measuredFPS / cpuCapacity
//
// The accelerator cost scales with the SOURCE fps (a 60p source needs 60x
// headroom frames a second) against the fps the route was MEASURED to sustain
// - which is why a 4K 60p route the encoder did not speed up costs ~2.5x a 24p
// one, the C3/prod-host-60p finding. A CPU route's accel is 0 and its whole core
// count lands on the CPU pool. ok=false ⇒ no evidence, price the old way.
func (m *RouteCostModel) Cost(key transcoding.RouteCostKey, h int, sourceFPS float64) (RouteCost, bool) {
	r, ok := m.rungFor(key, h)
	if !ok {
		return RouteCost{}, false
	}
	if sourceFPS <= 0 {
		sourceFPS = r.SourceFPS
	}
	if sourceFPS <= 0 {
		sourceFPS = 24
	}
	required := sourceFPS * transcoding.RouteCostHeadroom
	// CPU is normalised to the rate the session really runs at in steady
	// state, never charged at the bench's max throughput: the session measured
	// CPUCores while racing at measuredFPS, but the runtime paces it - the
	// viewer's pace on a hardware route (after its bounded burst), the
	// producer's -readrate on a software one - so it burns CPUCores x
	// pace/measuredFPS cores. Charging the raw cores over-priced every fast
	// route massively (a 200 fps encode "costs" its racing cores, not the
	// fraction it uses at 30 fps). A route SLOWER than its pace prices above
	// its cores on purpose: that is the "cannot sustain it" signal admission
	// refuses on.
	pace := required
	if key.Backend == "" {
		pace = math.Max(required, sourceFPS*transcoding.SoftwareReadRate)
	}
	requiredCPU := r.CPUCores * pace / r.FPS
	cost := RouteCost{CPU: requiredCPU / m.cpuCapacity, FromRoute: true}
	if key.Backend != "" {
		cost.Accel = required / r.FPS
	} else {
		// A CPU encoder spends no accelerator; its cost is entirely the cores it
		// burns at the paced rate (requiredCPU above), on the CPU pool.
		cost.Accel = 0
	}
	// Decode adjustment (v1): NONE. The measured route fps already includes its
	// real decode - a zero-copy GPU route decoded on the GPU, a decoupled/CPU
	// route on the CPU - so there is nothing to add or subtract, and in
	// particular NO software-decode term is ever applied to a hardware route
	// (r.SoftwareDecode gates any future adjustment; a HW route has it false).
	_ = r.SoftwareDecode
	// The software FLOOR the policy deliberately keeps attemptable (720p SDR /
	// 480p HDR, DerivePolicy floors): a CPU route sampled AT OR UNDER that floor
	// and itself under realtime there. The host may still attempt it, with
	// conservative concurrency: its cost is clamped to one whole pool so exactly
	// one such session fits an idle CPU pool and nothing joins it. Unclamped it
	// would price above the pool and be refused forever on a host that has
	// nothing else. ONLY that case, and only at the cadence it was measured for:
	// a taller rung under realtime is never made admissible by it, a request at
	// a higher rate than the sample (a 60p source on a 24p floor, or on a
	// 24p-certified rung) keeps its honest >1 price and is refused, as does
	// every accelerator route.
	if key.Backend == "" && r.Height <= softwareFloorFor(key.Route) &&
		sourceFPS <= floorSourceFPS(r) && r.FPS < floorSourceFPS(r)*transcoding.RouteCostHeadroom {
		cost.CPU = math.Min(cost.CPU, 1)
	}
	return cost, true
}

// softwareFloorFor is the policy floor of a CPU route class: the HDR tone-map
// chain floors at SoftwareHDRFloor, every other software route at SoftwareFloor.
func softwareFloorFor(route transcoding.RouteCostClass) int {
	p := transcoding.DefaultDerivePolicy()
	if route == transcoding.RouteCostSoftwareHDR {
		return p.SoftwareHDRFloor
	}
	return p.SoftwareFloor
}

// floorSourceFPS is the rate a sample was measured against (film cadence when
// the row predates SourceFPS).
func floorSourceFPS(r transcoding.RouteSample) float64 {
	if r.SourceFPS > 0 {
		return r.SourceFPS
	}
	return 24
}

// CheapestPoolCost is the lowest positive per-session cost the evidence shows
// in each pool - the accelerator fraction of the cheapest GPU route, and the
// normalised CPU of the cheapest CPU-charging route - each priced at its own
// measured rung and source fps. It sizes the count belt from ROUTE evidence so
// that belt is never tighter than route-aware admission (the cumulative table
// priced a GPU 4K session ~2x dearer, which would have capped the count below
// what the pools actually admit). 0 for a pool with no measured route.
func (m *RouteCostModel) CheapestPoolCost() (cpu, gpu float64) {
	if m == nil {
		return 0, 0
	}
	for _, rs := range m.byKey {
		for _, r := range rs {
			c, ok := m.Cost(r.Key, r.Height, r.SourceFPS)
			if !ok {
				continue
			}
			if c.Accel > 0 && (gpu == 0 || c.Accel < gpu) {
				gpu = c.Accel
			}
			if c.CPU > 0 && (cpu == 0 || c.CPU < cpu) {
				cpu = c.CPU
			}
		}
	}
	return cpu, gpu
}

// Keys lists the measured route keys, for the admin panel / diagnostics.
func (m *RouteCostModel) Keys() []transcoding.RouteCostKey {
	if m == nil {
		return nil
	}
	out := make([]transcoding.RouteCostKey, 0, len(m.byKey))
	for k := range m.byKey {
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b transcoding.RouteCostKey) int {
		if a.Backend != b.Backend {
			return cmpString(a.Backend, b.Backend)
		}
		if a.Encoder != b.Encoder {
			return cmpString(a.Encoder, b.Encoder)
		}
		return cmpString(string(a.Route), string(b.Route))
	})
	return out
}

func cmpString(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// MaxStreams is floor(1/max(accel,cpu)): how many sessions of the route named
// by key (topping out at h, source sourceFPS) fit before the tighter of the
// two pools it draws on is full. 0 when even the first would not fit an idle
// pool (a route priced above a whole pool - admission refuses it, and a panel
// saying "1" would promise a stream the limiter never grants). For the admin
// capacity panel; admission itself sums fractional cost per pool.
func (m *RouteCostModel) MaxStreams(key transcoding.RouteCostKey, h int, sourceFPS float64) (int, bool) {
	c, ok := m.Cost(key, h, sourceFPS)
	if !ok {
		return 0, false
	}
	worst := math.Max(c.Accel, c.CPU)
	if worst <= 0 {
		return 1, true
	}
	// budgetEpsilon: a route priced exactly one pool (worst = 1 up to float
	// rounding) is the one stream admission does grant.
	return int(math.Floor((1.0 + budgetEpsilon) / worst)), true
}
