package transcoding

import (
	"math"
	"slices"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// route_cost_model.go - RouteCostModel prices a session on the per-route,
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
type RouteCostModel struct {
	cpuBudget int
	// byKey[key] holds that route's rungs, ascending by height.
	byKey map[transcoding.RouteCostKey][]transcoding.RouteSample
}

// NewRouteCostModel groups the samples by key. Returns nil when there are none
// (a pre-evidence capability, a software-only host that measured no route),
// so callers treat nil as "no route evidence, price the old way".
func NewRouteCostModel(samples []transcoding.RouteSample, cpuBudget int) *RouteCostModel {
	if len(samples) == 0 {
		return nil
	}
	if cpuBudget < 1 {
		cpuBudget = 1
	}
	m := &RouteCostModel{cpuBudget: cpuBudget, byKey: map[transcoding.RouteCostKey][]transcoding.RouteSample{}}
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
// rung >= h, or the tallest measured when h exceeds every rung (extrapolating
// past what was measured would be a guess; the tallest measured cost is the
// honest bound). ok=false when the key was never measured.
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
	return rs[len(rs)-1], true
}

// Cost prices a session of the route named by key, topping out at height h, on
// a source of sourceFPS. Both pools:
//
//	required = sourceFPS x headroom
//	accel    = required / measuredFPS   (0 for a CPU encoder)
//	cpu      = measuredCPUcores / cpuBudget
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
	cost := RouteCost{CPU: r.CPUCores / float64(m.cpuBudget), FromRoute: true}
	if key.Backend != "" {
		cost.Accel = required / r.FPS
	} else {
		// A CPU encoder spends no accelerator; its throughput is bounded by the
		// cores it burns, already on the CPU pool. Price the CPU pool by the
		// realtime ratio (like VideoCostModel), not by the raw core count, so a
		// box's software concurrency is still sized by throughput.
		cost.CPU = required / r.FPS
	}
	return cost, true
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

// MaxStreams is floor(1/max(accel,cpu)), never < 1: how many sessions of the
// route named by key (topping out at h, source sourceFPS) fit before the
// tighter of the two pools it draws on is full. For the admin capacity panel;
// admission itself sums fractional cost per pool rather than this per-route.
func (m *RouteCostModel) MaxStreams(key transcoding.RouteCostKey, h int, sourceFPS float64) (int, bool) {
	c, ok := m.Cost(key, h, sourceFPS)
	if !ok {
		return 0, false
	}
	worst := math.Max(c.Accel, c.CPU)
	if worst <= 0 {
		return 1, true
	}
	return max(int(math.Floor(1.0/worst)), 1), true
}
