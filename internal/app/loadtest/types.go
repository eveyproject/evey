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

// ErrUnknownScenario is returned by Start for a client mix it cannot play (a
// profile it does not know, a weight that is not a number). Unlike a mistyped
// plain scenario, a mix is a list of claims the report would otherwise make
// about clients that never played.
var ErrUnknownScenario = errors.New("loadtest: invalid client mix")

// MediaCandidate is one piece of media the runner may point simulated
// viewers at.
type MediaCandidate struct {
	ID          string
	DurationSec int
	// VideoCodec / VideoHeight name the DECODE workload this title imposes. A
	// run's verdict is meaningless without them: the same output rung costs
	// ~4x more from a 4K HEVC source than from a 1080p H.264 one (measured
	// 2.12x vs 8.43x realtime), so "N viewers" is a different claim depending
	// on what was drawn. Empty/0 when the streams were never probed.
	VideoCodec  string
	VideoHeight int
	// VideoRange is the source's dynamic range (RangeSDR, RangeHDR10, ...):
	// a tone map or a Dolby Vision profile 5 costs an encode several times
	// what an SDR source of the same height does. Empty when unprobed.
	VideoRange string
	// BitrateBps is the file's overall bitrate: what a direct-play viewer of
	// this title pulls from the disk and the network. 0 when unknown.
	BitrateBps int64
	// Path is where the file actually lives, so the run can measure how fast
	// this box reads its media (see diskbench.go). Empty is tolerated: the disk
	// probe is simply skipped.
	Path string
}

// Dynamic ranges a candidate's video may carry (MediaCandidate.VideoRange).
// Dolby Vision profile 5 is apart: its base layer is not displayable without
// DoVi, so it is encoded for every client.
const (
	RangeSDR   = "sdr"
	RangeHDR10 = "hdr10"
	RangeHLG   = "hlg"
	RangeDoVi  = "dovi"
	RangeDoVi5 = "dovi5"
)

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
	// reverse proxy (Caddy), with its TLS, HTTP/2 and buffering. Optional, and
	// only used when ViaProxy asks for it: the default run measures the SERVER
	// and hits BaseURL. A proxy run is a different measurement - the proxy's
	// own limits are part of its answer (a hairpin through the operator's box
	// and a second TLS hop added 200 ms to the median fetch on 2026-10-15), and
	// the report says which path was taken. When the proxy cannot be reached
	// (none deployed, a public hostname the box can't resolve from inside), the
	// run falls back to BaseURL and says so.
	PublicBaseURL string
	// ViaProxy sends the viewers through PublicBaseURL instead of the loopback.
	ViaProxy bool
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
	// ColdCache keeps evicting the library's files from the OS page cache for
	// the whole run, so the server's reads hit the actual storage. On a box
	// whose library fits in RAM, a default (warm) run measures the memory bus
	// and overstates the viewer count "sometimes by a factor of several" - its
	// own report says so. Cold-cache mode measures the day the library dwarfs
	// the RAM, which is what a growing library eventually does. Neither figure
	// is wrong; they answer different days, and the report says which one was
	// measured.
	ColdCache bool

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

	// ProfileHarnessCPU turns on the one measurement that separates the server's
	// CPU from the load generator's (see harnesscpu.go). On by default for a real
	// run: without it, the harness's own HTTP overhead is billed to the server and
	// a viewer looks several times more expensive than it is. Tests leave it off -
	// a CPU profile in a unit test measures the test.
	ProfileHarnessCPU bool
	// HarnessProfileFor is how long that profile runs. Zero means the default.
	HarnessProfileFor time.Duration
	// Seed orders the draw of titles. Zero (the default) takes a fresh one from
	// the clock; the run logs and reports whichever it used, so a draw can be
	// replayed by passing it back.
	Seed uint64
	// BaselineFor overrides how long the idle server is watched before the first
	// viewer joins. Zero means baselineFor, scaled down when a test shrinks the
	// sampling or ramp cadence; negative skips the baseline.
	BaselineFor time.Duration
}

