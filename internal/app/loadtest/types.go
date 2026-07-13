// Package loadtest simulates real HLS viewers against Evey's own HTTP stack so
// an admin can measure how many concurrent streams the running server (on its
// actual hardware) can sustain, without touching a third-party tool or writing
// to disk. It is deliberately read-only from the server's point of view: every
// request a simulated viewer makes is indistinguishable from a real player
// hitting the same endpoints with a real bearer token.
package loadtest

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAlreadyRunning is returned by Start when a load test is already in
// progress; only one run is allowed at a time.
var ErrAlreadyRunning = errors.New("loadtest: already running")

// ErrStreamsActive is returned by Start when real viewers are currently
// streaming. Load testing on top of real traffic would both skew the
// measurement and steal capacity from real users, so it is refused outright.
var ErrStreamsActive = errors.New("loadtest: real streams are active")

// ErrUnknownMode is returned by Start when Config.Mode is set to something
// other than "" (ramp) or one of the Mode* constants. Unlike an unrecognised
// Scenario (which falls back to "natural" - a household never crashes just
// because an admin fat-fingered a scenario name), an unknown Mode fails fast:
// the three modes drive fundamentally different stop/verdict logic, and
// silently falling back to ramp would misreport what was actually measured.
var ErrUnknownMode = errors.New("loadtest: unknown mode")

// MediaCandidate is one piece of media the runner may point simulated
// viewers at.
type MediaCandidate struct {
	ID          string
	DurationSec int
}

// MediaLister supplies the pool of media the load test can pick viewers'
// streams from. The parent wires this to its media query service.
type MediaLister interface {
	ListLoadTestCandidates(ctx context.Context, limit int) ([]MediaCandidate, error)
}

// Scenario selects how a simulated viewer negotiates playback.
const (
	// ScenarioTranscode forces every viewer onto the transcode path
	// (vd=transcode&ad=transcode), the worst case for CPU/GPU capacity.
	ScenarioTranscode = "transcode"
	// ScenarioNatural lets each viewer negotiate playback the way a real
	// client would (no forced decision), and never shares a media item
	// across two simulated viewers so direct-play/remux cache hits can't
	// flatter the numbers.
	ScenarioNatural = "natural"
	// ScenarioMkvgo measures how far playback served by the mkvgo on-demand
	// packager alone climbs: natural negotiation, distinct media only, and
	// every response is verified to actually come from mkvgo (the
	// X-Evey-Served-By header) rather than falling back to a transcode -
	// zero encode, zero capacity slot burned.
	ScenarioMkvgo = "mkvgo"
	// ScenarioLight, ScenarioBalanced and ScenarioHeavy simulate a mixed
	// household: each simulated viewer is independently typed mkvgo or
	// transcode by a deterministic weighted pattern (see scenarioViewerType),
	// and behaves exactly like ScenarioMkvgo or ScenarioTranscode
	// respectively. These answer "how many different users can this
	// machine serve" rather than a single worst/best case.
	ScenarioLight    = "light"    // 80% mkvgo / 20% transcode
	ScenarioBalanced = "balanced" // 50% / 50%
	ScenarioHeavy    = "heavy"    // 20% mkvgo / 80% transcode
)

// Mode selects HOW the ramp behaves; it is orthogonal to Scenario (WHO the
// simulated viewers are). The zero value is the classic ramp: one viewer
// joins every RampEvery until a verdict stops the run.
const (
	// ModeFlash is the 9pm-thunder mode: no ramp, min(MaxViewers, available
	// candidates) viewers all start at once, and the run measures how many
	// survive a 3xRampEvery observation window rather than how far a gradual
	// ramp climbs.
	ModeFlash = "flash"
	// ModeSeek is the seek-storm mode: a normal ramp, but every viewer jumps
	// to a pseudo-random position in its playlist every 10 segment-durations,
	// measuring the churn a real "everyone scrubs the timeline" pattern
	// causes on the server.
	ModeSeek = "seek"
	// ModeEndurance is the marathon mode: a normal ramp up to MaxViewers,
	// then holds that plateau (SLO/admission verdicts still active) until
	// MaxDuration, to catch a slow drift a short run would miss.
	ModeEndurance = "endurance"
)

