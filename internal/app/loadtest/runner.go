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
	kind   eventKind
	vt     viewerType
	ms     int
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
	cfg = cfg.withDefaults()

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
		"max_viewers", cfg.MaxViewers,
		"ramp_every", cfg.RampEvery.String(),
		"max_duration", cfg.MaxDuration.String(),
	)

	candidates, err := r.media.ListLoadTestCandidates(runCtx, cfg.MaxViewers)
	if err != nil {
		r.finish(Report{
			Scenario: cfg.Scenario, StopReason: StopError,
			StopDetail: fmt.Sprintf("listing media candidates: %v", err),
			StartedAt:  startedAt, EndedAt: time.Now(),
		})
		return
	}
	if len(candidates) == 0 {
		r.finish(Report{
			Scenario: cfg.Scenario, StopReason: StopLibraryLimit,
			StopDetail: "no media candidates available",
			StartedAt:  startedAt, EndedAt: time.Now(),
		})
		return
	}

	viewerCtx, cancelViewers := context.WithCancel(runCtx)
	defer cancelViewers()
	events := make(chan viewerEvent, 256)
	var wg sync.WaitGroup

	tier := 0
	nextCandidate := 0
	sharedCacheViewers := 0
	skippedCandidates := 0
	viewerTypeBySlot := map[int]viewerType{}

	// spawn tries to fill the next ramp slot (tier+1). mkvgo-typed slots
	// only ever take a fresh distinct candidate (never share); transcode-
	// typed slots fall back to forking (audioOffsetMs) once distinct
	// candidates run out. It returns false with an explanatory detail
	// (naming the type that couldn't be filled) when the slot can't be
	// filled at all.
	spawn := func() (bool, string) {
		slot := tier + 1
		vt := scenarioViewerType(cfg.Scenario, slot)

		var media MediaCandidate
		forked := false
		switch {
		case nextCandidate < len(candidates):
			media = candidates[nextCandidate]
			nextCandidate++
		case vt == viewerTypeTranscode:
			media = candidates[tier%len(candidates)]
			forked = true
		case vt == viewerTypeMkvgo:
			return false, "no more mkvgo-eligible titles"
		default:
			return false, fmt.Sprintf("no more distinct media available for %s viewers", vt)
		}
		tier++
		viewerTypeBySlot[tier] = vt
		if forked {
			sharedCacheViewers++
		}
		wg.Add(1)
		go r.runViewer(viewerCtx, cfg, tier, vt, media, forked, events, &wg)
		return true, ""
	}

	r.setStatus("ramping", tier)
	if ok, detail := spawn(); !ok {
		cancelViewers()
		wg.Wait()
		if detail == "" {
			detail = "no media candidates available"
		}
		r.finish(Report{
			Scenario: cfg.Scenario, StopReason: StopLibraryLimit,
			StopDetail: detail,
			StartedAt:  startedAt, EndedAt: time.Now(),
		})
		return
	}
	r.setStatus("ramping", tier)

	sloMs := cfg.SegmentSLOMs
	sloLocked := sloMs > 0

	var ring []int
	tierSegments := map[int][]int{}
	tierTTFF := map[int][]int{}
	perTypeSegments := map[viewerType]map[int][]int{}

	rampTicker := time.NewTicker(cfg.RampEvery)
	defer rampTicker.Stop()

	var stopReason StopReason
	var stopDetail string
	var sustained int

