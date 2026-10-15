package loadtest

import (
	"fmt"
)

// runner_slo.go holds the measurement + verdict machinery: the per-type SLO
// tracker, the flash-crowd verdict and the per-tier sample state feeding the
// report.

// sloWindowSize is how many of the most recent segment-fetch samples (across
// every active viewer) are considered when evaluating the SLO.
const sloWindowSize = 20

// minSLOSamples is the minimum number of samples in the rolling window
// before an SLO breach is even considered, so a couple of early jittery
// fetches at 1-2 viewers can't trip the whole run.
const minSLOSamples = 5

// flashState buckets outcomes by ramp slot for ModeFlash, which has no
// meaningful tier ordering: every slot launches at t0.
type flashState struct {
	served          map[int]bool
	shed            map[int]bool
	admissionDenied int
	sloBreachSeen   bool
}

// verdict tallies, at the end of the flash observation window, how many of
// the launched slots were served and never shed, and builds the matching
// stop detail.
func (f *flashState) verdict(tier int) (sustained int, detail string) {
	served := 0
	for slot := 1; slot <= tier; slot++ {
		if f.served[slot] && !f.shed[slot] {
			served++
		}
	}
	detail = fmt.Sprintf("flash: %d of %d viewers served", served, tier)
	switch {
	case served == tier:
	case f.admissionDenied > 0:
		detail += " (admission)"
	case f.sloBreachSeen:
		detail += " (slo)"
	}
	return served, detail
}

// segmentBudgetFactor is the share of a segment's own playback duration within
// which it must ARRIVE.
//
// Not 100%. A player that receives a 7s segment in 6.9s is technically "keeping
// up", and a load test that accepts this will happily report hundreds of
// viewers - every one of them a second away from buffering, with a video that
// takes six seconds to start. Real playback needs the download to outrun the
// playhead so the buffer actually FILLS: at half the segment duration, a client
// gains one second of buffer for every second played, which is what lets it
// absorb a hiccup. This is the difference between "the server technically
// answered" and "people can watch".
const segmentBudgetFactor = 0.5

// sloReferenceSegmentMs is the segment length every fetch is scaled to before
// it is judged: Evey's own on-demand segment duration. Segments are cut on
// keyframes and run 4 to 17 s depending on the title, so a budget taken from
// one stream's target duration and applied to all made a run's verdict depend
// on whichever title it drew first (3 s in one run, 8.5 s in the next on the
// same server, 2026-10-06). Scaled to one length, "a segment arrives within
// half its own duration" is the same bar for every title and every run.
const sloReferenceSegmentMs = 6000

// startupBudgetMs is how long a viewer may wait for its FIRST segment before
// the tier counts as broken. Time-to-first-frame is the most visible number in
// the whole product: past a few seconds, a viewer thinks the app is broken and
// presses back. It is judged separately from the streaming budget because a
// server can be steady-state healthy and still take forever to admit newcomers.
const startupBudgetMs = 4000

// startupMinSamples is how many first-segment timings a tier needs before its
// startup latency is judged, so one cold start can't condemn a tier.
const startupMinSamples = 3

// sloTracker evaluates the segment-fetch latency budget PER VIEWER TYPE.
//
// The budget derives from a stream's own #EXT-X-TARGETDURATION, scaled by
// segmentBudgetFactor. But the paths do not share a segment length - mkvgo
// packages on keyframe boundaries (~6-11s), while the ffmpeg ladder emits 3s
// segments - so a single budget locked from whichever stream happened to start
// first would judge one path against the other's yardstick. In a mixed
// scenario, slot 1 is mkvgo, and every transcode viewer would then be measured
// against an ~11s budget that is nearly impossible to breach: the run would
// climb until the library ran dry, and report no stall at all. Each type
// carries its own budget.
//
// An operator-pinned SegmentSLOMs overrides all of them: an explicit budget is
// an explicit budget.
type sloTracker struct {
	pinned int
	byType map[viewerType]*typeSLO
}

type typeSLO struct {
	ms     int
	locked bool
	ring   []int
}

func newSLOTracker(pinnedMs int) *sloTracker {
	return &sloTracker{pinned: pinnedMs, byType: map[viewerType]*typeSLO{}}
}

func (s *sloTracker) forType(vt viewerType) *typeSLO {
	t, ok := s.byType[vt]
	if !ok {
		t = &typeSLO{ms: s.pinned, locked: s.pinned > 0}
		s.byType[vt] = t
	}
	return t
}

// lockTarget sets that viewer type's budget the first time one of its viewers
// starts: the share of the reference segment within which a segment must
// arrive. The stream's own target duration no longer sets it (see
// sloReferenceSegmentMs); each fetch is scaled to the reference instead.
func (s *sloTracker) lockTarget(vt viewerType, _ int) {
	t := s.forType(vt)
	if !t.locked {
		t.ms = int(float64(sloReferenceSegmentMs) * segmentBudgetFactor)
		t.locked = true
	}
}

// scaled is a fetch time as if the segment had lasted the reference length:
// 2 s for a 12 s segment counts as 1 s, the same share of its duration. An
// operator-pinned budget is an absolute one, and a segment of unknown length
// is taken as it is.
func (s *sloTracker) scaled(ms, segMs int) int {
	if s.pinned > 0 || segMs <= 0 {
		return ms
	}
	return ms * sloReferenceSegmentMs / segMs
}

// observe records one more segment-fetch sample for its viewer type and
// reports whether that type's rolling p95 just breached ITS budget. p95 and
// sloMs are only meaningful when breached is true.
func (s *sloTracker) observe(vt viewerType, ms int) (breached bool, p95, sloMs int) {
	t := s.forType(vt)
	t.ring = append(t.ring, ms)
	if len(t.ring) > sloWindowSize {
		t.ring = t.ring[len(t.ring)-sloWindowSize:]
	}
	if t.ms <= 0 || len(t.ring) < minSLOSamples {
		return false, 0, t.ms
	}
	p95 = percentile(t.ring, 95)
	return p95 > t.ms, p95, t.ms
}