// Config configures one load test run.
type Config struct {
	// BaseURL is the server's own loopback origin (e.g. http://127.0.0.1:8080),
	// supplied by the parent. Viewers hit real HTTP endpoints on it.
	BaseURL string
	// Token is the bearer token used by every simulated viewer. It is a real
	// credential exercising the real auth path, not a bypass.
	Token string
	// Scenario is ScenarioTranscode or ScenarioNatural.
	Scenario string
	// MaxViewers caps how far the ramp will go. Defaults to 12: a weak NAS
	// is the target hardware, so the default stays conservative and an
	// admin who wants more has to ask for it explicitly.
	MaxViewers int
	// RampEvery is both the cadence at which a new viewer joins and the
	// minimum observation window given to each tier before advancing.
	RampEvery time.Duration
	// SegmentSLOMs is the segment-fetch latency budget. Zero means auto:
	// the detected #EXT-X-TARGETDURATION of the stream, in milliseconds
	// (a segment that takes longer to arrive than its own playback
	// duration means a real player would stall).
	SegmentSLOMs int
	// MaxDuration is a hard wall on the whole run, regardless of ramp
	// progress. Defaults to 4 minutes, except in ModeEndurance, which
	// defaults to 30 minutes - the whole point of that mode is to hold a
	// plateau long enough to catch a slow drift.
	MaxDuration time.Duration
	// Mode is "" (ramp, the default), ModeFlash, ModeSeek or ModeEndurance.
	// It is the HOW; Scenario stays the WHO.
	Mode string

	// FSRoot overrides the filesystem root the resource sampler reads its
	// counters from (/sys/fs/cgroup, /proc). Empty means the real one; tests
	// stage a fake tree here.
	FSRoot string
	// ResourceWarmup overrides how long a tier's samples are discarded as
	// start-up transients. Zero means resourceWarmup. Tests shrink it; an
	// operator has no reason to.
	ResourceWarmup time.Duration
	// ResourceSampleEvery overrides the resource sampling cadence. Zero means
	// resourceSampleEvery. Tests shrink it; an operator has no reason to.
	ResourceSampleEvery time.Duration
}

func (c Config) fsRoot() string {
	if c.FSRoot == "" {
		return "/"
	}
	return c.FSRoot
}

func (c Config) resourceWarmup() time.Duration {
	if c.ResourceWarmup <= 0 {
		return resourceWarmup
	}
	return c.ResourceWarmup
}

func (c Config) resourceSampleEvery() time.Duration {
	if c.ResourceSampleEvery <= 0 {
		return resourceSampleEvery
	}
	return c.ResourceSampleEvery
}

// Defaults applied when the corresponding Config field is left at its zero
// value.
const (
	DefaultMaxViewers = 12

	// DefaultRampEvery is how long a tier is held before the next viewer
	// joins, and it is the single most important number in the whole test: it
	// must be long enough for the tier to reach STEADY STATE, or the run
	// measures start-up transients instead of what a viewer actually costs.
	// A joining viewer pays an ffmpeg cold start (up to ~25s worst case, see
	// transcode_vod_wire.go), then bursts segments to fill its buffer, then
	// finally settles into real-time pacing. resourceWarmup covers that
	// transient; the rest of the tier is the measurement window.
	DefaultRampEvery = 60 * time.Second

	// DefaultMaxDuration must be long enough for the ramp to actually reach
	// MaxViewers (MaxViewers x RampEvery), plus room to spare. A run cut short
	// by the clock reports a viewer count that measures the timeout, not the
	// server.
	DefaultMaxDuration          = 30 * time.Minute
	DefaultEnduranceMaxDuration = 45 * time.Minute
)

// candidatePoolSize is how many titles the runner asks the library for. It is
// deliberately far above any plausible MaxViewers: the pool multiplies titles
// by start position, and a generous list is what lets an ineligible title be
// skipped without costing a viewer. Listing a few hundred rows is free next to
// the run itself.
const candidatePoolSize = 300

func (c Config) withDefaults() (Config, error) {
	switch c.Mode {
	case "", ModeFlash, ModeSeek, ModeEndurance:
	default:
		return c, fmt.Errorf("%w: %q", ErrUnknownMode, c.Mode)
	}
	if c.MaxViewers <= 0 {
		c.MaxViewers = DefaultMaxViewers
	}
	if c.RampEvery <= 0 {
		c.RampEvery = DefaultRampEvery
	}
	if c.MaxDuration <= 0 {
		if c.Mode == ModeEndurance {
			c.MaxDuration = DefaultEnduranceMaxDuration
		} else {
			c.MaxDuration = DefaultMaxDuration
		}
	}
	if c.Scenario == "" {
		c.Scenario = ScenarioNatural
	}
	return c, nil
}

// StopReason names why a run ended.
type StopReason string

