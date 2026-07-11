package loadtest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
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
		"shared_cache_viewers", report.SharedCacheViewers,
		"skipped_candidates", report.SkippedCandidates,
		"duration_ms", report.EndedAt.Sub(report.StartedAt).Milliseconds(),
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
	r.logger.Info("loadtest started",
		"scenario", cfg.Scenario,
		"mode", cfg.Mode,
		"max_viewers", cfg.MaxViewers,
		"ramp_every", cfg.RampEvery.String(),
		"max_duration", cfg.MaxDuration.String(),
	)

	candidates, err := r.media.ListLoadTestCandidates(runCtx, cfg.MaxViewers)
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

	isFlash := cfg.Mode == ModeFlash
	isEndurance := cfg.Mode == ModeEndurance

	viewerCtx, cancelViewers := context.WithCancel(runCtx)
	defer cancelViewers()
	events := make(chan viewerEvent, 256)
	var wg sync.WaitGroup

	ramp := rampState{viewerTypeBySlot: map[int]viewerType{}}

	r.setStatus("ramping", ramp.tier)
	if ok, detail := r.spawnInitialViewers(isFlash, viewerCtx, cfg, candidates, events, &wg, &ramp); !ok {
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

	slo := sloTracker{ms: cfg.SegmentSLOMs, locked: cfg.SegmentSLOMs > 0}

	samples := sampleState{
		tierSegments:    map[int][]int{},
		tierTTFF:        map[int][]int{},
		perTypeSegments: map[viewerType]map[int][]int{},
	}

	rampTicker := time.NewTicker(cfg.RampEvery)
	defer rampTicker.Stop()

	var stopReason StopReason
	var stopDetail string
	var sustained int

runLoop:
	for {
		select {
		case <-runCtx.Done():
			stopReason, stopDetail, sustained = r.cancelVerdict(runCtx, isEndurance, ramp.tier, cfg)
			break runLoop

		case <-flashWindowC:
			// The observation window is over: tally who made it. A refusal
			// mid-window never broke this loop (see evAdmissionFull below),
			// so every launched viewer had its full window to prove itself.
			stopReason = StopCompleted
			sustained, stopDetail = flash.verdict(ramp.tier)
			break runLoop

		case ev := <-events:
			if reason, detail, sust, stop := r.handleViewerEvent(ev, isFlash, viewerCtx, cfg, candidates, events, &wg, &ramp, &flash, &slo, &samples); stop {
				stopReason, stopDetail, sustained = reason, detail, sust
				break runLoop
			}

		case <-rampTicker.C:
			if isFlash {
				// Every flash viewer already launched at t0; the ticker has
				// nothing left to do but wait for flashWindowC.
				continue
			}
			if reason, detail, sust, stop := r.advanceRamp(isEndurance, viewerCtx, cfg, candidates, events, &wg, &ramp); stop {
				stopReason, stopDetail, sustained = reason, detail, sust
				break runLoop
			}
		}
	}

	r.setStatus("stopping", ramp.tier)
	cancelViewers()
	wg.Wait()

	// sampleTier indexes the per-tier sample buckets. In every mode except
	// flash, "sustained" IS the tier whose samples we want (lower tiers are
	// still-healthy prefixes of the ramp). Flash has no such tier ordering -
	// every viewer launched at t0 under the single tier == the final launch
	// count - and its SustainedViewers is a served-and-not-shed subset, not
	// a tier boundary, so the percentiles must be read from the full launch
	// tier instead: "the percentiles ARE the measure" of the whole herd.
	sampleTier := sustained
	if isFlash {
		sampleTier = ramp.tier
	}

	segSamples, ttffSamples, mix, p95ByType := samples.reportFields(sampleTier, &ramp)

	r.finish(Report{
		Scenario:           cfg.Scenario,
		Mode:               cfg.Mode,
		SustainedViewers:   sustained,
		StopReason:         stopReason,
		StopDetail:         stopDetail,
		TTFFMsP50:          percentile(ttffSamples, 50),
		TTFFMsP95:          percentile(ttffSamples, 95),
		SegmentMsP50:       percentile(segSamples, 50),
		SegmentMsP95:       percentile(segSamples, 95),
		SegmentSLOMs:       slo.ms,
		Samples:            len(segSamples),
		SharedCacheViewers: ramp.sharedCacheViewers,
		SkippedCandidates:  ramp.skippedCandidates,
		ViewerMix:          mix,
		SegmentMsP95ByType: p95ByType,
		StartedAt:          startedAt,
		EndedAt:            time.Now(),
	})
}

