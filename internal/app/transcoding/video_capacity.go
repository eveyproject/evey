package transcoding

import (
	"context"
	"log/slog"
	"math"
	"slices"
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
		slog.WarnContext(ctx, "transcoding: invalid transcoding.max_video_sessions, falling back to auto",
			slog.String("value", entry.Value))
		return auto
	}
	if n > auto {
		return auto
	}
	return n
}

// LadderClass identifies which bench "Ladder" family (see
// transcoding.BenchRungSample.Ladder) a session's video encode belongs to.
// It is the unit both the cost model prices and the session limiter bills:
// each class maps to exactly one ResourcePool (see LadderClass.Pool), and a
// session's cost is looked up in ITS class's own table - a hw session is
// never priced (or billed) against the software table and vice versa.
type LadderClass string

const (
	LadderClassSoftware    LadderClass = "software"
	LadderClassSoftwareHDR LadderClass = "software-hdr"
	LadderClassSoftwareAV1 LadderClass = "software-av1"
	LadderClassHW          LadderClass = "hw"
)

// ResourcePool is the physical resource a LadderClass's encode consumes.
// Software ladders (plain, HDR-tonemap, AV1) all burn a CPU core running
// libx264/libsvtav1 and share PoolCPU; the "hw" ladder runs on the
// accelerator (NVENC/QSV/VAAPI) instead and gets its own PoolGPU - the two
// are independent budgets because a GPU encode costs almost no CPU, and a
// box with a GPU sitting idle while software streams queue up is exactly
// the under-utilisation this pooling fixes.
type ResourcePool string

const (
	PoolCPU ResourcePool = "cpu"
	PoolGPU ResourcePool = "gpu"
)

// Pool returns the resource pool c's encodes are billed against.
func (c LadderClass) Pool() ResourcePool {
	if c == LadderClassHW {
		return PoolGPU
	}
	return PoolCPU
}

// LadderClassOf derives the ladder class a DecisionFingerprint's encode will
// run as - a pure function of the fingerprint, no I/O or lookup. HWAccel set
// wins unconditionally: a HW-routed decision never runs a software encoder
// for its video, whatever VideoCodec/HDRTonemap say. Otherwise the decision
// follows the same precedence the boot benchmark uses to pick a ladder: AV1
// output first (it gets its own, far heavier, software ceiling), then HDR
// tonemap, then plain software.
func LadderClassOf(fp DecisionFingerprint) LadderClass {
	if fp.HWAccel != "" {
		return LadderClassHW
	}
	if strings.EqualFold(strings.TrimSpace(fp.VideoCodec), "av1") {
		return LadderClassSoftwareAV1
	}
	if fp.HDRTonemap {
		return LadderClassSoftwareHDR
	}
	return LadderClassSoftware
}

// SourceClassOf names the DECODE workload a fingerprint's session will run.
// Zero when either half is unknown, which prices the output-rung way.
func SourceClassOf(fp DecisionFingerprint) transcoding.SourceClass {
	return transcoding.SourceClass{Codec: fp.SourceVideoCodec, Height: fp.SourceVideoHeight}
}

// RungFit is one resolution rung's admission cost, exposed for the admin
// capacity panel: the fraction of the host's realtime encode budget (within
// its ladder class's resource pool) a stream topping out at Height consumes,
// and how many such streams the budget fits.
type RungFit struct {
	Height     int
	Cost       float64
	MaxStreams int
	// BySource prices the SAME rung for each source class the host measured.
	// Cost/MaxStreams above hold only for the source the bench decoded, so a
	// panel showing them alone answers "how many streams" without saying of
	// what. Empty when the host has no decode evidence.
	BySource []SourceFit
}

// SourceFit is one rung's cost once the thing being decoded is named.
type SourceFit struct {
	Source     transcoding.SourceClass
	Cost       float64
	MaxStreams int
}

// representativeSourceHeights are the source heights the per-source breakdown
// is reported at: the two a real library is actually made of. The decode term
// scales by area within a codec, so these interpolate from the measurement
// rather than adding assumptions.
var representativeSourceHeights = []int{1080, 2160}