const (
	// StopSLOBreach: the rolling p95 segment-fetch latency crossed the SLO,
	// meaning a real player at that viewer count would stall.
	StopSLOBreach StopReason = "slo_breach"
	// StopAdmissionFull: the server itself refused a stream with
	// X-Evey-Error: too_many_streams. This is the server's own capacity
	// verdict, not a load test error.
	StopAdmissionFull StopReason = "admission_full"
	// StopCPUSaturated: the server's CPU sat at its ceiling long enough that
	// it had nothing left to give another viewer. This is a capacity verdict
	// and it names the bottleneck.
	StopCPUSaturated StopReason = "cpu_saturated"
	// StopMemorySaturated: the server's memory reached its limit; the next
	// viewer risks the OOM killer rather than a slowdown.
	StopMemorySaturated StopReason = "memory_saturated"
	// StopLibraryLimit: the media pool is empty or holds nothing the scenario
	// can use (e.g. an mkvgo run against a library where every title is
	// transcoded). NOT a capacity verdict, and no longer reachable just
	// because the library is smaller than MaxViewers - the pool replays a
	// title at a different start position instead.
	StopLibraryLimit StopReason = "library_limit"
	// StopMaxViewers: the ramp reached Config.MaxViewers cleanly.
	StopMaxViewers StopReason = "max_viewers"
	// StopCancelled: Cancel was called, or the caller's context was
	// cancelled, while viewers were still healthy.
	StopCancelled StopReason = "cancelled"
	// StopTokenExpired: a viewer got a 401; the admin's token expired
	// mid-run.
	StopTokenExpired StopReason = "token_expired"
	// StopError: a viewer failed repeatedly (3 consecutive non-admission
	// failures) or another unrecoverable error occurred.
	StopError StopReason = "error"
	// StopCompleted: a bounded mode (ModeFlash's observation window, or
	// ModeEndurance's held plateau) ran its full fixed duration cleanly, as
	// opposed to the ramp's StopMaxViewers (which means "climbed as high as
	// configured") or any of the failure verdicts above.
	StopCompleted StopReason = "completed"
)

// Report is the outcome of a completed (or cancelled) run.
type Report struct {
	Scenario         string
	Mode             string
	SustainedViewers int
	StopReason       StopReason
	StopDetail       string

	TTFFMsP50, TTFFMsP95       int
	SegmentMsP50, SegmentMsP95 int
	// SegmentSLOMs is the loosest budget in play, kept for callers that want a
	// single number. It is NOT what any given viewer was judged against.
	SegmentSLOMs int
	// SegmentSLOMsByType is the latency budget each path was actually judged
	// against, e.g. {"mkvgo": 11000, "transcode": 3000}. The paths do not share
	// a segment length, so they cannot share a budget: mkvgo packages on
	// keyframe boundaries while the ffmpeg ladder emits 3s segments.
	SegmentSLOMsByType map[string]int
	Samples            int

	// SharedCacheViewers counts viewers that reused an already-in-flight
	// media item (via a forked audio-resync param) because the library ran
	// out of distinct candidates. Only non-zero for transcode-typed viewers.
	SharedCacheViewers int

	// SkippedCandidates counts media items an mkvgo-typed viewer tried and
	// abandoned because the server didn't actually serve them via mkvgo
	// (missing X-Evey-Served-By: mkvgo). Each skip is retried against the
	// next distinct candidate without counting as a sustained viewer.
	SkippedCandidates int

	// Resources is what the server consumed at the sustained tier: the answer
	// to "what did those N viewers cost", and the evidence behind a
	// StopCPUSaturated / StopMemorySaturated verdict.
	Resources ResourceStats
	// ResourceBaseline is what the server consumed BEFORE the first viewer
	// joined. An operator reads the per-viewer cost against this floor, not
	// against zero.
	ResourceBaseline ResourceSample
	// PerViewerCost is what ONE more viewer costs in steady state, measured
	// as the slope across tiers rather than a naive total/viewers (which
	// would charge the idle server's own consumption to the viewers).
	// HasPerViewerCost is false when fewer than two tiers were measured.
	PerViewerCost    ResourceStats
	HasPerViewerCost bool

	// ViewerMix is the viewer-type composition of the sustained tier, e.g.
	// {"mkvgo": 4, "transcode": 1}. Populated for every scenario (a pure
	// scenario reports a single key).
	ViewerMix map[string]int
	// SegmentMsP95ByType is the per-type p95 segment-fetch latency at the
	// sustained tier, so a slow viewer type can't hide behind a fast one in
	// the blended SegmentMsP95.
	SegmentMsP95ByType map[string]int

	StartedAt, EndedAt time.Time
}

// Status is a live snapshot of the current (or most recent) run.
type Status struct {
	Running    bool
	Phase      string
	Viewers    int
	Elapsed    time.Duration
	LastReport *Report
}
