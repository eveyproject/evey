package loadtest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// maxRequestTimeout is the ceiling applied to the http.Client passed to New:
// a load test must never let a single stuck request hang the whole run past
// what a real player would tolerate.
const maxRequestTimeout = 30 * time.Second

// eventKind classifies a message a viewer goroutine sends back to the run
// loop, the single owner of all cross-viewer state.
type eventKind int

const (
	evTargetDuration eventKind = iota
	evTTFF
	evSegment
	evAdmissionFull
	evTokenExpired
	evViewerError
	// evCandidateSkipped: an mkvgo-typed viewer's candidate media wasn't
	// actually served by mkvgo. It never becomes a counted viewer; the run
	// loop retries the same ramp slot against the next distinct candidate.
	evCandidateSkipped
)

type viewerEvent struct {
	kind eventKind
	vt   viewerType
	ms   int
	// slot is the 1-based ramp slot the sending viewer was assigned at
	// spawn time. Only ModeFlash's verdict needs it (to bucket TTFF/shed
	// outcomes per viewer, since flash has no meaningful "tier" ordering);
	// every other mode ignores it.
	slot int
	// mediaID identifies the candidate an evCandidateSkipped event is about,
	// so the pool can retire the title instead of re-offering it forever.
	mediaID string
	detail  string
}

// Runner drives one load test at a time against the server's own HTTP stack.
type Runner struct {
	httpClient    *http.Client
	media         MediaLister
	activeStreams func() int
	logger        *slog.Logger

	// reapSessions, when wired, asks the server to kill the run's OWN leftover
	// streaming sessions (by media ID). See WithSessionReaper.
	reapSessions func(mediaIDs []string)

	// servedBytes is the running total of bytes every viewer has pulled this
	// run - which is exactly what the server had to push. It is the honest
	// measure of served throughput: the simulated viewers reach the server
	// over loopback, so no NIC counter would see this traffic. Reset at the
	// start of each run (only one runs at a time).
	servedBytes atomic.Int64

	// counters is where the resource sampler reads CPU/memory/disk from. nil
	// means the server's own cgroup; tests inject a modelled machine.
	counters counterSource

	// evictFile drops one file from the OS page cache (cold-cache mode). The
	// real one is posix_fadvise; tests record the calls instead.
	evictFile func(path string) error
	// mountFSType names the filesystem behind a library path (see libraryStorage);
	// swapped in tests, sysinfo.MountFSType in production.
	mountFSType func(path string) string

	// liveViewers is the audience size RIGHT NOW, updated as each viewer is
	// spawned or shed rather than once per tier. Status() reads it, so a caller
	// polling mid-resize sees the audience actually in flight instead of the
	// count from before the batch joined.
	liveViewers atomic.Int64

	// audienceEpoch bumps whenever viewers are shed. Their sessions on the
	// server are over, which anything modelling per-session capacity needs to
	// know.
	audienceEpoch atomic.Int64

	// liveTranscodeViewers is how many of the live audience are on the transcode
	// path - the ones that actually cost an encoder. Kept alongside the headcount
	// because the scarce resource on a real box is the encoder, not the number of
	// people watching: direct play is nearly free.
	liveTranscodeViewers atomic.Int64

	mu              sync.Mutex
	running         bool
	cancelFn        context.CancelFunc
	cancelRequested bool
	runStartedAt    time.Time
	status          Status
}

// New builds a Runner. httpClient is the caller's own client (its per-request
// timeout is clamped to 30s if unset or larger); media supplies the pool of
// streamable candidates; activeStreams reports how many real viewers are
// currently streaming, so Start can refuse to load test on top of them.
func New(httpClient *http.Client, media MediaLister, activeStreams func() int, logger *slog.Logger) *Runner {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if httpClient.Timeout <= 0 || httpClient.Timeout > maxRequestTimeout {
		httpClient.Timeout = maxRequestTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		httpClient:    httpClient,
		media:         media,
		activeStreams: activeStreams,
		logger:        logger,
		evictFile:     evictFromCache,
		mountFSType:   sysinfo.MountFSType,
		status:        Status{Phase: "idle"},
	}
}