// MeasuredSources lists the source classes this model can price, at the
// representative heights. Empty without decode evidence.
func (m *VideoCostModel) MeasuredSources() []transcoding.SourceClass {
	if m == nil || len(m.decode) == 0 {
		return nil
	}
	codecs := make([]string, 0, len(m.decode))
	for codec := range m.decode {
		codecs = append(codecs, codec)
	}
	slices.Sort(codecs)
	out := make([]transcoding.SourceClass, 0, len(codecs)*len(representativeSourceHeights))
	for _, codec := range codecs {
		for _, h := range representativeSourceHeights {
			out = append(out, transcoding.SourceClass{Codec: codec, Height: h})
		}
	}
	return out
}

// costRung is a single usable bench rung, sorted ascending by Height inside
// VideoCostModel.tables[class]. FPS/RequiredFPS are kept because decode-aware
// pricing takes the rung apart again to isolate its encode half.
type costRung struct {
	Height      int
	Cost        float64
	FPS         float64
	RequiredFPS float64
}

// VideoCostModel translates the resolution a session tops out at into the
// fraction of its ladder class's resource-pool budget it costs, derived from
// the boot benchmark's rungs. A rung's cost is RequiredFPS/FPS - the same
// ratio AutoVideoSessionCapacity uses, but kept PER RUNG instead of
// collapsed to the single tallest passing one, so two sessions at cheaper
// rungs (e.g. 1440p + 1080p, cost 0.530 + 0.431 = 0.961) can share the
// machine instead of each being billed as "a full stream" the old
// single-counter model charged.
//
// Unlike the software ladder, a HW rung's cost is NOT monotonic in height -
// a measured box can show 720p hw more expensive than 1080p hw (VPP/decode
// overhead does not scale the way software encode time does) - so nothing
// here assumes taller = pricier within a class; CheapestRung exists
// specifically because "shortest rung" would be the wrong answer for hw.
//
// See VideoSessionLimiter, which spends this cost against a PER-POOL budget
// (default 1.0 = one whole pool) instead of just counting streams.
type VideoCostModel struct {
	// tables holds, per ladder class, only that class's Passing rungs with
	// FPS > 0, sorted ascending by Height. A class absent from tables has no
	// usable evidence at all (never an empty slice - see NewVideoCostModel).
	tables map[LadderClass][]costRung

	// benchSource + decode are OPTIONAL decode evidence (see WithDecodeEvidence).
	// Zero on a model built from encode rungs alone, where CostForSource prices
	// exactly as CostFor always did.
	benchSource transcoding.SourceClass
	decode      map[string]transcoding.DecodeRungSample
}

// NewVideoCostModel builds a VideoCostModel from the boot benchmark's rungs,
// keeping every Passing==true sample with FPS > 0 (a non-positive FPS is
// bogus evidence, not a real measurement, and would divide-by-zero or invert
// the cost), grouped into one table per LadderClass(sample.Ladder). Returns
// nil when NO sample of ANY class qualifies - callers must treat a nil model
// as "no cost evidence anywhere", not "zero cost" (see CostFor's fail-safe).
//
// A server with no hw bench samples simply never gets a LadderClassHW table
// - CostFor(LadderClassHW, h) then falls back to the software table exactly
// as it would have priced a "hw" session before this type existed, so a
// software-only box's behaviour is byte-identical to today.
func NewVideoCostModel(samples []transcoding.BenchRungSample) *VideoCostModel {
	tables := make(map[LadderClass][]costRung)
	for _, s := range samples {
		if !s.Passing || s.FPS <= 0 {
			continue
		}
		class := LadderClass(s.Ladder)
		tables[class] = append(tables[class], costRung{
			Height:      s.Height,
			Cost:        s.RequiredFPS / s.FPS,
			FPS:         s.FPS,
			RequiredFPS: s.RequiredFPS,
		})
	}
	if len(tables) == 0 {
		return nil
	}
	for class, rungs := range tables {
		sort.Slice(rungs, func(i, j int) bool { return rungs[i].Height < rungs[j].Height })
		tables[class] = rungs
	}
	return &VideoCostModel{tables: tables}
}