// rampState is the ramp's own bookkeeping across every spawn attempt: which
// tier it's at, which candidate is next, how many viewers had to fork (share
// a candidate) or got retried after a skipped candidate, and each slot's
// assigned viewer type.
type rampState struct {
	tier               int
	nextCandidate      int
	sharedCacheViewers int
	skippedCandidates  int
	viewerTypeBySlot   map[int]viewerType
}

// spawnNext tries to fill the next ramp slot (ramp.tier+1). mkvgo-typed
// slots only ever take a fresh distinct candidate (never share); transcode-
// typed slots fall back to forking (audioOffsetMs) once distinct candidates
// run out. It returns false with an explanatory detail (naming the type
// that couldn't be filled) when the slot can't be filled at all.
func (r *Runner) spawnNext(viewerCtx context.Context, cfg Config, candidates []MediaCandidate, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	slot := ramp.tier + 1
	vt := scenarioViewerType(cfg.Scenario, slot)

	var media MediaCandidate
	forked := false
	switch {
	case ramp.nextCandidate < len(candidates):
		media = candidates[ramp.nextCandidate]
		ramp.nextCandidate++
	case vt == viewerTypeTranscode:
		media = candidates[ramp.tier%len(candidates)]
		forked = true
	case vt == viewerTypeMkvgo:
		return false, "no more mkvgo-eligible titles"
	default:
		return false, fmt.Sprintf("no more distinct media available for %s viewers", vt)
	}
	ramp.tier++
	ramp.viewerTypeBySlot[ramp.tier] = vt
	if forked {
		ramp.sharedCacheViewers++
	}
	wg.Add(1)
	go r.runViewer(viewerCtx, cfg, ramp.tier, vt, media, forked, events, wg)
	return true, ""
}

