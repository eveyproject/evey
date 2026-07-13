package loadtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxRequestTimeout is the ceiling applied to the http.Client passed to New:
// a load test must never let a single stuck request hang the whole run past
// what a real player would tolerate.
const maxRequestTimeout = 30 * time.Second

// sloWindowSize is how many of the most recent segment-fetch samples (across
// every active viewer) are considered when evaluating the SLO.
const sloWindowSize = 20

// minSLOSamples is the minimum number of samples in the rolling window
// before an SLO breach is even considered, so a couple of early jittery
// fetches at 1-2 viewers can't trip the whole run.
const minSLOSamples = 5

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
	slot   int
	detail string
}

// Runner drives one load test at a time against the server's own HTTP stack.
type Runner struct {
	httpClient    *http.Client
	media         MediaLister
	activeStreams func() int
	logger        *slog.Logger

	// servedBytes is the running total of bytes every viewer has pulled this
	// run - which is exactly what the server had to push. It is the honest
	// measure of served throughput: the simulated viewers reach the server
	// over loopback, so no NIC counter would see this traffic. Reset at the
	// start of each run (only one runs at a time).
	servedBytes atomic.Int64

	// counters is where the resource sampler reads CPU/memory/disk from. nil
	// means the server's own cgroup; tests inject a modelled machine.
	counters counterSource

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
		status:        Status{Phase: "idle"},
	}
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
		"skipped_candidates", report.SkippedCandidates,
		"duration_ms", report.EndedAt.Sub(report.StartedAt).Milliseconds(),
		"cpu_pct_avg", int(report.Resources.CPUPercentAvg),
		"cpu_pct_peak", int(report.Resources.CPUPercentPeak),
		"mem_used_mb", report.Resources.MemUsedBytesAvg / (1 << 20),
		"mem_limit_mb", report.Resources.MemLimitBytes / (1 << 20),
		"disk_read_mbps", report.Resources.DiskReadBytesPerSecAvg / (1 << 20),
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

