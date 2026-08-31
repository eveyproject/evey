package loadtest

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

// runner_loop.go holds the main load-test control loop: run() drives ramp/hold/
// shed across tiers, evicts the page cache to keep reads cold, and dispatches
// each viewer event to the verdict logic.

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

	// Cold-cache mode: drop the library from the page cache before anything is
	// measured - the disk probe below then reads cold storage too - and note
	// whether the eviction actually landed. A run that claims "cold cache"
	// while every byte still came from RAM would be the exact lie this mode
	// exists to kill, so an eviction that cannot work (no paths, wrong OS)
	// downgrades the run to warm and says so.
	coldCache := false
	if cfg.ColdCache {
		evicted := r.evictLibrary(candidates)
		coldCache = evicted > 0
		if coldCache {
			r.logger.Info("loadtest: cold cache on - the library is evicted from the page cache for the whole run",
				"files", evicted, "every", cfg.resourceSampleEvery().String())
		} else {
			r.logger.Warn("loadtest: cold cache requested but no file could be evicted; measuring with a warm cache")
		}
	}

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

	// Keep the cache cold for the whole run: a single up-front eviction would
	// let the first viewer re-warm it for everyone behind. The cadence rides
	// the resource sampler's - fast enough that no tier gets to build a warm
	// working set, cheap enough (one fadvise per file) to cost nothing.
	if coldCache {
		go r.keepEvicting(viewerCtx, candidates, cfg.resourceSampleEvery())
	}

	ramp := rampState{
		viewerTypeBySlot: map[int]viewerType{},
		mediaBySlot:      map[int]string{},
		cancelBySlot:     map[int]context.CancelFunc{},
		roundBySlot:      map[int]int{},
		replayedByTier:   map[int]int{},
		herdBySlot:       map[int]bool{},
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

	// Measure, once, how much of the CPU is the load generator rather than the
	// server. Without it the report bills the harness's own HTTP overhead to the
	// server and tells an operator a viewer costs several times what it does.
	// It runs on its own goroutine so the climb never waits on it.
	var splitMu sync.Mutex
	var split cpuSplit
	if cfg.ProfileHarnessCPU {
		go func() {
			// Wait for a real audience: profiling an empty server measures the
			// sampler and nothing else.
			for r.liveViewers.Load() < harnessProfileMinViewers {
				select {
				case <-runCtx.Done():
					return
				case <-time.After(200 * time.Millisecond):
				}
			}
			s, err := profileCPUSplit(runCtx, cfg.harnessProfileFor())
			if err != nil {
				// Another profile is already running (a live /debug/pprof pull, say).
				// The run is worth more than the figure: carry on without it.
				r.logger.Info("loadtest: cpu split not measured", "err", err)
				return
			}
			splitMu.Lock()
			split = s
			splitMu.Unlock()
			r.logger.Info("loadtest: cpu split",
				"harness_pct", int(s.HarnessFraction*100),
				"server_pct", int(s.serverShare()*100))
		}()
	}

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

			// A measured tier may still be short of evidence when its hold
			// expires: the regime gate (see resources.go) holds samples back
			// until the audience has actually resumed after the resize, and a
			// slow recovery can eat most of the window. One or two samples is a
			// snapshot, not a regime - so the hold stretches, within a hard
			// bound, until the tier has something to be judged ON. A tier that
			// already broke needs no more evidence.
			if cl.phase != phaseFast && cl.broke == "" && ramp.tier > 0 &&
				res.stats(ramp.tier).Samples < minJudgeSamples &&
				time.Since(res.tierStartedAt) < 2*cl.holdFor() {
				holdTimer.Reset(min(2*cfg.resourceSampleEvery(), cl.holdFor()))
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
				// Over for the HARNESS - the server still runs the encoder
				// sessions those viewers started, and they hold their
				// admission slots until an inactivity timeout far longer
				// than a bisection step. Judged against those leftovers,
				// every smaller tier is refused in turn and a healthy box
				// measures as "holds 1" (seen live 2026-08-30). Reap them
				// so this tier is admitted on its own load.
				r.reapLeftoverSessions(pool)
				// The viewers about to repopulate this audience all start at
				// once: a herd whose first fetches must not feed the startup
				// verdict (see evTTFF).
				ramp.rebuilding = true
			}
			ok, d := r.resizeTo(cl.target, viewerCtx, cfg, pool, events, &wg, &ramp)
			ramp.rebuilding = false
			if !ok {
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
			if cl.phase != phaseFast {
				// A measured hold judges this tier on its OWN fetches: the tier
				// number may have been visited on a FAST hold before, and the
				// join-burst latencies from back then must not sit inside the
				// percentiles the report quotes for it.
				samples.resetTier(ramp.tier)
			}
			res.startTier(ramp.tier, time.Now(), cl.phase != phaseFast)
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
	// The last audience's encoder sessions would otherwise grind on for
	// minutes after the verdict: leave the server as idle as it was found.
	r.reapLeftoverSessions(pool)

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

	// Charge the load generator's own CPU back to the load generator. Everything
	// the report says about CPU - the tier table, the per-viewer cost, the
	// headline figure - is the SERVER's share from here on.
	splitMu.Lock()
	cpu := split
	splitMu.Unlock()
	serverShare := cpu.serverShare()
	sustainedRes.CPUPercentAvg *= serverShare
	sustainedRes.CPUPercentPeak *= serverShare
	perViewer.CPUPercentAvg *= serverShare
	tiers := res.tierCosts(serverShare)

	r.finish(Report{
		Scenario:                   cfg.Scenario,
		Mode:                       cfg.Mode,
		SustainedViewers:           sustained,
		StopReason:                 stopReason,
		StopDetail:                 stopDetail,
		TTFFMsP50:                  percentile(ttffSamples, 50),
		TTFFMsP95:                  percentile(ttffSamples, 95),
		SegmentMsP50:               percentile(segSamples, 50),
		SegmentMsP95:               percentile(segSamples, 95),
		SegmentSLOMs:               slo.worstBudget(),
		SegmentSLOMsByType:         slo.budgets(),
		Samples:                    len(segSamples),
		Endpoint:                   cfg.BaseURL,
		ViaProxy:                   viaProxy,
		ReplayedTitleViewers:       replayed,
		DiskReadStarved:            diskReadStarved(sustainedRes),
		ColdCache:                  coldCache,
		ReadAmplification:          readAmplification(sustainedRes),
		AppReadAmplification:       appReadAmplification(sustainedRes),
		Tiers:                      tiers,
		HarnessCPUFraction:         cpu.HarnessFraction,
		HarnessCPUMeasured:         cpu.Measured,
		DiskReadBytesPerSecCeiling: disk.TotalBytesPerSec,
		DiskCeilingAtReaders:       disk.bestWidth(),
		// Read the storage curve at the width THIS audience imposes, not at the
		// width the hardware likes: it is the only one of the two figures the run
		// was ever competing against.
		DiskReadBytesPerSecAtAudience: disk.atWidth(sustained),
		SlowestDiskBytesPerSec:        disk.slowest(),
		DisksProbed:                   len(disk.Disks),
		StorageCollapsesAtReaders:     disk.CollapsesAtReaders,
		ThroughputCeilingBytesPerSec:  throughputCeiling,
		SkippedCandidates:             ramp.skippedCandidates,
		Resources:                     sustainedRes,
		ResourceBaseline:              res.baseline,
		PerViewerCost:                 perViewer,
		HasPerViewerCost:              hasPerViewer,
		ViewerMix:                     mix,
		SourceMix:                     pool.sourceMix(),
		SegmentMsP95ByType:            p95ByType,
		StartedAt:                     startedAt,
		EndedAt:                       time.Now(),
	})
}

// evictLibrary drops every candidate file from the OS page cache and reports
// how many evictions actually landed. A file with no known path, or one the
// kernel refuses, is skipped silently - partial cold is still colder than warm,
// and the caller only cares whether ANY of it worked.
func (r *Runner) evictLibrary(candidates []MediaCandidate) int {
	evicted := 0
	for _, c := range candidates {
		if c.Path == "" {
			continue
		}
		if err := r.evictFile(c.Path); err == nil {
			evicted++
		}
	}
	return evicted
}

// keepEvicting re-drops the library from the page cache on a fixed cadence
// until the run ends, so no tier gets to quietly rebuild a warm working set.
func (r *Runner) keepEvicting(ctx context.Context, candidates []MediaCandidate, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.evictLibrary(candidates)
		}
	}
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
		if isFlash {
			// The whole point of a flash slam is everyone starting at once: the
			// herd IS the measurement there.
			samples.tierTTFF[ramp.tier] = append(samples.tierTTFF[ramp.tier], ev.ms)
			flash.served[ev.slot] = true
			break
		}
		// A first-fetch timing only counts when the viewer joined an EXISTING
		// audience. After a REBUILD every viewer restarts at once and their
		// first fetches all land together - a herd no real audience produces: a
		// household's 21st viewer joins 20 ESTABLISHED ones, it does not start
		// alongside them. Judging the startup budget on the herd condemns a
		// tier whose organic joins are instant, and hands the run a ceiling
		// that is really its own restart colliding with the storage.
		if ramp.herdBySlot[ev.slot] {
			return "", "", 0, false
		}
		samples.tierTTFF[ramp.tier] = append(samples.tierTTFF[ramp.tier], ev.ms)
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
			// The slot stays counted (served-of-launched), but any encoder
			// session the server admitted before shedding it would keep grinding
			// for nobody and steal CPU from the viewers still under observation.
			r.releaseViewerMedia(pool, ramp, ev.slot)

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
			r.releaseViewerMedia(pool, ramp, ev.slot)
		// This slot never became a real viewer: give it back and immediately
		// retry against the next assignment. A skip costs a candidate, never a
		// viewer - the ramp keeps climbing. The title itself is retired: mkvgo
		// eligibility is a property of the title, not the start position, and
		// re-offering it at every rebuild would re-skip it forever.
		pool.markIneligible(ev.mediaID)
		ramp.skippedCandidates++
		ramp.consecutiveSkips++
		if ramp.consecutiveSkips >= len(pool.candidates) {
			return StopLibraryLimit, fmt.Sprintf("no mkvgo-eligible title in the library (%d tried, all served by the transcoder)", ramp.skippedCandidates), cl.lo, true
		}
		// The replacement inherits the herd flag: a retry spawned into a still
		// recovering rebuild starts alongside it, it does not "join" anything.
		wasHerd := ramp.herdBySlot[ev.slot]
		r.dropViewer(ramp, ev.slot)
		prevRebuilding := ramp.rebuilding
		ramp.rebuilding = wasHerd
		ok, d := r.spawnNext(viewerCtx, cfg, pool, events, wg, ramp)
		ramp.rebuilding = prevRebuilding
		if !ok {
			return StopLibraryLimit, d, cl.lo, true
		}
	}
	return "", "", 0, false
}