// spawnInitialViewers launches the run's very first wave: exactly one
// viewer for every mode except flash, which launches min(MaxViewers,
// available candidates) at once (no ramp at all). It reports whether the
// run has anything to observe; on failure the caller finishes the run
// immediately with StopLibraryLimit.
func (r *Runner) spawnInitialViewers(isFlash bool, viewerCtx context.Context, cfg Config, candidates []MediaCandidate, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	// flashTarget is min(MaxViewers, available candidates): the flash slam
	// spawns every one of them at once, with no ramp at all. Every other
	// mode spawns exactly the first slot here and lets the ramp ticker add
	// the rest.
	flashTarget := 1
	if isFlash {
		flashTarget = min(cfg.MaxViewers, len(candidates))
	}
	for range flashTarget {
		if ok, detail := r.spawnNext(viewerCtx, cfg, candidates, events, wg, ramp); !ok {
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

// sloTracker holds the rolling segment-fetch latency window used to evaluate
// the SLO, and the SLO target itself once it's known.
type sloTracker struct {
	ms     int
	locked bool
	ring   []int
}

// lockTarget adopts the stream's own detected target duration as the SLO
// the first time a viewer reports one, unless the operator already pinned
// an explicit SegmentSLOMs.
func (s *sloTracker) lockTarget(ms int) {
	if !s.locked {
		s.ms = ms
		s.locked = true
	}
}

// observe records one more segment-fetch sample and reports whether the
// rolling p95 over the last sloWindowSize samples just breached the SLO.
// p95 is only meaningful when breached is true.
func (s *sloTracker) observe(ms int) (breached bool, p95 int) {
	s.ring = append(s.ring, ms)
	if len(s.ring) > sloWindowSize {
		s.ring = s.ring[len(s.ring)-sloWindowSize:]
	}
	if s.ms <= 0 || len(s.ring) < minSLOSamples {
		return false, 0
	}
	p95 = percentile(s.ring, 95)
	return p95 > s.ms, p95
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

// advanceRamp handles one ramp-ticker firing for every non-flash mode
// (flash has no ticker-driven ramp; its case in run never calls this). It
// reports whether the run must stop and, if so, the verdict; endurance's
// held plateau at MaxViewers is the one case where firing again is a
// deliberate no-op (stop is false and the caller does nothing further).
func (r *Runner) advanceRamp(isEndurance bool, viewerCtx context.Context, cfg Config, candidates []MediaCandidate, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (reason StopReason, detail string, sustained int, stop bool) {
	if ramp.tier >= cfg.MaxViewers {
		if isEndurance {
			// Hold the plateau: verdicts (SLO/admission) above stay fully
			// active, but nothing new joins. runCtx.Done() (MaxDuration) is
			// what ends this mode.
			return "", "", 0, false
		}
		return StopMaxViewers, fmt.Sprintf("reached configured max of %d viewers cleanly", ramp.tier), ramp.tier, true
	}
	if ok, d := r.spawnNext(viewerCtx, cfg, candidates, events, wg, ramp); !ok {
		return StopLibraryLimit, d, ramp.tier, true
	}
	r.logger.Info("loadtest tier advanced", "viewers", ramp.tier)
	r.setStatus("ramping", ramp.tier)
	return "", "", 0, false
}

// handleViewerEvent processes one event a viewer goroutine sent back to the
// run loop, updating the shared ramp/flash/slo/sample state. It reports
// whether the run must stop and, if so, the verdict. isFlash selects
// between flash's own lenient handling (a refusal or an SLO blip sheds one
// viewer and keeps observing the rest) and every other mode's fail-fast
// handling (a refusal ends the run immediately).
func (r *Runner) handleViewerEvent(ev viewerEvent, isFlash bool, viewerCtx context.Context, cfg Config, candidates []MediaCandidate, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState, flash *flashState, slo *sloTracker, samples *sampleState) (reason StopReason, detail string, sustained int, stop bool) {
	switch ev.kind {
	case evTargetDuration:
		slo.lockTarget(ev.ms)

	case evTTFF:
		samples.tierTTFF[ramp.tier] = append(samples.tierTTFF[ramp.tier], ev.ms)
		if isFlash {
			flash.served[ev.slot] = true
		}

	case evSegment:
		samples.tierSegments[ramp.tier] = append(samples.tierSegments[ramp.tier], ev.ms)
		if samples.perTypeSegments[ev.vt] == nil {
			samples.perTypeSegments[ev.vt] = map[int][]int{}
		}
		samples.perTypeSegments[ev.vt][ramp.tier] = append(samples.perTypeSegments[ev.vt][ramp.tier], ev.ms)
		if breached, p95 := slo.observe(ev.ms); breached {
			if isFlash {
				// A flash slam is designed to overload; a global latency
				// blip doesn't end the measurement, it's one of the reasons
				// some viewers get shed.
				flash.sloBreachSeen = true
				return "", "", 0, false
			}
			return StopSLOBreach, fmt.Sprintf("p95 segment fetch %dms exceeds %dms SLO at %d viewers", p95, slo.ms, ramp.tier), max(ramp.tier-1, 0), true
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
		return StopAdmissionFull, ev.detail, max(ramp.tier-1, 0), true

	case evTokenExpired:
		return StopTokenExpired, ev.detail, max(ramp.tier-1, 0), true

	case evViewerError:
		if isFlash {
			flash.shed[ev.slot] = true
			return "", "", 0, false
		}
		return StopError, ev.detail, max(ramp.tier-1, 0), true

	case evCandidateSkipped:
		// This slot never became a real viewer: give it back and
		// immediately retry the same slot (same type) against the next
		// distinct candidate.
		ramp.skippedCandidates++
		ramp.tier--
		if ok, d := r.spawnNext(viewerCtx, cfg, candidates, events, wg, ramp); !ok {
			return StopLibraryLimit, d, ramp.tier, true
		}
		r.setStatus("ramping", ramp.tier)
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