// run owns every piece of cross-viewer state (the current tier, the sample
// buffers, the SLO). Viewer goroutines only ever talk to it through events,
// so none of that state needs its own lock.
func (r *Runner) run(runCtx context.Context, cfg Config) {
	startedAt := time.Now()

	// Prefer the path real viewers take - through the reverse proxy - and fall
	// back to the loopback when it isn't reachable from in here.
	viaProxy := false
	if cfg.PublicBaseURL != "" && r.endpointReachable(runCtx, cfg.PublicBaseURL, cfg.Token) {
		cfg.BaseURL, viaProxy = cfg.PublicBaseURL, true
	}

	r.logger.Info("loadtest started",
		"scenario", cfg.Scenario,
		"mode", cfg.Mode,
		"max_viewers", cfg.MaxViewers,
		"max_duration", cfg.MaxDuration.String(),
		"endpoint", cfg.BaseURL,
		"via_proxy", viaProxy,
	)

	// Ask for far more titles than viewers. The pool multiplies them by start
	// position anyway, and a generous list is what lets an ineligible title be
	// skipped without costing a viewer.
	candidates, err := r.media.ListLoadTestCandidates(runCtx, candidatePoolSize)
	if err != nil {
		r.finish(Report{
			Scenario: cfg.Scenario, Mode: cfg.Mode, StopReason: StopError,
			StopDetail: fmt.Sprintf("listing media candidates: %v", err),
			StartedAt:  startedAt, EndedAt: time.Now(),
		})
		return
	}
	if len(candidates) == 0 {
		r.finish(Report{
			Scenario: cfg.Scenario, Mode: cfg.Mode, StopReason: StopLibraryLimit,
			StopDetail: "no media candidates available",
			StartedAt:  startedAt, EndedAt: time.Now(),
		})
		return
	}
	pool := newCandidatePool(candidates, cfg.MaxDuration)

	// Measure what this box can actually read off its storage, BEFORE putting it
	// under load. On a direct-play server this is usually the real ceiling, and
	// the kernel's own I/O counters can't see it when the library lives on a
	// network share or a 9p/virtiofs mount - so the run reads the files itself
	// and times them.
	//
	// The PRNG picks read offsets inside the media files, nothing more: no secret,
	// no token, no identifier derives from it. A cryptographic source would be
	// slower and no more random for scattering reads across a disk.
	//nolint:gosec // G404: offsets for a disk benchmark, not a security decision
	rng := rand.New(rand.NewPCG(uint64(startedAt.UnixNano()), 0))
	disk := measureDiskThroughput(runCtx, candidates, rng)
	if disk.Measured {
		for _, d := range disk.Disks {
			// The whole curve, because a single number hides the thing that
			// matters: storage that COLLAPSES past a few concurrent readers looks
			// perfectly healthy when sampled at one width.
			for _, lvl := range d.Levels {
				r.logger.Info("loadtest: disk read curve",
					"readers", lvl.Readers,
					"mib_per_sec", lvl.BytesPerSec/(1<<20),
					"worst_read_ms", lvl.WorstReadMs,
					"sample_path", d.SamplePath,
				)
			}
			if d.CollapsesAt > 0 {
				r.logger.Warn("loadtest: storage collapses under concurrency",
					"collapses_at_readers", d.CollapsesAt,
					"best_mib_per_sec", d.BytesPerSec/(1<<20),
					"best_at_readers", d.BestReaders,
					"sample_path", d.SamplePath,
				)
			}
		}
		r.logger.Info("loadtest: media read throughput (all disks at once)",
			"mbps", disk.TotalBytesPerSec*8/1e6,
			"mib_per_sec", disk.TotalBytesPerSec/(1<<20),
			"disks", len(disk.Disks),
			"collapses_at_readers", disk.CollapsesAtReaders,
		)
	}

	isFlash := cfg.Mode == ModeFlash
	isEndurance := cfg.Mode == ModeEndurance

	viewerCtx, cancelViewers := context.WithCancel(runCtx)
	defer cancelViewers()
	events := make(chan viewerEvent, 256)
	var wg sync.WaitGroup

	ramp := rampState{
		viewerTypeBySlot: map[int]viewerType{},
		cancelBySlot:     map[int]context.CancelFunc{},
		roundBySlot:      map[int]int{},
		replayedByTier:   map[int]int{},
		audienceIntact:   true,
	}
	// Zero the live audience BEFORE the first viewer spawns: spawnNext counts
	// up from here, and a leftover value from the previous run would make the
	// server look busier than it is.
	r.liveViewers.Store(0)
	r.audienceEpoch.Store(0)

	r.setStatus("ramping", ramp.tier)
	if ok, detail := r.spawnInitialViewers(isFlash, viewerCtx, cfg, pool, events, &wg, &ramp); !ok {
		cancelViewers()
		wg.Wait()
		r.finish(Report{
			Scenario: cfg.Scenario, Mode: cfg.Mode, StopReason: StopLibraryLimit,
			StopDetail: detail,
			StartedAt:  startedAt, EndedAt: time.Now(),
		})
		return
	}
	r.setStatus("ramping", ramp.tier)

	// flashWindowC fires once, 3xRampEvery after every flash viewer has
	// launched, ending the observation window. nil on every other mode, so
	// this case in the select below simply never fires.
	var flashWindowC <-chan time.Time
	if isFlash {
		flashWindow := time.NewTimer(3 * cfg.RampEvery)
		defer flashWindow.Stop()
		flashWindowC = flashWindow.C
	}
	// flash buckets outcomes by ramp slot (flash has no meaningful tier
	// ordering: every slot launches at t0). served&&!shed at window end is
	// exactly "obtained its first segment and was never shed".
	flash := flashState{served: map[int]bool{}, shed: map[int]bool{}}

	slo := newSLOTracker(cfg.SegmentSLOMs)
	samples := sampleState{
		tierSegments:    map[int][]int{},
		tierTTFF:        map[int][]int{},
		perTypeSegments: map[viewerType]map[int][]int{},
	}

	// The climb is the search for the ceiling: big strides while the machine is
	// wide open, a bisection once something gives out, and a confirmation pass
	// so a short-hold false alarm can never cap the result. Its hold length
	// changes with the phase, so it drives a timer rather than a fixed ticker.
	cl := &climb{
		target: ramp.tier, batch: fastBatch,
		fastHold: cfg.FastHold, measureHold: cfg.MeasureHold,
		// A run that never starts an encoder needs a far shorter measurement
		// window: there is no ffmpeg cold start to wait out.
		spawnsEncoders: scenarioSpawnsEncoders(cfg.Scenario),
	}
	// Tests shrink the holds; the warm-up must shrink with them, or a 50ms hold
	// with a 15s warm-up would discard every sample it ever took.
	if cfg.FastHold > 0 {
		cl.fastWarmup = cfg.FastHold / 3
	}
	if cfg.MeasureHold > 0 {
		cl.measureWarmup = cfg.MeasureHold / 3
	}
	holdTimer := time.NewTimer(cl.holdFor())
	defer holdTimer.Stop()

	// The resource sampler is what lets the run name its own bottleneck. It
	// samples the server's cgroup (ffmpeg included) on its own cadence,
	// independent of the ramp, and discards each tier's warm-up so a tier is
	// only ever judged once it has settled.
	r.servedBytes.Store(0)
	src := r.counters
	if src == nil {
		src = cgroupSource{fsRoot: cfg.fsRoot()}
	}
	sampler := newResourceSampler(src)
	res := newResourceState(cl.warmupFor(), time.Now())
	resTicker := time.NewTicker(cfg.resourceSampleEvery())
	defer resTicker.Stop()

	var stopReason StopReason
	var stopDetail string
	var sustained int
	// enduranceHold: the climb has found the ceiling and the run has parked
	// there, watching for a slow drift until MaxDuration.
	var enduranceHold bool
	// throughputCeiling is the byte rate the box could not exceed, once a tier
	// proves it has stopped gaining any.
	var throughputCeiling int64

runLoop:
	for {
		select {
		case <-runCtx.Done():
			// Report the audience that was actually streaming when the run was
			// cut short. cl.lo is the highest count PROVEN to hold, and a run
			// cancelled during its very first tier has proven nothing yet - it
			// would report zero viewers while several were happily watching.
			stopReason, stopDetail, sustained = r.cancelVerdict(runCtx, isEndurance, ramp.tier, cfg)
			break runLoop

		case <-resTicker.C:
			// Stamp the sample when the counters are READ, not when the ticker
			// fired. The run loop may have been busy draining viewer events, and
			// dating a read against an older tick would divide a larger CPU delta
			// by a smaller wall-clock delta - inflating the percentage enough to
			// fake a saturation verdict.
			now := time.Now()
			sample, ok := sampler.sample(r.servedBytes.Load(), now)
			if !ok {
				continue
			}
			if reason, detail := res.observe(ramp.tier, sample, now); reason != "" {
				// A saturated resource does not end the run: it ends this TIER.
				// The climb decides what that means - narrow the search, or
				// re-test under a longer window before believing it.
				cl.tierBroke(reason, detail)
			}

		case <-flashWindowC:
			// The observation window is over: tally who made it. A refusal
			// mid-window never broke this loop (see evAdmissionFull below),
			// so every launched viewer had its full window to prove itself.
			stopReason = StopCompleted
			sustained, stopDetail = flash.verdict(ramp.tier)
			break runLoop

		case ev := <-events:
			if reason, detail, sust, stop := r.handleViewerEvent(ev, isFlash, viewerCtx, cfg, pool, events, &wg, &ramp, &flash, slo, &samples, cl, res); stop {
				stopReason, stopDetail, sustained = reason, detail, sust
				break runLoop
			}

		case <-holdTimer.C:
			if isFlash {
				// Every flash viewer already launched at t0; nothing to do but
				// wait for flashWindowC.
				holdTimer.Reset(cl.holdFor())
				continue
			}
			if enduranceHold {
				// Parked on the plateau. Nothing new joins; the run is now
				// watching for a slow drift, and a break here IS the finding.
				if cl.broke != "" {
					stopReason, stopDetail, sustained = cl.broke, cl.detail, cl.lo
					break runLoop
				}
				holdTimer.Reset(cl.holdFor())
				continue
			}

			// A tier that pushed no more bytes than a smaller one already did has
			// found the storage ceiling: the viewers are there, the server just
			// can't feed them any faster.
			if plateaued, ceiling := res.throughputPlateaued(ramp.tier); plateaued {
				throughputCeiling = ceiling
				cl.tierBroke(StopThroughputSaturated, fmt.Sprintf("throughput flat at %d Mb/s with %d viewers: the server cannot read the media any faster", ceiling*8/1e6, ramp.tier))
			}

			outcome, reason, detail := cl.next(cfg.MaxViewers, res.tierLoad(ramp.tier))
			if outcome == climbDone {
				// Endurance doesn't stop when it finds the ceiling - finding it
				// is only the warm-up. It parks at the highest count that held
				// and stays there until MaxDuration, hunting for the slow drift
				// a short run would miss. Every verdict stays armed during the
				// hold: a breach there is exactly what this mode exists to catch.
				if isEndurance && reason == StopMaxViewers {
					if ok, d := r.resizeTo(cl.lo, viewerCtx, cfg, pool, events, &wg, &ramp); !ok {
						stopReason, stopDetail, sustained = StopLibraryLimit, d, cl.lo
						break runLoop
					}
					r.setStatus("holding", ramp.tier)
					enduranceHold = true
					holdTimer.Reset(cl.holdFor())
					continue
				}
				stopReason, stopDetail, sustained = reason, detail, cl.lo
				break runLoop
			}
			// Move the audience to the new target by ADDING or REMOVING the
			// difference. Rebuilding it wholesale every time - which is what this
			// used to do, to keep the scenario's mix exact - makes every viewer
			// restart on the same second: sixty players all burst-filling their
			// buffers at once is a thundering herd no real audience produces. The
			// latency spike condemns a tier that would have held, the bisection
			// chases the phantom downwards, the confirmation finds the tier was
			// fine after all, and the climb yo-yos.
			//
			// The one case that DOES need a rebuild is an audience with holes in
			// it: viewers the server refused leave gaps at exactly the ranks it
			// couldn't serve, so the survivors skew towards the cheap path and the
			// mix silently drifts. Rebuild then, and only then.
			if !ramp.audienceIntact && cl.phase != phaseFast {
				r.shedTo(&ramp, 0)
				ramp.audienceIntact = true
				// Every session the server was holding is now over.
				r.audienceEpoch.Add(1)
			}
			if ok, d := r.resizeTo(cl.target, viewerCtx, cfg, pool, events, &wg, &ramp); !ok {
				// The pool cannot supply another viewer. Whatever held so far is
				// the answer we have.
				stopReason, stopDetail, sustained = StopLibraryLimit, d, cl.lo
				break runLoop
			}
			res.setWarmup(cl.warmupFor())
			// This tier is judged on its own evidence, never on the previous
			// one's. Both halves matter: the latency window is cleared, and the
			// warm-up clock starts NOW rather than at the sampler's next tick -
			// otherwise requests issued under the previous (overloaded) tier are
			// still landing, and the smaller tier is condemned for them.
			slo.startTier()
			res.startTier(ramp.tier, time.Now())
			// Snapshot how many of THIS audience are replaying a title, so the
			// report can say it for the tier that ends up being the answer.
			ramp.replayedByTier[ramp.tier] = ramp.replayedInAudience()
			r.logger.Info("loadtest tier", "phase", cl.phase.String(), "viewers", ramp.tier, "lo", cl.lo, "hi", cl.hi)
			r.setStatus(cl.phase.String(), ramp.tier)
			holdTimer.Reset(cl.holdFor())
		}
	}

	r.setStatus("stopping", ramp.tier)
	cancelViewers()
	wg.Wait()

	// sampleTier indexes the per-tier sample buckets. In every mode except
	// flash, "sustained" IS the tier whose samples we want (it is the highest
	// count proven to hold). Flash has no such tier ordering - every viewer
	// launched at t0 under the single tier == the final launch count - and its
	// SustainedViewers is a served-and-not-shed subset, not a tier boundary, so
	// the percentiles must be read from the full launch tier instead: "the
	// percentiles ARE the measure" of the whole herd.
	sampleTier := sustained
	if isFlash {
		sampleTier = ramp.tier
	}

	segSamples, ttffSamples, mix, p95ByType := samples.reportFields(sampleTier, &ramp)
	perViewer, hasPerViewer := res.perViewerCost(sampleTier)
	replayed := ramp.replayedByTier[sampleTier]
	sustainedRes := res.stats(sampleTier)

	r.finish(Report{
		Scenario:                     cfg.Scenario,
		Mode:                         cfg.Mode,
		SustainedViewers:             sustained,
		StopReason:                   stopReason,
		StopDetail:                   stopDetail,
		TTFFMsP50:                    percentile(ttffSamples, 50),
		TTFFMsP95:                    percentile(ttffSamples, 95),
		SegmentMsP50:                 percentile(segSamples, 50),
		SegmentMsP95:                 percentile(segSamples, 95),
		SegmentSLOMs:                 slo.worstBudget(),
		SegmentSLOMsByType:           slo.budgets(),
		Samples:                      len(segSamples),
		Endpoint:                     cfg.BaseURL,
		ViaProxy:                     viaProxy,
		ReplayedTitleViewers:         replayed,
		DiskReadStarved:              diskReadStarved(sustainedRes),
		DiskReadBytesPerSecCeiling:   disk.TotalBytesPerSec,
		SlowestDiskBytesPerSec:       disk.slowest(),
		DisksProbed:                  len(disk.Disks),
		StorageCollapsesAtReaders:    disk.CollapsesAtReaders,
		ThroughputCeilingBytesPerSec: throughputCeiling,
		SkippedCandidates:            ramp.skippedCandidates,
		Resources:                    sustainedRes,
		ResourceBaseline:             res.baseline,
		PerViewerCost:                perViewer,
		HasPerViewerCost:             hasPerViewer,
		ViewerMix:                    mix,
		SegmentMsP95ByType:           p95ByType,
		StartedAt:                    startedAt,
		EndedAt:                      time.Now(),
	})
}