// CostFor returns the fraction of class's resource-pool budget a session in
// that class, topping out at height h, costs: the cost of the SMALLEST
// calibrated rung in class's table whose Height >= h (the rung that
// actually bounds what ffmpeg has to sustain for this output), or the
// tallest rung's cost when h exceeds every calibrated rung in the table
// (extrapolating past what was benched would be a guess - the tallest
// measured cost is the honest answer).
//
// class absent from the model (no usable bench evidence for it, e.g. a
// software-only box asked to price LadderClassHW) falls back to the
// software table; if software itself has no table either, returns 1.0 - the
// fail-safe "this stream fills the whole pool" answer used when there is no
// evidence to size anything smaller. A nil receiver also returns 1.0.
func (m *VideoCostModel) CostFor(class LadderClass, h int) float64 {
	if m == nil {
		return 1.0
	}
	rungs, ok := m.tables[class]
	if !ok || len(rungs) == 0 {
		if class != LadderClassSoftware {
			return m.CostFor(LadderClassSoftware, h)
		}
		return 1.0
	}
	for _, r := range rungs {
		if r.Height >= h {
			return r.Cost
		}
	}
	return rungs[len(rungs)-1].Cost
}

// WithDecodeEvidence attaches the bench's DECODE-only measurements so costs can
// be priced against the source the viewer asked for. Unusable rows are dropped:
// decode evidence only ADDS discrimination, so a host without it is unchanged.
func (m *VideoCostModel) WithDecodeEvidence(benchSource transcoding.SourceClass, decode []transcoding.DecodeRungSample) *VideoCostModel {
	if m == nil {
		return nil
	}
	m.benchSource = benchSource
	m.decode = nil
	for _, d := range decode {
		if d.FPS <= 0 || d.Height <= 0 || d.Codec == "" {
			continue
		}
		if m.decode == nil {
			m.decode = make(map[string]transcoding.DecodeRungSample, len(decode))
		}
		m.decode[canonicalDecodeCodec(d.Codec)] = d
	}
	return m
}

// canonicalDecodeCodec folds a codec alias onto the name the decode table is
// keyed by, so the runtime's spelling finds the bench's measurement.
func canonicalDecodeCodec(codec string) string {
	if c := transcoding.CanonicalVideoCodec(codec); c != "" {
		return c
	}
	return strings.ToLower(strings.TrimSpace(codec))
}

// decodeFPSFor returns measured decode throughput for src, scaled by frame AREA
// from the height it was measured at. That scaling is the ONE modelled step
// here, and it only interpolates WITHIN a codec - never across codecs, where the
// real 2-4x gaps live and a model would be guessing.
func (m *VideoCostModel) decodeFPSFor(src transcoding.SourceClass) (float64, bool) {
	if m == nil || len(m.decode) == 0 || !src.Known() {
		return 0, false
	}
	sample, ok := m.decode[canonicalDecodeCodec(src.Codec)]
	if !ok || sample.FPS <= 0 || sample.Height <= 0 {
		return 0, false
	}
	measured := transcoding.SourceClass{Codec: sample.Codec, Height: sample.Height}
	if measured.Pixels() <= 0 || src.Pixels() <= 0 {
		return 0, false
	}
	return sample.FPS * float64(measured.Pixels()) / float64(src.Pixels()), true
}

// CostForSource prices a session for the SOURCE the viewer asked for, not the
// one the bench decoded:
//
//	cost = RequiredFPS × ( (1/rungFPS - 1/decodeFPS(benchSource)) + 1/decodeFPS(src) )
//
// Priced on the rung alone, a 1080p H.264 and a 4K HEVC source feeding the same
// 480p rung cost the same, though they measured 8.43x and 2.12x realtime. Note
// src == benchSource cancels both terms, reproducing CostFor exactly.
//
// Falls back to CostFor whenever evidence is missing, or when the isolated
// encode term would be non-positive (a rung measured slower than the decode
// probe itself - real on a loaded box, not something to extrapolate from).
func (m *VideoCostModel) CostForSource(class LadderClass, h int, src transcoding.SourceClass) float64 {
	base := m.CostFor(class, h)
	rung, ok := m.rungFor(class, h)
	if !ok || rung.RequiredFPS <= 0 || rung.FPS <= 0 {
		return base
	}
	benchDecodeFPS, ok := m.decodeFPSFor(m.benchSource)
	if !ok {
		return base
	}
	srcDecodeFPS, ok := m.decodeFPSFor(src)
	if !ok {
		return base
	}
	encodeTime := 1/rung.FPS - 1/benchDecodeFPS
	if encodeTime <= 0 {
		return base
	}
	return rung.RequiredFPS * (encodeTime + 1/srcDecodeFPS)
}