// TierCost is what ONE audience size cost the server: one row of the report's
// table. Every CPU figure is the SERVER's share - the load generator's own
// overhead has already been subtracted (see harnesscpu.go) - and MemUsedBytes is
// anonymous memory, never the page cache of the films being read.
type TierCost struct {
	Viewers    int
	CPUPercent float64
	// MachineCPUPercent / MachineIOWaitPct are the whole machine's busy and
	// iowait shares beside the container's CPU: what served the viewers from
	// outside the container, and the storage path saturating.
	MachineCPUPercent float64
	MachineIOWaitPct  float64
	MemUsedBytes      int64
	// ProcessRSSBytes is the server process alone; MemUsedBytes is the whole
	// container (Meilisearch and Postgres included in the -full image).
	ProcessRSSBytes     int64
	DiskReadBytesPerSec int64
	ServedBytesPerSec   int64
	Samples             int
	// GoHeapLiveBytes / GoGCCPUPercent: the server's own Go runtime over this
	// tier - how much heap was reachable and how much CPU the collector took.
	GoHeapLiveBytes int64
	GoGCCPUPercent  float64
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

// baselineFor is how long the idle server is watched before the first viewer.
// A test that shrinks the sampling cadence gets a baseline of a few samples
// at that cadence rather than five real seconds.
func (c Config) baselineFor() time.Duration {
	switch {
	case c.BaselineFor < 0:
		return 0
	case c.BaselineFor > 0:
		return c.BaselineFor
	case c.ResourceSampleEvery > 0:
		return 5 * c.ResourceSampleEvery
	}
	// A shrunk hold or ramp cadence (a test) shrinks the baseline with it: five
	// real seconds in front of a hold of a few hundred milliseconds would be
	// the whole run.
	out := baselineFor
	for _, d := range []time.Duration{c.RampEvery, c.FastHold, c.MeasureHold} {
		if d > 0 && d < out {
			out = d
		}
	}
	return out
}

// evictEvery is the cold-cache re-eviction cadence: coldCacheEvictEvery, or
// twice a shrunk sampling cadence under test.
func (c Config) evictEvery() time.Duration {
	if c.ResourceSampleEvery > 0 {
		return 2 * c.ResourceSampleEvery
	}
	return coldCacheEvictEvery
}

func (c Config) harnessProfileFor() time.Duration {
	if c.HarnessProfileFor <= 0 {
		return harnessProfileWindow
	}
	return c.HarnessProfileFor
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
// SourceMixEntry is one source class and how many viewers were placed on it.
type SourceMixEntry struct {
	Codec  string
	Height int
	// Range is the class's dynamic range (RangeSDR, RangeHDR10, ...).
	Range string
	Count int
}

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

	// SourceMix is what the run actually served, per source class, counted at
	// the moment each title was handed to a viewer. Without it a verdict cannot
	// be read: the candidate pool is drawn by duration alone, so two runs on the
	// same host legitimately land on different numbers purely by codec draw.
	SourceMix []SourceMixEntry

	// Endpoint is the origin the viewers actually hit: the server's own
	// loopback by default, the reverse proxy when the run asked for it and it
	// answered. Two runs are not comparable across different endpoints.
	Endpoint string
	// ViaProxy is true when Endpoint was the reverse proxy rather than the
	// server's loopback.
	ViaProxy bool
	// Seed is the title draw's seed: passing it back replays the same draw.
	Seed uint64

	// DiskDevices is the mapping the run resolved at its start: which block
	// device each library location (a branch, a mount) is read from. Device
	// letters rotate at reboot; the mapping is what makes the disk lines below
	// readable later. DiskReadMeasured is false when no device could be tied to
	// the library - the disk figures are then absent, not zero.
	DiskDevices      []DeviceMapping
	DiskReadMeasured bool
	// AppReadMeasured is false when nothing counts, in-process, the bytes the
	// packager read: the app-read figures are then absent.
	AppReadMeasured bool

	// Windows is every hold the run made, in order, broken ones included: the
	// climb as it happened, one row per window. A tier measured twice is two
	// rows. Tiers below keeps one row per measured tier for the summary.
	Windows []TierWindow
	// DiskVerdict judges the busiest disk of the hold the stop is read on: the
	// lowest tier that broke when one did, else the sustained tier.
	DiskVerdict DiskVerdict
	// BreakReason / BreakDetail name what broke BreakViewers, when a tier did.
	BreakReason StopReason
	BreakDetail string

	// DiskReadBytesPerSecCeiling is how fast this box can READ its media - every
	// disk the library spans, read at once. This is the number that decides a
	// direct-play server's capacity, and the kernel cannot report it when the
	// library sits on anything but a plain block device. Zero when no file could
	// be probed.
	DiskReadBytesPerSecCeiling int64
	// DiskCeilingAtReaders is the concurrency width at which that ceiling was
	// reached. It is meaningless on its own and indispensable next to the figure
	// above: a peak measured at 4 sequential readers says nothing about what 21
	// scrubbing viewers will get.
	DiskCeilingAtReaders int
	// DiskReadBytesPerSecAtAudience is what the same storage delivered at the
	// concurrency the sustained audience actually imposes on it. THIS is the
	// number the run is up against; the ceiling above is the number the hardware
	// gives under laboratory conditions it will never see in production.
	//
	// Reporting only the ceiling is how a saturated disk gets read as 40% busy.
	// The two together say whether the run stopped because the storage ran out of
	// bytes, or because it ran out of heads.
	DiskReadBytesPerSecAtAudience int64
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

	// ReadAmplification is how many bytes the DISK delivered for every byte
	// served, at the sustained tier. This is what the hardware actually paid.
	// Zero when the kernel could not account the reads (a library on a network
	// share or a 9p/virtiofs mount, where per-cgroup I/O accounting is blind).
	ReadAmplification float64
	// AppReadAmplification is how many bytes the SERVER ASKED FOR for every byte
	// served - the packager's own appetite, page cache or not.
	//
	// The two together name the culprit, and they have entirely different fixes:
	//
	//   - app 1.0, disk 1.0  -> nothing to win.
	//   - app 2.5, disk 2.5  -> the CODE reads data it does not serve (the
	//     packager traverses tracks it then discards). Fix it in the packager.
	//   - app 1.3, disk 2.6  -> the KERNEL is reading ahead into data nobody
	//     asked for. The code is nearly optimal; the fix is in how the file is
	//     read (fadvise, read sizes), not in what it reads.
	//
	// Reporting only the disk figure says "the server reads 2.5x what it serves"
	// and leaves an operator - or a packager author - guessing which of the two
	// they are looking at.
	AppReadAmplification float64

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

	// ColdCache is true when the run kept evicting the library from the page
	// cache (and at least one eviction actually landed): the figures were
	// measured against the storage, not the RAM. A warm and a cold run of the
	// same box are two different answers, and they must never be compared as
	// if they were one.
	ColdCache bool

	// StorageFSType names the filesystem type(s) the library's files sit on
	// ("ext4", "fuse.mergerfs", "nfs4", ...), comma-joined when several.
	StorageFSType string

	// StorageOpaque is true when at least one of those mounts hides the disks
	// from this kernel (a FUSE pool, a network or VM share): the disk counter
	// cannot see its reads, a page-cache eviction stops at the mount, and a
	// pool counts as one disk. The report states it up front so the storage
	// figures are read as "the delivery path", not "the disks".
	StorageOpaque bool
	// PoolBranches is where the audience's titles sit on a pooled mount, one
	// entry per branch, most viewers first. Empty when the library is not on a
	// pool that can say. A pool is not a RAID: a file lives on one disk, so a
	// skewed placement is a skewed load whatever the pool's total capacity.
	PoolBranches []BranchShare

	// SkippedCandidates counts media items an mkvgo-typed viewer tried and
	// abandoned because the server didn't actually serve them via mkvgo
	// (missing X-Evey-Served-By: mkvgo). Each skip is retried against the
	// next distinct candidate without counting as a sustained viewer.
	SkippedCandidates int

	// Resources is what the server consumed at the sustained tier: the answer
	// to "what did those N viewers cost", and the evidence behind a
	// StopCPUSaturated / StopMemorySaturated verdict.
	Resources ResourceStats
	// BreakViewers and BreakResources are the lowest audience that broke and
	// what the server consumed under it: where the cause of a stop shows,
	// since the sustained tier is by definition the one that held. Zero and
	// empty when no tier broke (a flash run, the operator's limit).
	BreakViewers   int
	BreakResources ResourceStats
	// ResourceBaseline is what the server consumed BEFORE the first viewer
	// joined, averaged over the baseline window. An operator reads the
	// per-viewer cost against this floor, and the disk verdict reads each
	// device's wait against its wait at rest here.
	ResourceBaseline ResourceStats
	// PerViewerCost is what ONE more viewer costs in steady state, measured
	// as the slope across tiers rather than a naive total/viewers (which
	// would charge the idle server's own consumption to the viewers).
	// HasPerViewerCost is false when fewer than two tiers were measured.
	PerViewerCost    ResourceStats
	HasPerViewerCost bool

	// Tiers is one row per audience size the run actually measured, smallest
	// first: the climb itself, not just the rung it stopped on.
	//
	// A single final number cannot say WHY the run stopped where it did. The
	// table can: read the per-viewer cost down the rows and it either holds
	// steady (the server is scaling, and the ceiling is elsewhere) or it bends
	// upward (the server is starting to struggle, and the ceiling is real).
	Tiers []TierCost

	// HarnessCPUFraction is the share of the measured CPU burned by the load
	// GENERATOR - the simulated viewers draining loopback traffic - rather than by
	// the server. HarnessCPUMeasured is false when no profile could be taken, in
	// which case every CPU figure here is the pessimistic reading (all of it
	// charged to the server).
	//
	// It is not a detail. A run measured 25% CPU for 21 viewers; billing the
	// harness to the server makes a viewer look several times more expensive than
	// it is, and sizes an operator's NAS accordingly.
	HarnessCPUFraction float64
	HarnessCPUMeasured bool

	// ViewerMix is the viewer-type composition of the sustained tier, e.g.
	// {"mkvgo": 4, "transcode": 1}. Populated for every scenario (a pure
	// scenario reports a single key).
	ViewerMix map[string]int
	// PathsByType is what the server decided for each viewer type over the
	// whole run, counted once per viewer that started, by the path its first
	// segment came from: e.g. {"client:web-chrome": {"mkvgo": 30, "transcode":
	// 12}}. It is how a client-mix verdict says what it was measured on: the
	// same audience costs a different server depending on who could copy what.
	PathsByType map[string]map[string]int
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