// rampState is the ramp's own bookkeeping across every spawn attempt: which
// tier it's at, how many viewers replayed a title at another position, how
// many candidates were skipped as ineligible, and each slot's assigned viewer
// type.
type rampState struct {
	tier              int
	skippedCandidates int
	// roundBySlot is which replay of its title each viewer is on (0 = the first
	// viewer on that title). Counting the replays of the LIVE audience - rather
	// than every replay the run ever spawned - is what makes the figure mean
	// something: the climb rebuilds its audience between tiers, and a running
	// total would keep counting viewers that no longer exist.
	roundBySlot map[int]int
	// replayedByTier snapshots, for each tier the run held, how many of THOSE
	// viewers were replaying a title.
	replayedByTier map[int]int

	// audienceIntact is true while the live audience is exactly ranks 1..tier -
	// which is what makes the scenario's mix (e.g. 80% transcode) actually hold.
	// A viewer the server REFUSED punches a hole in that sequence: the audience
	// is then a subset, missing precisely the ranks the server couldn't serve,
	// and the survivors skew towards the cheap path. Judging the next tier on
	// that would quietly turn a heavy run into a direct-play run and report a
	// capacity nobody asked about, so the audience is rebuilt once before the
	// next judged tier - and only then, which is what keeps the thundering-herd
	// restart out of the common case.
	audienceIntact bool
	// consecutiveSkips counts ineligible candidates since the last viewer that
	// actually started streaming. Once it reaches the number of distinct
	// titles, every title has been tried and none is mkvgo-eligible: keep
	// retrying and the run would burn the whole pool one HTTP request at a
	// time. Any successful viewer resets it.
	consecutiveSkips int
	viewerTypeBySlot map[int]viewerType

	// cancelBySlot lets the ramp take viewers AWAY, not just add them - which
	// is what makes the bisection possible at all. Each viewer owns a context
	// derived from the run's, so shedding one leaves the others untouched.
	cancelBySlot map[int]context.CancelFunc
	// nextSlot is a monotonic id: a slot number is never reused, even after
	// its viewer has been shed, so a late event from a dead viewer can never be
	// mistaken for a live one's.
	nextSlot int
}