runLoop:
	for {
		select {
		case <-runCtx.Done():
			stopReason = StopCancelled
			switch {
			case r.wasCancelRequested():
				stopDetail = "cancelled by operator"
			case errors.Is(runCtx.Err(), context.DeadlineExceeded):
				stopDetail = "max duration reached"
			default:
				stopDetail = "parent context ended"
			}
			sustained = tier
			break runLoop

		case ev := <-events:
			switch ev.kind {
			case evTargetDuration:
				if !sloLocked {
					sloMs = ev.ms
					sloLocked = true
				}
			case evTTFF:
				tierTTFF[tier] = append(tierTTFF[tier], ev.ms)
			case evSegment:
				tierSegments[tier] = append(tierSegments[tier], ev.ms)
				if perTypeSegments[ev.vt] == nil {
					perTypeSegments[ev.vt] = map[int][]int{}
				}
				perTypeSegments[ev.vt][tier] = append(perTypeSegments[ev.vt][tier], ev.ms)
				ring = append(ring, ev.ms)
				if len(ring) > sloWindowSize {
					ring = ring[len(ring)-sloWindowSize:]
				}
				if sloMs > 0 && len(ring) >= minSLOSamples {
					if p95 := percentile(ring, 95); p95 > sloMs {
						stopReason = StopSLOBreach
						sustained = max(tier-1, 0)
						stopDetail = fmt.Sprintf("p95 segment fetch %dms exceeds %dms SLO at %d viewers", p95, sloMs, tier)
						break runLoop
					}
				}
			case evAdmissionFull:
				stopReason = StopAdmissionFull
				sustained = max(tier-1, 0)
				stopDetail = ev.detail
				break runLoop
			case evTokenExpired:
				stopReason = StopTokenExpired
				sustained = max(tier-1, 0)
				stopDetail = ev.detail
				break runLoop
			case evViewerError:
				stopReason = StopError
				sustained = max(tier-1, 0)
				stopDetail = ev.detail
				break runLoop
			case evCandidateSkipped:
				// This slot never became a real viewer: give it back and
				// immediately retry the same slot (same type) against the
				// next distinct candidate.
				skippedCandidates++
				tier--
				if ok, detail := spawn(); !ok {
					stopReason = StopLibraryLimit
					sustained = tier
					stopDetail = detail
					break runLoop
				}
				r.setStatus("ramping", tier)
			}

		case <-rampTicker.C:
			if tier >= cfg.MaxViewers {
				stopReason = StopMaxViewers
				sustained = tier
				stopDetail = fmt.Sprintf("reached configured max of %d viewers cleanly", tier)
				break runLoop
			}
			if ok, detail := spawn(); !ok {
				stopReason = StopLibraryLimit
				sustained = tier
				stopDetail = detail
				break runLoop
			}
			r.logger.Info("loadtest tier advanced", "viewers", tier)
			r.setStatus("ramping", tier)
		}
	}

	r.setStatus("stopping", tier)
	cancelViewers()
	wg.Wait()

	segSamples := tierSegments[sustained]
	var ttffSamples []int
	for t := 1; t <= sustained; t++ {
		ttffSamples = append(ttffSamples, tierTTFF[t]...)
	}

	mix := map[string]int{}
	for slot := 1; slot <= sustained; slot++ {
		if vt, ok := viewerTypeBySlot[slot]; ok {
			mix[string(vt)]++
		}
	}
	p95ByType := map[string]int{}
	for vt, byTier := range perTypeSegments {
		if samples := byTier[sustained]; len(samples) > 0 {
			p95ByType[string(vt)] = percentile(samples, 95)
		}
	}

	r.finish(Report{
		Scenario:           cfg.Scenario,
		SustainedViewers:   sustained,
		StopReason:         stopReason,
		StopDetail:         stopDetail,
		TTFFMsP50:          percentile(ttffSamples, 50),
		TTFFMsP95:          percentile(ttffSamples, 95),
		SegmentMsP50:       percentile(segSamples, 50),
		SegmentMsP95:       percentile(segSamples, 95),
		SegmentSLOMs:       sloMs,
		Samples:            len(segSamples),
		SharedCacheViewers: sharedCacheViewers,
		SkippedCandidates:  skippedCandidates,
		ViewerMix:          mix,
		SegmentMsP95ByType: p95ByType,
		StartedAt:          startedAt,
		EndedAt:            time.Now(),
	})
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