// startTier drops every sample from the previous tier, keeping the budgets.
//
// The rolling window must NOT span tiers. When the bisection steps back down
// from an overloaded tier, that tier's slow fetches are still sitting in the
// window; the smaller tier - which is serving perfectly - then inherits them and
// is condemned for a stall it never had. The climb chases that phantom all the
// way to the bottom. A tier is judged on ITS OWN samples, and nothing else.
func (s *sloTracker) startTier() {
	for _, t := range s.byType {
		t.ring = t.ring[:0]
	}
}

// budgetFor is the budget a viewer type is judged against.
func (s *sloTracker) budgetFor(vt viewerType) int { return s.forType(vt).ms }

// budgets reports each type's budget, so the report can say what each path was
// actually judged against instead of a single blended number.
func (s *sloTracker) budgets() map[string]int {
	out := make(map[string]int, len(s.byType))
	for vt, t := range s.byType {
		if t.ms > 0 {
			out[string(vt)] = t.ms
		}
	}
	return out
}

// worstBudget is the single number the legacy SegmentSLOMs field carries: the
// loosest budget in play. Callers that care about correctness read budgets().
func (s *sloTracker) worstBudget() int {
	worst := 0
	for _, t := range s.byType {
		worst = max(worst, t.ms)
	}
	return worst
}

// sampleState collects the raw TTFF/segment-fetch samples the run measures,
// bucketed per tier (and, for segments, per viewer type too), so the final
// report can read back whichever tier turned out to be the verdict.
type sampleState struct {
	tierSegments    map[int][]int
	tierTTFF        map[int][]int
	perTypeSegments map[viewerType]map[int][]int
	// Per hold (window id): the raw fetch times, the same fetches scaled to the
	// reference segment length (what the stall verdict reads), and the first
	// fetches. A tier measured twice keeps both holds apart.
	windowSegments       map[int][]int
	windowSegmentsScaled map[int][]int
	windowTTFF           map[int][]int
	// windowScaledByType keeps a window's scaled fetches per viewer type: each
	// type is judged against its own budget.
	windowScaledByType map[int]map[viewerType][]int
}

func newSampleState() sampleState {
	return sampleState{
		tierSegments:         map[int][]int{},
		tierTTFF:             map[int][]int{},
		perTypeSegments:      map[viewerType]map[int][]int{},
		windowSegments:       map[int][]int{},
		windowSegmentsScaled: map[int][]int{},
		windowTTFF:           map[int][]int{},
		windowScaledByType:   map[int]map[viewerType][]int{},
	}
}

// recordSegment files one fetch under its hold, raw and scaled.
func (s *sampleState) recordSegment(windowID int, vt viewerType, rawMs, scaledMs int) {
	if windowID <= 0 {
		return
	}
	s.windowSegments[windowID] = append(s.windowSegments[windowID], rawMs)
	s.windowSegmentsScaled[windowID] = append(s.windowSegmentsScaled[windowID], scaledMs)
	if s.windowScaledByType == nil {
		s.windowScaledByType = map[int]map[viewerType][]int{}
	}
	if s.windowScaledByType[windowID] == nil {
		s.windowScaledByType[windowID] = map[viewerType][]int{}
	}
	s.windowScaledByType[windowID][vt] = append(s.windowScaledByType[windowID][vt], scaledMs)
}

// recordTTFF files one first fetch under its hold.
func (s *sampleState) recordTTFF(windowID, ms int) {
	if windowID <= 0 {
		return
	}
	s.windowTTFF[windowID] = append(s.windowTTFF[windowID], ms)
}

// windowPercentiles reads a hold's latencies back: raw p50 and p95, the
// scaled p95 the verdict is read on, how many fetches, and the p95 of first
// fetches.
func (s *sampleState) windowPercentiles(windowID int) (p50, p95, p95Scaled, n, ttffP95 int) {
	raw := s.windowSegments[windowID]
	return percentile(raw, 50), percentile(raw, 95), percentile(s.windowSegmentsScaled[windowID], 95), len(raw), percentile(s.windowTTFF[windowID], 95)
}

// resetTier drops a tier's segment-latency samples before a measured hold: the
// tier number may have been visited before (a FAST pass), and those fetches
// happened inside a join burst that must not colour the tier's percentiles.
// TTFF is kept - joining is the only thing it ever measures.
func (s *sampleState) resetTier(tier int) {
	delete(s.tierSegments, tier)
	for _, byTier := range s.perTypeSegments {
		delete(byTier, tier)
	}
}

// reportFields reads back the samples at sampleTier - the tier the run's
// verdict settled on - into the shapes Report needs.
func (s *sampleState) reportFields(sampleTier int, ramp *rampState) (segSamples, ttffSamples []int, mix, p95ByType map[string]int) {
	segSamples = s.tierSegments[sampleTier]
	for t := 1; t <= sampleTier; t++ {
		ttffSamples = append(ttffSamples, s.tierTTFF[t]...)
	}

	mix = map[string]int{}
	for slot := 1; slot <= sampleTier; slot++ {
		if vt, ok := ramp.viewerTypeBySlot[slot]; ok {
			mix[string(vt)]++
		}
	}
	p95ByType = map[string]int{}
	for vt, byTier := range s.perTypeSegments {
		if samples := byTier[sampleTier]; len(samples) > 0 {
			p95ByType[string(vt)] = percentile(samples, 95)
		}
	}
	return segSamples, ttffSamples, mix, p95ByType
}