// shedTo drops viewers until only `target` remain, youngest first. The viewers
// that stay are the oldest ones, which are also the ones furthest past their
// own cold start - so the audience that remains is the most settled available.
func (r *Runner) shedTo(ramp *rampState, target int) {
	for slot := ramp.nextSlot; slot >= 1 && ramp.tier > target; slot-- {
		cancel, live := ramp.cancelBySlot[slot]
		if !live {
			continue
		}
		cancel()
		delete(ramp.cancelBySlot, slot)
		ramp.tier--
		r.liveViewers.Add(-1)
		if ramp.viewerTypeBySlot[slot] == viewerTypeTranscode {
			r.liveTranscodeViewers.Add(-1)
		}
	}
}

// spawnNext tries to fill the next ramp slot (ramp.tier+1) with the pool's
// next (title, start position) assignment. Reusing a title is normal and
// expected once the library is exhausted - the viewer is simply placed further
// into the file - so the only way this fails is a pool with nothing left at
// all, which is a library that cannot even fill titles x positions viewers.
func (r *Runner) spawnNext(viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	// The viewer's TYPE follows its rank in the live audience (tier+1), not its
	// slot id: the scenario's mix (e.g. 50/50) must hold for the viewers that
	// are actually watching, even after the bisection has shed some.
	vt := scenarioViewerType(cfg.Scenario, ramp.tier+1)

	asg, ok := pool.take()
	if !ok {
		return false, fmt.Sprintf("media pool exhausted (%d titles x %d start positions)", len(pool.candidates), len(startPositions))
	}
	// A transcode-typed viewer replaying a title must fork the server's
	// decision hash, or it would attach to the session an earlier viewer
	// already drives instead of costing the server a new encode. The mkvgo
	// path is stateless, so a distinct start position is enough there.
	forked := asg.round > 0 && vt == viewerTypeTranscode

	ramp.nextSlot++
	slot := ramp.nextSlot
	ramp.tier++
	r.liveViewers.Add(1)
	if vt == viewerTypeTranscode {
		r.liveTranscodeViewers.Add(1)
	}
	ramp.viewerTypeBySlot[slot] = vt
	ramp.roundBySlot[slot] = asg.round

	// Its own context, so the bisection can shed THIS viewer without touching
	// any other.
	vCtx, cancel := context.WithCancel(viewerCtx)
	ramp.cancelBySlot[slot] = cancel

	wg.Add(1)
	go r.runViewer(vCtx, cfg, slot, vt, asg, forked, events, wg)
	return true, ""
}