// rungFor returns the rung CostFor would have priced h against, so both paths
// take apart the same measurement.
func (m *VideoCostModel) rungFor(class LadderClass, h int) (costRung, bool) {
	if m == nil {
		return costRung{}, false
	}
	rungs, exists := m.tables[class]
	if !exists || len(rungs) == 0 {
		if class != LadderClassSoftware {
			return m.rungFor(LadderClassSoftware, h)
		}
		return costRung{}, false
	}
	for _, r := range rungs {
		if r.Height >= h {
			return r, true
		}
	}
	return rungs[len(rungs)-1], true
}

// CostForHeight is CostFor(LadderClassSoftware, h) - kept for callers that
// only ever dealt with the software ladder before ladder classes existed.
func (m *VideoCostModel) CostForHeight(h int) float64 {
	return m.CostFor(LadderClassSoftware, h)
}

// CheapestRung returns the rung of class with the LOWEST cost - not the
// shortest rung, deliberately: hw costs are not monotonic in height (see the
// VideoCostModel doc), so "steer to the smallest resolution" can recommend a
// MORE expensive rung than a taller one. This is the answer a demotion path
// should use to pick where to land a session that no longer fits at its
// requested height. ok=false when class has no usable table in this model -
// no fallback to software here (a caller demoting within a specific class,
// e.g. hw, wants that class's own cheapest rung or nothing).
func (m *VideoCostModel) CheapestRung(class LadderClass) (height int, cost float64, ok bool) {
	if m == nil {
		return 0, 0, false
	}
	rungs, exists := m.tables[class]
	if !exists || len(rungs) == 0 {
		return 0, 0, false
	}
	best := rungs[0]
	for _, r := range rungs[1:] {
		if r.Cost < best.Cost {
			best = r
		}
	}
	return best.Height, best.Cost, true
}

// maxStreamsForCost returns floor(1/cost), never less than 1 - how many
// concurrent streams at that cost fit inside one whole pool budget.
func maxStreamsForCost(cost float64) int {
	if cost <= 0 {
		return 1
	}
	return max(int(math.Floor(1.0/cost)), 1)
}

// MaxStreamsAt returns floor(1/CostForHeight(h)), never less than 1 - how
// many concurrent software-ladder streams topping out at height h fit inside
// one whole host budget. Purely informational (the admin panel's "N streams
// at this resolution" estimate); the limiter itself sums fractional cost
// rather than pre-computing this per admission.
func (m *VideoCostModel) MaxStreamsAt(h int) int {
	return maxStreamsForCost(m.CostForHeight(h))
}

// RungsFor returns every calibrated rung's cost + derived MaxStreamsAt for
// class, sorted ascending by Height, for the admin capacity panel. Empty
// (never nil) on a nil receiver or a class with no table.
func (m *VideoCostModel) RungsFor(class LadderClass) []RungFit {
	if m == nil {
		return []RungFit{}
	}
	rungs, ok := m.tables[class]
	if !ok {
		return []RungFit{}
	}
	sources := m.MeasuredSources()
	out := make([]RungFit, 0, len(rungs))
	for _, r := range rungs {
		fit := RungFit{Height: r.Height, Cost: r.Cost, MaxStreams: maxStreamsForCost(r.Cost)}
		for _, src := range sources {
			c := m.CostForSource(class, r.Height, src)
			fit.BySource = append(fit.BySource, SourceFit{
				Source: src, Cost: c, MaxStreams: maxStreamsForCost(c),
			})
		}
		out = append(out, fit)
	}
	return out
}

// Rungs is RungsFor(LadderClassSoftware) - kept for callers (and the admin
// panel) that only ever dealt with the software ladder before ladder classes
// existed.
func (m *VideoCostModel) Rungs() []RungFit {
	return m.RungsFor(LadderClassSoftware)
}

// Classes returns every ladder class with a usable cost table, for the admin
// panel to discover which resource pools have evidence at all. A box with no
// hw bench samples reports only LadderClassSoftware (plus software-hdr/av1 if
// benched) - never LadderClassHW.
func (m *VideoCostModel) Classes() []LadderClass {
	if m == nil {
		return nil
	}
	out := make([]LadderClass, 0, len(m.tables))
	for class := range m.tables {
		out = append(out, class)
	}
	slices.Sort(out)
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