// WithSessionReaper wires the hook the runner uses to kill ITS OWN leftover
// streaming sessions server-side, by media ID. An admitted encoder session
// outlives the simulated viewer that started it - the server only reclaims it
// on an inactivity timeout minutes long - so between tiers those leftovers
// still hold their admission slots and get the NEXT tier refused for load it
// does not produce (measured live 2026-08-30: a box calibrated for 25 streams
// reported "holds 1"). Reaping aggressively is safe: a run never starts over
// real viewers (see Start), so every live session is the run's own.
// Chainable; nil (the default) disables reaping.
func (r *Runner) WithSessionReaper(reap func(mediaIDs []string)) *Runner {
	r.reapSessions = reap
	return r
}

// Start kicks off a run asynchronously. It refuses to start when a run is
// already in progress (ErrAlreadyRunning) or when real viewers are currently
// streaming (ErrStreamsActive): a load test must never compete with real
// traffic for capacity, and never skew its own measurement by doing so.
func (r *Runner) Start(ctx context.Context, cfg Config) error {
	if r.activeStreams != nil && r.activeStreams() > 0 {
		return ErrStreamsActive
	}
	cfg, err := cfg.withDefaults()
	if err != nil {
		return err
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return ErrAlreadyRunning
	}
	runCtx, cancel := context.WithTimeout(ctx, cfg.MaxDuration)
	r.running = true
	r.cancelFn = cancel
	r.cancelRequested = false
	r.runStartedAt = time.Now()
	r.status = Status{Running: true, Phase: "starting", Viewers: 0}
	r.mu.Unlock()

	go r.run(runCtx, cfg)
	return nil
}

// Cancel stops the current run, if any, and lets it report StopCancelled
// with whatever it had measured so far. It is a no-op when no run is active.
func (r *Runner) Cancel() {
	r.mu.Lock()
	r.cancelRequested = true
	cancel := r.cancelFn
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Status returns a live snapshot of the current (or most recently finished)
// run.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.status
	if r.running {
		st.Elapsed = time.Since(r.runStartedAt)
		// The audience as it stands this instant, not as of the last tier
		// boundary: a batch of ten joining takes a moment, and a caller polling
		// through it must see the truth.
		st.Viewers = int(r.liveViewers.Load())
	}
	return st
}

func (r *Runner) setStatus(phase string, viewers int) {
	r.mu.Lock()
	r.status.Phase = phase
	r.status.Viewers = viewers
	r.mu.Unlock()
}

func (r *Runner) wasCancelRequested() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelRequested
}