// resizeTo brings the live audience to `target`, spawning or shedding as
// needed. It reports false only when the pool cannot supply another viewer.
func (r *Runner) resizeTo(target int, viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	if target < ramp.tier {
		r.shedTo(ramp, target)
		return true, ""
	}
	for ramp.tier < target {
		if ok, detail := r.spawnNext(viewerCtx, cfg, pool, events, wg, ramp); !ok {
			return false, detail
		}
	}
	return true, ""
}

// spawnInitialViewers launches the run's very first wave: exactly one
// viewer for every mode except flash, which launches min(MaxViewers,
// available candidates) at once (no ramp at all). It reports whether the
// run has anything to observe; on failure the caller finishes the run
// immediately with StopLibraryLimit.
func (r *Runner) spawnInitialViewers(isFlash bool, viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	// flashTarget is how many viewers the slam launches at once. Unlike the
	// climb, flash cannot discover its own size - it IS the size - so an
	// unbounded MaxViewers (the default) needs a concrete number here rather
	// than "every assignment in the pool", which would be thousands.
	flashTarget := 1
	if isFlash {
		want := cfg.MaxViewers
		if want <= 0 {
			want = defaultFlashViewers
		}
		flashTarget = min(want, pool.capacity())
	}
	for range flashTarget {
		if ok, detail := r.spawnNext(viewerCtx, cfg, pool, events, wg, ramp); !ok {
			if isFlash && ramp.tier > 0 {
				// Some viewers already launched; the flash slam is smaller
				// than intended but still valid - fall through and let it
				// run its observation window rather than discarding it.
				break
			}
			if detail == "" {
				detail = "no media candidates available"
			}
			return false, detail
		}
	}
	return true, ""
}

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

