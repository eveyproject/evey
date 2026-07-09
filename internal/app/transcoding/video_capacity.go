package transcoding

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// MaxVideoSessionsConfigKey is the server_config key an admin uses to REDUCE
// (never raise) the auto-derived video-session capacity (admin panel only -
// never an env var). Empty/absent/"auto" means
// "use the auto capacity"; a positive integer is clamped to at most the auto
// capacity by ResolveVideoSessionLimit.
const MaxVideoSessionsConfigKey = "transcoding.max_video_sessions"

// VideoCapacityEvidence is the human-readable "why" behind
// AutoVideoSessionCapacity's answer - the admin panel surfaces it verbatim so
// an operator can see the bench rung the number was derived from instead of
// trusting a bare integer.
type VideoCapacityEvidence struct {
	// Height is the resolution rung the capacity was derived from (0 when
	// Derived is false).
	Height int
	// FPS is that rung's measured encode throughput.
	FPS float64
	// RequiredFPS is the realtime threshold the rung had to clear.
	RequiredFPS float64
	// Ratio is FPS / RequiredFPS - "how many realtimes" one encoder of this
	// rung sustains, the basis for the session count.
	Ratio float64
	// CPUBudget is the host's usable core count (sysinfo.CPUBudget), the
	// hard ceiling capacity is clamped to.
	CPUBudget int
	// Derived is false when no usable calibration evidence existed and
	// capacity fell back to the conservative default of 1.
	Derived bool
}

// AutoVideoSessionCapacity derives the number of concurrent VIDEO transcode
// sessions (i.e. sessions that actually ENCODE video - never a video-copy/
// remux rung) a host can sustain, from the boot benchmark's software rungs.
//
// It picks the tallest PASSING "software" ladder rung, takes the ratio of
// its measured fps over the realtime fps it had to clear, and floors that
// ratio to an integer session count: the same reasoning an operator would
// use by hand ("this box does 2.4x realtime at the top resolution it
// passed, so about 2 full-speed encodes fit before either drops below
// realtime"). The result is bounded to [1, cpuBudget] - never zero (a
// hard-zero cap would wedge every viewer) and never more sessions than the
// host has cores for (a software encode is ~1 full core).
//
// The TALLEST passing rung is the reference on purpose: it is the most
// expensive thing this host is willing to serve, so sizing on it never
// promises more sessions than the worst realistic case. A box that sustains
// 1.9x realtime at 1440p reports capacity 1, even though it would fit two
// 1080p encodes - a deliberate under-promise. Sizing on a mid rung instead
// would over-promise the moment one viewer asks for the top rung, which is
// exactly the situation a weak NAS cannot absorb. Operator decision, 2026-07.
//
// Fail-safe: no samples, no passing "software" rung, or a non-positive
// RequiredFPS all fall back to capacity=1 with Derived=false - the most
// conservative answer available when the evidence doesn't support anything
// better. Hardware ("hw") and HDR ("software-hdr") rungs are ignored - this
// is a bound on SOFTWARE video-encode concurrency only.
func AutoVideoSessionCapacity(samples []transcoding.BenchRungSample, cpuBudget int) (int, VideoCapacityEvidence) {
	cpuBudget = max(cpuBudget, 1)

	var best *transcoding.BenchRungSample
	for i := range samples {
		s := &samples[i]
		if s.Ladder != "software" || !s.Passing {
			continue
		}
		if best == nil || s.Height > best.Height {
			best = s
		}
	}

	if best == nil || best.RequiredFPS <= 0 {
		return 1, VideoCapacityEvidence{CPUBudget: cpuBudget, Derived: false}
	}

	ratio := best.FPS / best.RequiredFPS
	capacity := max(int(math.Floor(ratio)), 1)
	capacity = min(capacity, cpuBudget)

	return capacity, VideoCapacityEvidence{
		Height:      best.Height,
		FPS:         best.FPS,
		RequiredFPS: best.RequiredFPS,
		Ratio:       ratio,
		CPUBudget:   cpuBudget,
		Derived:     true,
	}
}

// ResolveVideoSessionLimit reads MaxVideoSessionsConfigKey from server_config
// and combines it with the auto-derived capacity: the admin can only REDUCE
// the ceiling, never exceed it. An empty value, an absent row, a nil repo, the
// literal "auto", or a value that fails to parse as a positive integer all
// resolve to auto unchanged (a parse failure logs a Warn so a typo doesn't
// silently do nothing). A parsed value is clamped to [1, auto] - min(n, auto).
func ResolveVideoSessionLimit(ctx context.Context, cfgRepo shared.ServerConfigRepository, auto int) int {
	if auto < 1 {
		auto = 1
	}
	if cfgRepo == nil {
		return auto
	}
	entry, err := cfgRepo.Get(ctx, MaxVideoSessionsConfigKey)
	if err != nil || entry == nil {
		return auto
	}
	val := strings.TrimSpace(entry.Value)
	if val == "" || strings.EqualFold(val, "auto") {
		return auto
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		slog.Warn("transcoding: invalid transcoding.max_video_sessions, falling back to auto",
			slog.String("value", entry.Value))
		return auto
	}
	if n > auto {
		return auto
	}
	return n
}