func (r *Runner) finish(report Report) {
	args := []any{
		"scenario", report.Scenario,
		"mode", report.Mode,
		"sustained_viewers", report.SustainedViewers,
		"stop_reason", string(report.StopReason),
		"stop_detail", report.StopDetail,
		"ttff_ms_p50", report.TTFFMsP50,
		"ttff_ms_p95", report.TTFFMsP95,
		"segment_ms_p50", report.SegmentMsP50,
		"segment_ms_p95", report.SegmentMsP95,
		"segment_slo_ms", report.SegmentSLOMs,
		"samples", report.Samples,
		"replayed_title_viewers", report.ReplayedTitleViewers,
		"disk_read_starved", report.DiskReadStarved,
		"storage_fs_type", report.StorageFSType,
		"storage_opaque", report.StorageOpaque,
		// Whether the library was being evicted from the page cache throughout.
		// A warm and a cold run of the same box are two different answers, and a
		// log line that doesn't say which one this was invites comparing them.
		"cold_cache", report.ColdCache,
		"skipped_candidates", report.SkippedCandidates,
		"duration_ms", report.EndedAt.Sub(report.StartedAt).Milliseconds(),
		"cpu_pct_avg", int(report.Resources.CPUPercentAvg),
		"cpu_pct_peak", int(report.Resources.CPUPercentPeak),
		// The server's ANONYMOUS memory - what it genuinely holds. The page cache
		// of the films is reported apart (mem_cache_mb) and is not consumption:
		// the kernel hands it straight back. Conflating the two once reported
		// 4.3 GB "used" by a packager measured at under a megabyte per viewer.
		"mem_used_mb", report.Resources.MemUsedBytesAvg / (1 << 20),
		"mem_cache_mb", report.Resources.MemCacheBytesAvg / (1 << 20),
		"mem_limit_mb", report.Resources.MemLimitBytes / (1 << 20),
		// How much of the CPU above was the load GENERATOR rather than the server.
		// Unmeasured means everything was charged to the server (the pessimistic
		// reading), which the operator deserves to know.
		"cpu_harness_pct", int(report.HarnessCPUFraction * 100),
		"cpu_harness_measured", report.HarnessCPUMeasured,
		// Two amplifications, because they have different fixes: what the DISK
		// delivered per byte served, and what the CODE asked for. A gap between
		// them is the kernel reading ahead, not a greedy packager.
		"read_amplification_disk", report.ReadAmplification,
		"read_amplification_app", report.AppReadAmplification,
		"app_read_mib_per_sec", report.Resources.AppReadBytesPerSecAvg / (1 << 20),
		"disk_ceiling_at_readers", report.DiskCeilingAtReaders,
		"disk_at_audience_mbps", report.DiskReadBytesPerSecAtAudience * 8 / 1e6,
		// Named for the unit it actually carries: this one is MEBIbytes, while
		// served/ceiling below are megaBITS. Mixing the two silently is how a
		// 4x read amplification hides in plain sight in a log line.
		"disk_read_mib_per_sec", report.Resources.DiskReadBytesPerSecAvg / (1 << 20),
		"media_read_ceiling_mbps", report.DiskReadBytesPerSecCeiling * 8 / 1e6,
		"slowest_disk_mbps", report.SlowestDiskBytesPerSec * 8 / 1e6,
		"disks_probed", report.DisksProbed,
		"served_mbps", report.Resources.ServedBytesPerSecAvg * 8 / 1e6,
	}
	if report.HasPerViewerCost {
		args = append(args,
			"per_viewer_cpu_pct", int(report.PerViewerCost.CPUPercentAvg),
			"per_viewer_mem_mb", report.PerViewerCost.MemUsedBytesAvg/(1<<20),
			"per_viewer_disk_read_kbps", report.PerViewerCost.DiskReadBytesPerSecAvg/(1<<10),
			"per_viewer_served_mbps", report.PerViewerCost.ServedBytesPerSecAvg*8/1e6,
		)
	}
	for _, vt := range sortedKeys(report.ViewerMix) {
		args = append(args, "mix_"+vt, report.ViewerMix[vt])
	}
	for _, vt := range sortedKeys(report.SegmentMsP95ByType) {
		args = append(args, "segment_ms_p95_"+vt, report.SegmentMsP95ByType[vt])
	}
	// A single flat-attribute line is deliberate: an operator can drive and
	// read back a whole run with curl + `docker logs`, no admin panel
	// required.
	r.logger.Info("loadtest: report", args...)

	// The climb itself, one line per audience size. The panel shows this as a
	// table; the logs keep it too, because the SHAPE of the per-viewer cost down
	// the rows is what says whether the server was still scaling when it stopped -
	// and that is the question a single headline figure can never answer.
	for _, t := range report.Tiers {
		r.logger.Info("loadtest: tier cost",
			"viewers", t.Viewers,
			"cpu_pct", int(t.CPUPercent),
			"cpu_pct_per_viewer", fmt.Sprintf("%.2f", t.CPUPercentPerViewer),
			"mem_mb", t.MemUsedBytes/(1<<20),
			"mem_mb_per_viewer", t.MemBytesPerViewer/(1<<20),
			"disk_read_mib_per_sec", t.DiskReadBytesPerSec/(1<<20),
			"served_mbps", t.ServedBytesPerSec*8/1e6,
			"samples", t.Samples,
		)
	}

	r.mu.Lock()
	r.running = false
	r.cancelFn = nil
	r.status = Status{
		Running:    false,
		Phase:      "idle",
		Viewers:    0,
		Elapsed:    report.EndedAt.Sub(report.StartedAt),
		LastReport: &report,
	}
	r.mu.Unlock()
}