// lockTarget adopts a stream's own detected target duration - scaled down to
// the share of it within which a segment must actually arrive - as the budget
// for that viewer type, the first time a viewer of that type reports one.
func (s *sloTracker) lockTarget(vt viewerType, targetDurationMs int) {
	t := s.forType(vt)
	if !t.locked {
		t.ms = int(float64(targetDurationMs) * segmentBudgetFactor)
		t.locked = true
	}
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

// cancelVerdict decides why the run is ending when runCtx fires: endurance's
// own MaxDuration is a clean StopCompleted (it held the plateau as
// designed), while every other exit through this path - an explicit
// Cancel, another mode's MaxDuration, or the caller's own context ending -
// is StopCancelled.
func (r *Runner) cancelVerdict(runCtx context.Context, isEndurance bool, tier int, cfg Config) (StopReason, string, int) {
	if isEndurance && !r.wasCancelRequested() && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return StopCompleted, fmt.Sprintf("held %d viewers for %s", tier, cfg.MaxDuration), tier
	}
	detail := "parent context ended"
	switch {
	case r.wasCancelRequested():
		detail = "cancelled by operator"
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		detail = "max duration reached"
	}
	return StopCancelled, detail, tier
}

// handleViewerEvent processes one event a viewer goroutine sent back to the
// run loop, updating the shared ramp/flash/slo/sample state.
//
// A capacity signal (latency budget missed, server refusing new streams) does
// NOT end the run: it marks the current TIER as broken and lets the climb
// decide what that means - narrow the search around it, or re-test it under a
// longer window before believing it. Only a signal that invalidates the whole
// measurement (an expired credential, a viewer failing outright) stops
// everything.
func (r *Runner) handleViewerEvent(ev viewerEvent, isFlash bool, viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState, flash *flashState, slo *sloTracker, samples *sampleState, cl *climb, res *resourceState) (reason StopReason, detail string, sustained int, stop bool) {
	// An event from a viewer the bisection has already shed is stale: its tier
	// is gone, and folding its samples into the live one would pollute the
	// measurement with a dead viewer's latency.
	if _, live := ramp.cancelBySlot[ev.slot]; !live && ev.slot > 0 && !isFlash {
		return "", "", 0, false
	}

	switch ev.kind {
	case evTargetDuration:
		slo.lockTarget(ev.vt, ev.ms)

	case evTTFF:
		// A viewer got its first segment: the library is not a dead end.
		ramp.consecutiveSkips = 0
		samples.tierTTFF[ramp.tier] = append(samples.tierTTFF[ramp.tier], ev.ms)
		if isFlash {
			flash.served[ev.slot] = true
			break
		}
		// Startup is judged on its own. A server can be feeding its existing
		// viewers perfectly well and still take six seconds to let a new one
		// start watching - which, to the person holding the remote, IS the
		// server being full.
		if ttff := samples.tierTTFF[ramp.tier]; len(ttff) >= startupMinSamples {
			if p95 := percentile(ttff, 95); p95 > startupBudgetMs {
				cl.tierBroke(StopStartupTooSlow, fmt.Sprintf("playback takes %dms to start at %d viewers (budget %dms)", p95, ramp.tier, startupBudgetMs))
			}
		}

	case evSegment:
		// Drop everything a freshly-resized tier reports while it settles. The
		// viewers that just joined are burst-filling their buffers, and that
		// burst slows down everyone - including the viewers that were streaming
		// happily a second ago. Judging a tier on its own arrival spike is what
		// made the climb chase failures that weren't there.
		if res.settling(time.Now()) {
			return "", "", 0, false
		}
		samples.tierSegments[ramp.tier] = append(samples.tierSegments[ramp.tier], ev.ms)
		if samples.perTypeSegments[ev.vt] == nil {
			samples.perTypeSegments[ev.vt] = map[int][]int{}
		}
		samples.perTypeSegments[ev.vt][ramp.tier] = append(samples.perTypeSegments[ev.vt][ramp.tier], ev.ms)
		if breached, p95, sloMs := slo.observe(ev.vt, ev.ms); breached {
			if isFlash {
				// A flash slam is designed to overload; a global latency
				// blip doesn't end the measurement, it's one of the reasons
				// some viewers get shed.
				flash.sloBreachSeen = true
				return "", "", 0, false
			}
			// The players at this tier would be buffering. That breaks the
			// TIER, not the run.
			cl.tierBroke(StopSLOBreach, fmt.Sprintf("%s viewers: p95 segment fetch %dms exceeds their %dms budget at %d viewers", ev.vt, p95, sloMs, ramp.tier))
		}

	case evAdmissionFull:
		if isFlash {
			// The whole point of a flash slam is to see how many survive
			// it - a refusal counts one viewer as not served and the run
			// keeps observing the rest.
			flash.shed[ev.slot] = true
			flash.admissionDenied++
			return "", "", 0, false
		}
		// The server itself refused the stream: this tier is past what it
		// admits. That is a capacity answer, and the climb takes it from here.
		cl.tierBroke(StopAdmissionFull, ev.detail)
		// A refused viewer is NOT watching anything: it never got a stream.
		// Leaving it in the audience would inflate every tier above this one and
		// hand the bisection a count the server never actually served.
		r.dropViewer(ramp, ev.slot)

	case evTokenExpired:
		// Not a capacity signal at all - the run can no longer authenticate, so
		// nothing it measures from here on means anything.
		return StopTokenExpired, ev.detail, cl.lo, true

	case evViewerError:
		if isFlash {
			flash.shed[ev.slot] = true
			return "", "", 0, false
		}
		// The server stopped serving this viewer under this load. That is the
		// box giving out, not the harness misbehaving: end the TIER, let the
		// climb narrow around it, and drop the viewer (it is watching nothing).
		cl.tierBroke(StopServerFailing, ev.detail)
		r.dropViewer(ramp, ev.slot)

	case evCandidateSkipped:
		// This slot never became a real viewer: give it back and immediately
		// retry against the next assignment. A skip costs a candidate, never a
		// viewer - the ramp keeps climbing.
		ramp.skippedCandidates++
		ramp.consecutiveSkips++
		if ramp.consecutiveSkips >= len(pool.candidates) {
			return StopLibraryLimit, fmt.Sprintf("no mkvgo-eligible title in the library (%d tried, all served by the transcoder)", ramp.skippedCandidates), cl.lo, true
		}
		r.dropViewer(ramp, ev.slot)
		if ok, d := r.spawnNext(viewerCtx, cfg, pool, events, wg, ramp); !ok {
			return StopLibraryLimit, d, cl.lo, true
		}
	}
	return "", "", 0, false
}