// RungFit is one resolution rung's admission cost, exposed for the admin
// capacity panel: the fraction of the host's realtime software-encode budget
// a stream topping out at Height consumes, and how many such streams the
// budget fits.
type RungFit struct {
	Height     int
	Cost       float64
	MaxStreams int
}

// costRung is a single usable software bench rung, sorted ascending by
// Height inside VideoCostModel.rungs.
type costRung struct {
	Height int
	Cost   float64
}

// VideoCostModel translates the resolution a session tops out at into the
// fraction of the host's realtime software-encode capacity it costs, derived
// from the boot benchmark's "software" rungs. A rung's cost is
// RequiredFPS/FPS - the same ratio AutoVideoSessionCapacity uses, but kept
// PER RUNG instead of collapsed to the single tallest passing one, so two
// sessions at cheaper rungs (e.g. 1440p + 1080p, cost 0.530 + 0.431 = 0.961)
// can share the machine instead of each being billed as "a full stream" the
// old single-counter model charged. See VideoSessionLimiter, which spends
// this cost against a budget (default 1.0 = one whole host) instead of just
// counting streams.
type VideoCostModel struct {
	// rungs holds only "software"-ladder, Passing rungs with FPS > 0, sorted
	// ascending by Height. Never empty on a non-nil *VideoCostModel -
	// NewVideoCostModel returns nil instead of an empty model.
	rungs []costRung
}

// NewVideoCostModel builds a VideoCostModel from the boot benchmark's rungs,
// keeping only Ladder=="software" AND Passing==true samples with FPS > 0 (a
// non-positive FPS is bogus evidence, not a real measurement, and would
// divide-by-zero or invert the cost). Returns nil when no rung qualifies -
// callers must treat a nil model as "no cost evidence", not "zero cost" (see
// CostForHeight's fail-safe).
func NewVideoCostModel(samples []transcoding.BenchRungSample) *VideoCostModel {
	var rungs []costRung
	for _, s := range samples {
		if s.Ladder != "software" || !s.Passing || s.FPS <= 0 {
			continue
		}
		rungs = append(rungs, costRung{Height: s.Height, Cost: s.RequiredFPS / s.FPS})
	}
	if len(rungs) == 0 {
		return nil
	}
	sort.Slice(rungs, func(i, j int) bool { return rungs[i].Height < rungs[j].Height })
	return &VideoCostModel{rungs: rungs}
}

// CostForHeight returns the fraction of the host's realtime software-encode
// capacity a session topping out at height h costs: the cost of the
// SMALLEST calibrated rung whose Height >= h (the rung that actually bounds
// what ffmpeg has to sustain for this output), or the tallest rung's cost
// when h exceeds every calibrated rung (extrapolating past what was benched
// would be a guess - the tallest measured cost is the honest answer).
//
// A nil receiver or an empty model returns 1.0 - the fail-safe "this stream
// fills the whole machine" answer used when there is no evidence to size
// anything smaller, matching VideoCostModel's role as an optional refinement
// layered on top of the always-safe per-stream counter.
func (m *VideoCostModel) CostForHeight(h int) float64 {
	if m == nil || len(m.rungs) == 0 {
		return 1.0
	}
	for _, r := range m.rungs {
		if r.Height >= h {
			return r.Cost
		}
	}
	return m.rungs[len(m.rungs)-1].Cost
}

// MaxStreamsAt returns floor(1/CostForHeight(h)), never less than 1 - how
// many concurrent streams topping out at height h fit inside one whole host
// budget. Purely informational (the admin panel's "N streams at this
// resolution" estimate); the limiter itself sums fractional cost rather than
// pre-computing this per admission.
func (m *VideoCostModel) MaxStreamsAt(h int) int {
	cost := m.CostForHeight(h)
	if cost <= 0 {
		return 1
	}
	return max(int(math.Floor(1.0/cost)), 1)
}

// Rungs returns every calibrated rung's cost + derived MaxStreamsAt, sorted
// ascending by Height, for the admin capacity panel. Empty (never nil) on a
// nil receiver.
func (m *VideoCostModel) Rungs() []RungFit {
	if m == nil {
		return []RungFit{}
	}
	out := make([]RungFit, 0, len(m.rungs))
	for _, r := range m.rungs {
		out = append(out, RungFit{Height: r.Height, Cost: r.Cost, MaxStreams: m.MaxStreamsAt(r.Height)})
	}
	return out
}

// topProfileHeight returns the tallest Height among the profiles a
// video-encoding process is about to be started with - single-variant
// producers pass a one-profile slice (that profile's own height), the
// multi-variant orchestrator path passes the whole ladder it hands to one
// ffmpeg (the top rung it climbs to, the actual cost driver: a
// multi-variant process pays for its tallest output regardless of how many
// shorter renditions ride along). 0 on an empty slice - VideoCostModel's
// CostForHeight then returns the cost of its shortest rung, never a spurious
// refusal.
func topProfileHeight(profiles []transcoding.HLSProfile) int {
	top := 0
	for _, p := range profiles {
		if p.Height > top {
			top = p.Height
		}
	}
	return top
}
