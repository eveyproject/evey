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
	// Path is where the file actually lives, so the run can measure how fast
	// this box reads its media (see diskbench.go). Empty is tolerated: the disk
	// probe is simply skipped.
	Path string
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
	// supplied by the parent. Viewers hit real HTTP endpoints on it. Always
	// reachable, and it measures the SERVER alone.
	BaseURL string
	// PublicBaseURL is the origin real viewers actually come through - the
	// reverse proxy (Caddy), with its TLS, HTTP/2 and buffering. Optional. When
	// set AND reachable, the run uses it instead of the loopback, which is a
	// truer picture of what a household experiences: the proxy is part of the
	// delivery path, and its own limits are part of the answer. When it cannot
	// be reached (no proxy deployed, a public hostname the box can't resolve
	// from inside), the run silently falls back to BaseURL rather than failing -
	// a measurement of the server alone beats no measurement.
	PublicBaseURL string
	// Token is the bearer token used by every simulated viewer. It is a real
	// credential exercising the real auth path, not a bypass.
	Token string
	// Scenario is ScenarioTranscode or ScenarioNatural.
	Scenario string
	// MaxViewers caps how far the climb will go. ZERO MEANS NO CEILING: the
	// run climbs until a resource actually gives out, which is the only number
	// worth reporting. A ceiling is an operator's own limit, not the machine's,
	// and a run that stops there has measured nothing about the server - the
	// report says so explicitly when it happens.
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
	// ResourceSampleEvery overrides the resource sampling cadence. Zero means
	// resourceSampleEvery. Tests shrink it; an operator has no reason to.
	ResourceSampleEvery time.Duration

	// FastHold / MeasureHold override the climb's per-phase observation windows
	// (see climb.go). Zero means the real ones. Tests shrink them so a whole
	// climb runs in milliseconds; an operator has no reason to touch them.
	FastHold    time.Duration
	MeasureHold time.Duration
}

func (c Config) fsRoot() string {
	if c.FSRoot == "" {
		return "/"
	}
	return c.FSRoot
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
	// DefaultMaxViewers is ZERO: no ceiling. The climb stops when a RESOURCE
	// gives out, not when it reaches a number someone picked in advance. A
	// direct-play viewer can cost the server almost nothing, so any hardcoded
	// cap would just be a cap on the answer.
	DefaultMaxViewers = 0

	// DefaultRampEvery is unused by the adaptive climb (which sets its own hold
	// per phase - see climb.go) and kept only for ModeFlash's observation
	// window.
	DefaultRampEvery = 5 * time.Second

	// DefaultMaxDuration walls the whole run. The climb is fast (batches while
	// the box is idle, a bisection near the ceiling), so this is a backstop
	// rather than a schedule.
	DefaultMaxDuration          = 20 * time.Minute
	DefaultEnduranceMaxDuration = 45 * time.Minute
)

// defaultFlashViewers is how big a flash slam is when the operator didn't say.
// The climb discovers its own size; a slam cannot, so it needs a number - and
// "everything in the pool" would be thousands of viewers at once.
const defaultFlashViewers = 20

// endpointProbeTimeout bounds the one-shot check of the reverse proxy before a
// run commits to it. Short: it is a local hop, and a slow answer here means the
// proxy is not a sane place to send hundreds of viewers.
const endpointProbeTimeout = 3 * time.Second

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
	// StopThroughputSaturated: adding viewers stopped adding throughput. The box
	// is already pushing as many bytes per second as it can pull, so every new
	// viewer just takes a slice of the same pie. On a media server this is the
	// usual ceiling - not the CPU, not the RAM, but how fast it can READ the
	// films - and no CPU or memory gauge can see it.
	StopThroughputSaturated StopReason = "throughput_saturated"
	// StopServerFailing: the server stopped serving a viewer altogether under
	// this load - repeated failed fetches, connections it could not honour. It
	// is the box giving out, not the harness misbehaving, so it is a capacity
	// verdict and the climb narrows around it like any other.
	StopServerFailing StopReason = "server_failing"
	// StopStartupTooSlow: the server still feeds its existing viewers, but a
	// NEW one now waits seconds for its first frame. To the person holding the
	// remote that is indistinguishable from a broken app, so it is a capacity
	// verdict like any other.
	StopStartupTooSlow StopReason = "startup_too_slow"
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

	// ReplayedTitleViewers counts viewers of the sustained tier that were
	// placed on a title another viewer was already watching, at a DIFFERENT
	// position. They are not served from any cache - the mkvgo path packages
	// each segment from the source on request - but they do read a file the
	// operating system may already be holding in memory, which is what
	// DiskReadStarved below is about.
	ReplayedTitleViewers int

	// Endpoint is the origin the viewers actually hit: the reverse proxy when it
	// was reachable (real delivery path, TLS and all), the server's own loopback
	// otherwise. Two runs are not comparable across different endpoints.
	Endpoint string
	// ViaProxy is true when Endpoint was the reverse proxy rather than the
	// server's loopback.
	ViaProxy bool

	// DiskReadBytesPerSecCeiling is how fast this box can READ its media - every
	// disk the library spans, read at once. This is the number that decides a
	// direct-play server's capacity, and the kernel cannot report it when the
	// library sits on anything but a plain block device. Zero when no file could
	// be probed.
	DiskReadBytesPerSecCeiling int64
	// SlowestDiskBytesPerSec is what the WEAKEST drive delivered. On a library
	// spread over several disks, this is what decides how bad it gets when
	// viewers happen to pick films that live on that one: four drives are not
	// one big drive, they each saturate on their own.
	SlowestDiskBytesPerSec int64
	// DisksProbed is how many distinct drives the library was found to span.
	DisksProbed int
	// StorageCollapsesAtReaders is the number of concurrent readers past which
	// the storage stops merely plateauing and FALLS OVER - throughput halves or
	// worse, and a single read starts taking seconds. Zero when it never did.
	//
	// When it is not zero, it is the most important number in the whole report:
	// on a direct-play server it is roughly how many people can watch at once
	// before the STORAGE - not the CPU, not Evey - makes playback crawl. A 9p or
	// virtiofs mount multiplexed over one channel, an over-subscribed NFS export
	// or a spinning disk thrashing between streams all do this, and no CPU or
	// memory gauge shows any of it.
	StorageCollapsesAtReaders int

	// ThroughputCeilingBytesPerSec is the byte rate the server could not exceed,
	// set when StopThroughputSaturated. It is the number an operator acts on:
	// the storage path (disk, array, or the protocol in front of it) delivers
	// this much and no more.
	ThroughputCeilingBytesPerSec int64

	// DiskReadStarved is true when the server pushed real traffic while
	// reading essentially NOTHING from disk: every byte came from the OS page
	// cache. The viewer count is still valid, but it was measured on a machine
	// whose storage was never asked to work - so it says nothing about what
	// happens when the library is too big to sit in RAM. The library is simply
	// too small (or too repeatedly read) to exercise the disk.
	DiskReadStarved bool

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