// sortedKeys returns m's keys in a deterministic order, so the same run
// always logs its per-type attributes in the same order.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// percentile returns the nearest-rank p-th percentile of samples (p in
// [0,100]). Samples is not mutated. An empty input returns 0.
func percentile(samples []int, p int) int {
	n := len(samples)
	if n == 0 {
		return 0
	}
	sorted := make([]int, n)
	copy(sorted, samples)
	sort.Ints(sorted)
	idx := min(max((p*n+99)/100, 1), n)
	return sorted[idx-1]
}

// replayedInAudience counts how many of the viewers currently watching were
// placed on a title someone else already had (at a different position).
func (ramp *rampState) replayedInAudience() int {
	n := 0
	for slot := range ramp.cancelBySlot {
		if ramp.roundBySlot[slot] > 0 {
			n++
		}
	}
	return n
}

// diskReadStarved reports that the server pushed real traffic while the disk
// counter barely moved. There are two very different reasons for that, and the
// report must not pretend to know which:
//
//   - the operating system served the bytes from its page cache, so the storage
//     never had to work (a library small enough to sit in RAM);
//   - or the media do not live on a block device at all - a network share, or a
//     9p/virtiofs mount under a VM - in which case the kernel's per-cgroup I/O
//     accounting simply cannot see those reads. It reports zero, and zero is NOT
//     the same as "no disk activity".
//
// Either way the viewer count still stands. What must not stand is a confident
// "0.0 MB/s read from disk" next to a gigabit of traffic, which reads as a fact
// and is at best a blind spot.
func diskReadStarved(s ResourceStats) bool {
	if s.Samples == 0 || s.ServedBytesPerSecAvg <= 0 {
		return false
	}
	// A tenth of what went out the door is a generous floor: below that, the
	// block layer plainly isn't what's feeding the viewers.
	return s.DiskReadBytesPerSecAvg < s.ServedBytesPerSecAvg/10
}

// endpointReachable probes an origin before pointing hundreds of viewers at it.
//
// The reverse proxy is the honest place to measure - it is what a household
// actually connects through - but from inside the container its public hostname
// may not resolve, or no proxy may be deployed at all. Rather than fail the run
// (or, worse, report a machine that "holds zero viewers" because DNS didn't
// answer), the run checks once and quietly falls back to the loopback.
func (r *Runner) endpointReachable(ctx context.Context, baseURL, token string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, endpointProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		r.logger.Info("loadtest: reverse proxy not reachable, measuring the server directly", "url", baseURL, "err", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	// Any answer at all proves something is listening and routing to us; the
	// exact status doesn't matter (healthz may be 200, or 404 on an older build).
	return resp.StatusCode < 500
}

// dropViewer removes one viewer from the live audience: it is no longer
// watching anything (the server refused it, or it failed outright), so leaving
// it in the count would inflate every tier above.
func (r *Runner) dropViewer(ramp *rampState, slot int) {
	cancel, live := ramp.cancelBySlot[slot]
	if !live {
		return
	}
	cancel()
	delete(ramp.cancelBySlot, slot)
	ramp.tier--
	r.liveViewers.Add(-1)
	if ramp.viewerTypeBySlot[slot] == viewerTypeTranscode {
		r.liveTranscodeViewers.Add(-1)
	}
	// The audience is now a subset with a hole where this viewer was, so the
	// scenario's mix no longer holds. The next judged tier gets a clean rebuild.
	ramp.audienceIntact = false
}
