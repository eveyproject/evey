package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- fake media pool ------------------------------------------------------

type fakeMediaLister struct{ candidates []MediaCandidate }

func (f fakeMediaLister) ListLoadTestCandidates(_ context.Context, limit int, _ uint64) ([]MediaCandidate, error) {
	if limit > 0 && limit < len(f.candidates) {
		return f.candidates[:limit], nil
	}
	return f.candidates, nil
}

func manyCandidates(n int) []MediaCandidate {
	out := make([]MediaCandidate, n)
	for i := range n {
		out[i] = MediaCandidate{ID: fmt.Sprintf("m%d", i), DurationSec: 3600}
	}
	return out
}

func zeroActive() int { return 0 }

// testLogger discards the run's logs; LT_DEBUG=1 prints them, to trace a
// climb that does not end where a test expects.
func testLogger() *slog.Logger {
	if os.Getenv("LT_DEBUG") != "" {
		return slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- fake HLS server --------------------------------------------------------
//
// Serves a 3-variant master playlist, per-variant playlists, and segments,
// close enough to real Evey HLS endpoints to exercise the runner end to end.
// Every request is recorded for assertions, and master/segment responses can
// be redirected through test-supplied hooks to inject capacity/auth/latency
// behavior.

type fetchRecord struct {
	path        string
	audioOffset string
	rawQuery    string
	byteRange   string
	at          time.Time
}

type serverState struct {
	mu sync.Mutex

	segCount     int
	targetDurRaw string
	// audioRendition: declare a demuxed audio track in the master, the way Evey's
	// direct-play path does.
	audioRendition bool
	// targetDurFn overrides targetDurRaw per request, so a test can give the
	// mkvgo path and the transcode path DIFFERENT segment lengths - which is
	// what they really have (keyframe-aligned vs a fixed 3s ladder).
	targetDurFn func(rawQuery string) string

	masterHits int
	// arrivals counts streams offered since the audience was last rebuilt, and
	// lastEpoch is the audience they belong to (see admit).
	arrivals  int
	lastEpoch int64

	// masterHook lets a test deny/redirect a master playlist request. A
	// zero status means "serve the default playlist". rawQuery lets a test
	// distinguish a forced-transcode request (vd=transcode) from a natural
	// one without adding a whole new dimension to the state.
	masterHook func(hitNum int, mediaID, rawQuery string) (status int, headers map[string]string)
	// segmentHook lets a test deny or delay a segment fetch. A zero status
	// means "200 OK", after the returned sleep (if any).
	segmentHook func(mediaID string, segIdx int) (status int, headers map[string]string, sleep time.Duration)

	// servedByFn decides whether a given resource (master/variant/segment)
	// carries X-Evey-Served-By: mkvgo. nil means "always". resource is one
	// of "master", "variant", "segment"; segIdx is -1 for master/variant.
	servedByFn func(mediaID, resource string, segIdx int) bool

	// playMethodFn is stream-info's verdict for a media and query; nil means
	// "PLAY_METHOD_TRANSCODE" (the HLS master). rawFileSize is the size of the
	// file /stream serves.
	playMethodFn func(mediaID, rawQuery string) string
	rawFileSize  int64

	fetches []fetchRecord
}

// viewersAdmitted is how many simulated viewers have started a stream: each one
// fetches the master playlist exactly once on startup. A test that models
// resource cost drives it off THIS rather than off elapsed time, so the modelled
// load stays locked to the ramp instead of racing it.
func (s *serverState) servedBy(mediaID, resource string, segIdx int) bool {
	s.mu.Lock()
	fn := s.servedByFn
	s.mu.Unlock()
	if fn == nil {
		return true
	}
	return fn(mediaID, resource, segIdx)
}

func newServerState() *serverState {
	return &serverState{segCount: 8, targetDurRaw: "0.05"}
}

// newDemuxedServerState is a server whose master carries a separate audio
// rendition - what Evey actually serves on the direct-play path.
func newDemuxedServerState() *serverState {
	st := newServerState()
	st.audioRendition = true
	return st
}

// audience is a late-bound view of how many viewers are CURRENTLY streaming.
//
// A fake server that models capacity must react to the live audience, not to a
// cumulative request count: the climb sheds viewers while bisecting, and a
// counter that only ever goes up would keep the fake server pinned at its peak
// long after the load came back down. The runner needs the server's URL and the
// server needs the runner, so the link is closed after both exist.
type audience struct {
	mu sync.Mutex
	r  *Runner
}

func (a *audience) bind(r *Runner) {
	a.mu.Lock()
	a.r = r
	a.mu.Unlock()
}

func (a *audience) size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.r == nil {
		return 0
	}
	return a.r.Status().Viewers
}

// epoch changes when the run REBUILDS its audience from scratch - the one moment
// every session the server was holding ends at once.
func (a *audience) epoch() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.r == nil {
		return 0
	}
	return a.r.audienceEpoch.Load()
}

// transcodes is how many of the live audience are on the transcode path - the
// ones that actually cost an encoder.
func (a *audience) transcodes() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.r == nil {
		return 0
	}
	return int(a.r.liveTranscodeViewers.Load())
}

// refuseBeyond models Evey's admission control: the server holds exactly
// `capacity` concurrent sessions and refuses anything past that outright.
//
// Admission is decided on ESTABLISHED sessions, in arrival order - not on how
// many viewers the runner happens to have in flight. That distinction matters:
// a batch of ten arrives near-simultaneously, and a server that refused based on
// the batch's size would turn away clients it has room for. When the bisection
// later sheds viewers, their sessions free up, which is what the resync below
// reflects.
func (s *serverState) refuseBeyond(a *audience, capacity int) {
	s.masterHook = func(_ int, _, _ string) (int, map[string]string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.admit(a, a.size(), capacity) {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
}

// admit decides one incoming stream, modelling a server that holds `capacity`
// concurrent sessions.
//
// Sessions are counted as ARRIVALS SINCE THE AUDIENCE WAS LAST REBUILT, clamped
// by how many viewers are actually alive. Both halves matter:
//
//   - counting arrivals (rather than reading the live headcount) gets the ORDER
//     right when a batch lands at once: each viewer of the batch sees a headcount
//     that already includes all its siblings, so a headcount test would refuse
//     clients the server has room for. Arrivals queue up one admission each.
//   - clamping by the live audience hands back the slots of viewers the run has
//     shed, which arrivals alone would never notice.
//
// Caller holds s.mu.
func (s *serverState) admit(a *audience, live int, capacity int) bool {
	if e := a.epoch(); e != s.lastEpoch {
		s.arrivals = 0 // the audience was rebuilt: every old session is gone
		s.lastEpoch = e
	}
	s.arrivals++
	return min(s.arrivals, live) <= capacity
}

// refuseTranscodeBeyond models a server with a limited ENCODER pool: it admits
// direct-play viewers freely and refuses transcodes past `capacity`.
func (s *serverState) refuseTranscodeBeyond(a *audience, _ string, capacity int) {
	s.masterHook = func(_ int, _, rawQuery string) (int, map[string]string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !strings.Contains(rawQuery, "vd=transcode") {
			return 0, nil // direct play costs the encoder nothing
		}
		// Encoder slots are freed by TRANSCODE viewers going away, so that is
		// what the live count must be measured against - not the headcount, which
		// direct-play viewers inflate for free.
		if !s.admit(a, a.transcodes(), capacity) {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
}

// refuseTranscodeSticky models the REAL server's encoder pool: an admitted
// transcode KEEPS its slot when its viewer goes away (the session lingers
// until an inactivity timeout far longer than a bisection step), and only a
// reap - the runner asking the server to kill its leftover sessions - frees
// it. refuseTranscodeBeyond's epoch shortcut (slots vanish with the audience)
// is exactly the optimistic shape the real server does not have.
func (s *serverState) refuseTranscodeSticky(capacity int) (reap func(mediaIDs []string)) {
	held := map[string]bool{}
	s.masterHook = func(_ int, mediaID, rawQuery string) (int, map[string]string) {
		if !strings.Contains(rawQuery, "vd=transcode") {
			return 0, nil // direct play costs the encoder nothing
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if held[mediaID] {
			return 0, nil // joins the session this title already has
		}
		if len(held) >= capacity {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		held[mediaID] = true
		return 0, nil
	}
	return func(mediaIDs []string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range mediaIDs {
			delete(held, id)
		}
	}
}

// stallBeyond models a server that keeps admitting viewers past its real
// capacity but can no longer feed them in time: segments arrive slower than
// they play, which is what a buffering player looks like.
func (s *serverState) stallBeyond(a *audience, capacity int, slow time.Duration) {
	s.segmentHook = func(_ string, _ int) (int, map[string]string, time.Duration) {
		if a.size() > capacity {
			return 0, nil, slow
		}
		return 0, nil, 0
	}
}

func (s *serverState) record(r *http.Request) {
	s.mu.Lock()
	s.fetches = append(s.fetches, fetchRecord{
		path:        r.URL.Path,
		audioOffset: r.URL.Query().Get("audioOffsetMs"),
		rawQuery:    r.URL.RawQuery,
		byteRange:   r.Header.Get("Range"),
		at:          time.Now(),
	})
	s.mu.Unlock()
}

// segIndexFromPath extracts the segment index from a recorded fetch path
// (mirrors serveSegment's own parsing), so a test can assert on the exact
// sequence of segments a viewer requested.
func segIndexFromPath(path string) (int, bool) {
	if !strings.Contains(path, "-seg") {
		return 0, false
	}
	base := path[strings.LastIndex(path, "/")+1:]
	_, after, ok := strings.CutLast(base, "seg")
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSuffix(after, ".ts"))
	if err != nil {
		return 0, false
	}
	return v, true
}

func mediaIDFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "media" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (s *serverState) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The run's eligibility check asks every candidate's master before the
		// draw: answered like a master (served-by included), but it is not a
		// viewer, so it is neither recorded nor counted as a master hit.
		if r.Header.Get(headerLoadTest) == loadTestEligibilityProbe {
			if s.servedBy(mediaIDFromPath(r.URL.Path), "master", -1) {
				w.Header().Set(headerServedBy, servedByMkvgo)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		s.record(r)
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			s.serveMaster(w, r)
		case strings.HasSuffix(r.URL.Path, ".m3u8"):
			s.serveVariant(w, r)
		case strings.HasSuffix(r.URL.Path, ".ts"):
			s.serveSegment(w, r)
		case strings.HasSuffix(r.URL.Path, "/stream-info"):
			s.serveStreamInfo(w, r)
		case strings.HasSuffix(r.URL.Path, "/stream"):
			s.serveRawFile(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func (s *serverState) serveStreamInfo(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.playMethodFn
	s.mu.Unlock()
	method := "PLAY_METHOD_TRANSCODE"
	if fn != nil {
		method = fn(strings.Split(r.URL.Path, "/")[4], r.URL.RawQuery)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"playMethod":%q}`, method)
}

// serveRawFile answers a Range read of a file of rawFileSize zero bytes.
func (s *serverState) serveRawFile(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	size := s.rawFileSize
	s.mu.Unlock()
	var from, to int64
	if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &from, &to); err != nil || from < 0 || from >= size {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	to = min(to, size-1)
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, size))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(make([]byte, to-from+1))
}

func (s *serverState) serveMaster(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.masterHits++
	hitNum := s.masterHits
	hook := s.masterHook
	s.mu.Unlock()

	mediaID := mediaIDFromPath(r.URL.Path)
	if hook != nil {
		if status, headers := hook(hitNum, mediaID, r.URL.RawQuery); status != 0 {
			for k, v := range headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(status)
			return
		}
	}

	dir := strings.TrimSuffix(r.URL.Path, "master.m3u8")
	// The variant URIs carry the master's own query, exactly like Evey's do:
	// the decision (vd/ad, codec caps) has to survive from the master down to
	// every sub-resource, or the variant would be negotiated all over again.
	suffix := ""
	if r.URL.RawQuery != "" {
		suffix = "?" + r.URL.RawQuery
	}
	bandwidths := []int{100_000, 200_000, 300_000}
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	// A DEMUXED audio rendition, exactly like Evey's direct-play master: a real
	// player pulls this playlist ALONGSIDE the video one, so a simulated viewer
	// that ignores it measures a client nobody runs.
	if s.audioRendition {
		fmt.Fprintf(&b, "#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aud\",NAME=\"VF\",DEFAULT=YES,URI=\"%sa1.m3u8%s\"\n", dir, suffix)
	}
	for i, bw := range bandwidths {
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,AUDIO=\"aud\"\n%sv%d.m3u8%s\n", bw, dir, i, suffix)
	}
	if s.servedBy(mediaID, "master", -1) {
		w.Header().Set(headerServedBy, servedByMkvgo)
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func (s *serverState) serveVariant(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	segCount := s.segCount
	targetDur := s.targetDurRaw
	if s.targetDurFn != nil {
		targetDur = s.targetDurFn(r.URL.RawQuery)
	}
	s.mu.Unlock()

	path := r.URL.Path
	mediaID := mediaIDFromPath(path)
	dir := path[:strings.LastIndex(path, "/")+1]
	variant := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".m3u8")

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%s\n", targetDur)
	for i := range segCount {
		// Each segment lasts the target duration, as a real packager writes it:
		// the SLO judges a fetch against its segment's own EXTINF.
		fmt.Fprintf(&b, "#EXTINF:%s,\n", targetDur)
		fmt.Fprintf(&b, "%s%s-seg%d.ts\n", dir, variant, i)
	}
	if s.servedBy(mediaID, "variant", -1) {
		w.Header().Set(headerServedBy, servedByMkvgo)
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func (s *serverState) serveSegment(w http.ResponseWriter, r *http.Request) {
	mediaID := mediaIDFromPath(r.URL.Path)
	base := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	segIdx := 0
	if _, after, ok := strings.CutLast(base, "seg"); ok {
		if v, err := strconv.Atoi(strings.TrimSuffix(after, ".ts")); err == nil {
			segIdx = v
		}
	}

	s.mu.Lock()
	hook := s.segmentHook
	s.mu.Unlock()

	status := http.StatusOK
	var headers map[string]string
	var sleep time.Duration
	if hook != nil {
		status, headers, sleep = hook(mediaID, segIdx)
		if status == 0 {
			status = http.StatusOK
		}
	}
	if sleep > 0 {
		time.Sleep(sleep)
	}
	if s.servedBy(mediaID, "segment", segIdx) {
		w.Header().Set(headerServedBy, servedByMkvgo)
	}
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(status)
	if status == http.StatusOK {
		_, _ = w.Write([]byte("segment-data"))
	}
}

// --- helpers ----------------------------------------------------------------

func waitForReport(t *testing.T, r *Runner, timeout time.Duration) *Report {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := r.Status()
		if !st.Running && st.LastReport != nil {
			return st.LastReport
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("loadtest did not finish within %v", timeout)
	return nil
}

// --- tests --------------------------------------------------------------

// A server that refuses the 4th stream holds 3. The climb must find that exact
// number - overshooting in batches, then bisecting back down to it - and report
// the server's own refusal as the reason.
func TestRunner_AdmissionFullStopsRampAtSustainedTier(t *testing.T) {
	st := newServerState()
	aud := &audience{}
	st.refuseBeyond(aud, 3)
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 6, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 20*time.Second)
	if report.StopReason != StopAdmissionFull {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopAdmissionFull, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3", report.SustainedViewers)
	}
	if report.Samples == 0 {
		t.Fatalf("expected segment samples from the sustained tier, got 0")
	}
}

// A server that keeps ADMITTING viewers past its capacity but can no longer
// feed them in time is just as full - the players buffer. The climb must land
// on the last count that streamed smoothly.
func TestRunner_SLOBreachStopsAtPriorTier(t *testing.T) {
	st := newServerState()
	st.targetDurRaw = "0.1" // 100ms auto SLO
	aud := &audience{}
	st.stallBeyond(aud, 3, 200*time.Millisecond) // comfortably beyond the 100ms budget
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 8, RampEvery: 700 * time.Millisecond, FastHold: 700 * time.Millisecond, MeasureHold: 1200 * time.Millisecond, StepWindows: 2, MaxDuration: 30 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopSLOBreach {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopSLOBreach, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3", report.SustainedViewers)
	}
	// The report names the tier that broke, where the cause of the stop shows.
	if report.BreakViewers <= report.SustainedViewers {
		t.Fatalf("break viewers = %d, want the lowest tier above the %d that held", report.BreakViewers, report.SustainedViewers)
	}
	// The budget is HALF a segment's own duration, stated for the reference
	// segment (6 s -> 3000ms) that every fetch is scaled to: a player that only
	// just keeps up never fills its buffer.
	if report.SegmentSLOMs != 3000 {
		t.Fatalf("segment SLO ms = %d, want 3000 (half the 6 s reference segment)", report.SegmentSLOMs)
	}
}

// THE 2026-07-16 RESIDUAL. Between measured tiers the run may REBUILD its
// audience: every viewer restarts at once and their first fetches all land
// together - a herd no real audience produces. A household's 21st viewer joins
// 20 ESTABLISHED ones; it does not start alongside them. Judging the startup
// budget on that herd condemned tier 21 on a box whose organic joins started in
// half a second, and handed the run a ceiling that was really the harness's own
// restart colliding with the storage.
//
// A first fetch only measures "startup" when the viewer actually JOINED an
// existing audience - so viewers respawned by a rebuild are marked as the herd,
// and their timings neither feed the verdict nor the report's percentiles. An
// organic newcomer's slow start remains exactly what the verdict exists to
// catch.
func TestRunner_AHerdOfRestartingViewersIsNotAStartupVerdict(t *testing.T) {
	r := New(&http.Client{}, fakeMediaLister{}, zeroActive, testLogger())
	pool := newCandidatePool(manyCandidates(3), time.Minute)
	slo := newSLOTracker(0)
	samples := newSampleState()
	flash := flashState{served: map[int]bool{}, shed: map[int]bool{}}
	ramp := rampState{
		tier:             21,
		viewerTypeBySlot: map[int]viewerType{},
		cancelBySlot:     map[int]context.CancelFunc{},
		roundBySlot:      map[int]int{},
		replayedByTier:   map[int]int{},
		herdBySlot:       map[int]bool{},
		audienceIntact:   true,
	}
	for slot := 1; slot <= 21+startupMinSamples; slot++ {
		ramp.cancelBySlot[slot] = func() {}
	}
	cl := &climb{phase: phaseBisect, target: 21}
	res := newResourceState(0, time.Now().Add(-time.Minute))
	// A measured size is judged on the window in progress.
	res.openWindow(21, cl.phase.String(), true, time.Now())
	events := make(chan viewerEvent, 8)
	var wg sync.WaitGroup

	sendTTFF := func(slot, ms int) {
		t.Helper()
		if _, _, _, stop := r.handleViewerEvent(
			viewerEvent{kind: evTTFF, vt: viewerTypeMkvgo, slot: slot, ms: ms},
			false, context.Background(), Config{}, pool, events, &wg,
			&ramp, &flash, slo, &samples, cl, res,
		); stop {
			t.Fatalf("a TTFF event must never stop the run outright")
		}
	}

	// The rebuild herd: 21 respawned viewers all report a first fetch far over
	// the startup budget - because they all started at once, not because the
	// server is slow to admit anyone.
	for slot := 1; slot <= 21; slot++ {
		ramp.herdBySlot[slot] = true
		sendTTFF(slot, startupBudgetMs*2)
	}
	if cl.broke != "" {
		t.Fatalf("the rebuild herd's TTFF became a %s verdict: a household's viewers never all start at once", cl.broke)
	}
	if got := len(samples.tierTTFF[21]); got != 0 {
		t.Fatalf("%d herd TTFF samples recorded: they would poison the report's startup percentiles", got)
	}

	// Organic newcomers - viewers actually joining that audience - waiting this
	// long IS the startup verdict.
	for slot := 22; slot <= 21+startupMinSamples; slot++ {
		sendTTFF(slot, startupBudgetMs*2)
	}
	if cl.broke != StopStartupTooSlow {
		t.Fatalf("broke = %q, want %s: a slow organic join is the real thing", cl.broke, StopStartupTooSlow)
	}
}

// Cold-cache mode exists because a dev box lies: when the whole library sits
// in the page cache, the disk never works and the viewer count comes out
// several times too high (the report itself warns about it). With ColdCache
// on, the run keeps evicting the library's files from the page cache - once
// before the disk probe, then continuously - so the server's reads hit the
// actual storage, the way they would when the library dwarfs the RAM.
func TestRunner_ColdCacheKeepsEvictingTheLibrary(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	candidates := manyCandidates(3)
	for i := range candidates {
		candidates[i].Path = fmt.Sprintf("/library/m%d.mkv", i)
	}

	var mu sync.Mutex
	evicted := map[string]int{}
	r := New(&http.Client{}, fakeMediaLister{candidates: candidates}, zeroActive, testLogger())
	r.evictFile = func(path string) error {
		mu.Lock()
		evicted[path]++
		mu.Unlock()
		return nil
	}
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, ColdCache: true,
		MaxViewers: 3, RampEvery: 100 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2,
		MaxDuration: 10 * time.Second, ResourceSampleEvery: 50 * time.Millisecond,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 10*time.Second)

	if !report.ColdCache {
		t.Fatalf("the report must say the run measured a cold cache: the two numbers are not comparable")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, c := range candidates {
		if evicted[c.Path] < 2 {
			t.Fatalf("%s evicted %d time(s), want at least 2 (once up front, then continuously): a single eviction lets the first viewer re-warm the cache for everyone else", c.Path, evicted[c.Path])
		}
	}
}

// And without the flag, the run must not touch the cache at all: measuring the
// warm-cache figure is the default, and it is a valid figure - it is the report
// that says which one was measured.
func TestRunner_WarmCacheRunNeverEvicts(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	candidates := manyCandidates(2)
	for i := range candidates {
		candidates[i].Path = fmt.Sprintf("/library/m%d.mkv", i)
	}
	evictions := 0
	r := New(&http.Client{}, fakeMediaLister{candidates: candidates}, zeroActive, testLogger())
	r.evictFile = func(string) error { evictions++; return nil }
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 2, RampEvery: 100 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 5*time.Second)
	if report.ColdCache {
		t.Fatalf("report claims cold cache on a default run")
	}
	if evictions != 0 {
		t.Fatalf("%d evictions on a run that never asked for them", evictions)
	}
}

// A library smaller than MaxViewers is not a capacity verdict and must never
// end the ramp: the pool replays each title at a distinct start position. Only
// the SERVER (SLO, admission) or the configured ceiling may stop a run.
func TestRunner_ShortLibraryDoesNotStopTheRamp(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 5, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 5*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s, want %s (detail=%q): 2 titles must still carry 5 viewers", report.StopReason, StopMaxViewers, report.StopDetail)
	}
	if report.SustainedViewers != 5 {
		t.Fatalf("sustained viewers = %d, want 5", report.SustainedViewers)
	}
	// Viewers 3..5 replay titles the first two already hold, at another offset.
	if report.ReplayedTitleViewers != 3 {
		t.Fatalf("replayed viewers = %d, want 3 (2 distinct titles, 5 viewers)", report.ReplayedTitleViewers)
	}
}

// The pool exhausts every distinct title before replaying any of them: titles
// are spread before they are stacked, so a run only ever reuses a file once it
// has nothing fresh left.
func TestPool_SpreadsTitlesBeforeStackingThem(t *testing.T) {
	pool := newCandidatePool(manyCandidates(3), time.Minute)

	seen := make([]string, 0, 6)
	for range 6 {
		asg, ok := pool.take()
		if !ok {
			t.Fatalf("pool exhausted early after %d takes", len(seen))
		}
		seen = append(seen, asg.media.ID)
		if len(seen) <= 3 && asg.startFrac != 0 {
			t.Fatalf("take %d: startFrac = %v, want 0 (a fresh title always starts at the beginning)", len(seen), asg.startFrac)
		}
		if len(seen) > 3 && asg.startFrac <= 0 {
			t.Fatalf("take %d (%s): a replayed title must start further in, got startFrac=%v", len(seen), asg.media.ID, asg.startFrac)
		}
	}
	// First three takes are the three distinct titles, in order; the next three
	// replay them.
	if seen[0] == seen[1] || seen[1] == seen[2] || seen[0] == seen[2] {
		t.Fatalf("first three takes reused a title before exhausting the library: %v", seen)
	}
}

// A title shorter than the run has no runway: a viewer placed anywhere but the
// start would wrap around and re-read bytes another viewer already pulled
// through the page cache.
func TestPool_ShortTitleHasNoUsableRunway(t *testing.T) {
	if got := usableFraction(600, 30*time.Minute); got != 0 {
		t.Fatalf("usableFraction(10min title, 30min run) = %v, want 0", got)
	}
	// A 2h film during a 15min run is usable up to ~87%.
	got := usableFraction(7200, 15*time.Minute)
	if got < 0.86 || got > 0.88 {
		t.Fatalf("usableFraction(2h title, 15min run) = %v, want ~0.875", got)
	}
}

func TestRunner_TranscodeScenarioForksBeyondLibrary(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioTranscode,
		MaxViewers: 4, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 3 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 3*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopMaxViewers, report.StopDetail)
	}
	if report.SustainedViewers != 4 {
		t.Fatalf("sustained viewers = %d, want 4", report.SustainedViewers)
	}
	if report.ReplayedTitleViewers != 2 {
		t.Fatalf("shared cache viewers = %d, want 2", report.ReplayedTitleViewers)
	}

	st.mu.Lock()
	forked := 0
	for _, f := range st.fetches {
		if strings.HasSuffix(f.path, "master.m3u8") {
			if !strings.Contains(f.rawQuery, "vd=transcode") {
				t.Errorf("master fetch %q missing forced vd=transcode", f.path)
			}
			if f.audioOffset != "" {
				forked++
			}
		}
	}
	st.mu.Unlock()
	// Two of four viewers replay a title; the grid's newcomers replay too, so
	// at least two masters carry the fork.
	if forked < 2 {
		t.Fatalf("master fetches carrying audioOffsetMs = %d, want at least 2", forked)
	}
}

func TestRunner_CancelStopsAndReportsPartial(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(20)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 20, RampEvery: 60 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 10 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for r.Status().Viewers < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("ramp never reached 2 viewers")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.Cancel()

	report := waitForReport(t, r, 3*time.Second)
	if report.StopReason != StopCancelled {
		t.Fatalf("stop reason = %s, want %s", report.StopReason, StopCancelled)
	}
	if report.SustainedViewers < 1 {
		t.Fatalf("sustained viewers = %d, want >= 1", report.SustainedViewers)
	}
	if !strings.Contains(report.StopDetail, "operator") {
		t.Fatalf("stop detail = %q, want it to mention operator cancellation", report.StopDetail)
	}
}

func TestRunner_TokenExpiredStopsImmediately(t *testing.T) {
	st := newServerState()
	st.masterHook = func(_ int, _, _ string) (int, map[string]string) {
		return http.StatusUnauthorized, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(3)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "expired", Scenario: ScenarioNatural,
		MaxViewers: 3, RampEvery: 50 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 2 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 2*time.Second)
	if report.StopReason != StopTokenExpired {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopTokenExpired, report.StopDetail)
	}
	if report.SustainedViewers != 0 {
		t.Fatalf("sustained viewers = %d, want 0", report.SustainedViewers)
	}
}

func TestRunner_StartRefusesWhenStreamsActive(t *testing.T) {
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(3)}, func() int { return 2 }, testLogger())
	err := r.Start(context.Background(), Config{BaseURL: "http://127.0.0.1:1", Token: "t"})
	if !errors.Is(err, ErrStreamsActive) {
		t.Fatalf("Start error = %v, want ErrStreamsActive", err)
	}
	if r.Status().Running {
		t.Fatalf("must not start while streams are active")
	}
}

func TestRunner_StartRefusesWhenAlreadyRunning(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(5)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 5, RampEvery: 500 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer r.Cancel()

	if err := r.Start(context.Background(), cfg); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start error = %v, want ErrAlreadyRunning", err)
	}
}

func TestRunner_PacesSegmentFetchesInRealTime(t *testing.T) {
	st := newServerState()
	st.targetDurRaw = "0.05" // 50ms
	st.segCount = 20
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(1)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 1, RampEvery: 5 * time.Second, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 2 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(600 * time.Millisecond)
	r.Cancel()
	waitForReport(t, r, 2*time.Second)

	st.mu.Lock()
	var segTimes []time.Time
	for _, f := range st.fetches {
		if strings.Contains(f.path, "-seg") {
			segTimes = append(segTimes, f.at)
		}
	}
	st.mu.Unlock()

	if len(segTimes) < 6 {
		t.Fatalf("not enough segment fetches recorded: %d", len(segTimes))
	}
	// After the initial burst-of-3 (fetched back to back), the viewer paces
	// fetches at ~50ms. Assert the AGGREGATE pace, not every individual gap: a
	// real-time pacer whose goroutine is briefly starved (e.g. under the full
	// -race suite) catches up by bunching two fetches, which is scheduling
	// jitter, not a pacing bug. A genuinely unpaced runner still fails the
	// average (its gaps collapse toward zero).
	if len(segTimes) <= burstSegments+1 {
		t.Fatalf("not enough post-burst segment fetches: %d", len(segTimes))
	}
	span := segTimes[len(segTimes)-1].Sub(segTimes[burstSegments])
	gaps := len(segTimes) - 1 - burstSegments
	if avg := span / time.Duration(gaps); avg < 35*time.Millisecond {
		t.Fatalf("segment fetches not paced in real time: avg gap=%v over %d gaps, want ~50ms", avg, gaps)
	}
}

// Each path is judged against ITS OWN segment length. mkvgo packages on
// keyframe boundaries (long segments), the ffmpeg ladder emits short ones; a
// single budget locked from whichever stream started first would judge the
// transcode path against mkvgo's far looser yardstick, and a real stall would
// go unreported. In a mixed scenario slot 1 is always mkvgo, so that is exactly
// what used to happen.
func TestRunner_SLOBudgetIsPerViewerType(t *testing.T) {
	st := newServerState()
	st.segCount = 40
	// mkvgo: 1000ms segments (a generous budget). transcode: 50ms segments.
	st.targetDurFn = func(rawQuery string) string {
		if strings.Contains(rawQuery, "vd=transcode") {
			return "0.05"
		}
		return "1.0"
	}
	// Every segment takes 150ms to arrive: fine against mkvgo's 1000ms budget,
	// a stall against the transcode path's 50ms one.
	st.segmentHook = func(_ string, _ int) (int, map[string]string, time.Duration) {
		return 0, nil, 150 * time.Millisecond
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioBalanced,
		MaxViewers: 6, RampEvery: 400 * time.Millisecond, FastHold: time.Second, MeasureHold: 2 * time.Second, StepWindows: 2, MaxDuration: 30 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopSLOBreach {
		t.Fatalf("stop reason = %s, want %s (detail=%q): the transcode path stalled against its own budget", report.StopReason, StopSLOBreach, report.StopDetail)
	}
	if !strings.Contains(report.StopDetail, string(viewerTypeTranscode)) {
		t.Fatalf("stop detail = %q, want it to name the transcode viewers as the ones that stalled", report.StopDetail)
	}
	// Each path is judged against its OWN segment length (each fetch scaled to
	// the 6 s reference), so both state the same bar: half the reference.
	if got := report.SegmentSLOMsByType[string(viewerTypeTranscode)]; got != 3000 {
		t.Fatalf("transcode budget = %dms, want 3000ms (half the 6 s reference)", got)
	}
	if got := report.SegmentSLOMsByType[string(viewerTypeMkvgo)]; got != 3000 {
		t.Fatalf("mkvgo budget = %dms, want 3000ms (half the 6 s reference)", got)
	}
}

// --- mkvgo scenario -------------------------------------------------------

// An ineligible title must cost a candidate, never a viewer: the ramp retries
// the slot against the next assignment and keeps climbing. A library smaller
// than MaxViewers is NOT a capacity verdict - the pool reuses each eligible
// title at a different start position instead of giving up.
func TestRunner_MkvgoSkipsIneligibleCandidateAndKeepsClimbing(t *testing.T) {
	st := newServerState()
	st.servedByFn = func(mediaID, resource string, _ int) bool {
		if resource == "master" && mediaID == "m1" {
			return false // m1 isn't actually mkvgo-served, sandwiched between two that are.
		}
		return true
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	// Only 2 of the 3 titles are mkvgo-eligible, yet 5 viewers are asked for:
	// the pool must reach 5 by replaying those 2 titles at distinct positions.
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(3)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 5, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 5*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s, want %s (detail=%q): a short library must not end the run", report.StopReason, StopMaxViewers, report.StopDetail)
	}
	if report.SustainedViewers != 5 {
		t.Fatalf("sustained viewers = %d, want 5", report.SustainedViewers)
	}
	if report.SkippedCandidates < 1 {
		t.Fatalf("skipped candidates = %d, want >= 1 (m1 is never mkvgo-served)", report.SkippedCandidates)
	}
}

// Two viewers sharing a title must not read the same bytes: the second one
// starts further into the file. Identical start offsets would let the OS page
// cache serve the second viewer for free and flatter the I/O measurement.
func TestRunner_ReusedTitleStartsAtADistinctPosition(t *testing.T) {
	st := newServerState()
	st.segCount = 100
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	// One single title, three viewers: they can only differ by start position.
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(1)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 3, RampEvery: 60 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 5*time.Second)
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3 (one title, three start positions)", report.SustainedViewers)
	}

	// Three mkvgo viewers on one title issue identical URLs, so they can only be
	// told apart by WHICH segments they asked for. If they all started at 0 they
	// would only ever touch the first handful of a 100-segment file (a burst of
	// 3 plus a few paced fetches). A segment fetched deep into the file proves a
	// viewer was placed there instead.
	st.mu.Lock()
	defer st.mu.Unlock()
	deepest := -1
	for _, f := range st.fetches {
		if idx, ok := segIndexFromPath(f.path); ok && idx > deepest {
			deepest = idx
		}
	}
	if deepest < 20 {
		t.Fatalf("deepest segment fetched = %d of 100: every viewer read from the start of the file, so reused titles are re-reading the same bytes", deepest)
	}
}

func TestRunner_MkvgoFirstSegmentWithoutHeaderCountsAsFailure(t *testing.T) {
	st := newServerState()
	st.segCount = 6
	st.servedByFn = func(_, resource string, segIdx int) bool {
		// Master is genuinely mkvgo-served (eligible), but the very first
		// segment silently isn't - the defense check must catch it.
		return resource != "segment" || segIdx != 0
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(1)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 1, RampEvery: 5 * time.Second, FastHold: 600 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Long enough for the tier to leave its warm-up (samples taken while it
	// settles are discarded) and stream a few real segments.
	time.Sleep(1500 * time.Millisecond)
	r.Cancel()
	report := waitForReport(t, r, 2*time.Second)

	if report.TTFFMsP50 != 0 || report.TTFFMsP95 != 0 {
		t.Fatalf("TTFF = (%d,%d), want (0,0): the first segment lacked the mkvgo header and must not count as a TTFF sample", report.TTFFMsP50, report.TTFFMsP95)
	}
	if report.Samples == 0 {
		t.Fatalf("expected later segments (which do carry the header) to still be counted")
	}
}

func TestRunner_LogsFinalReport(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, logger)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 2, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 3 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForReport(t, r, 3*time.Second)

	var found map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		if entry["msg"] == "loadtest: report" {
			found = entry
		}
	}
	if found == nil {
		t.Fatalf("no %q log line found in:\n%s", "loadtest: report", buf.String())
	}
	for _, key := range []string{
		"scenario", "sustained_viewers", "stop_reason", "stop_detail",
		"ttff_ms_p50", "ttff_ms_p95", "segment_ms_p50", "segment_ms_p95",
		"segment_slo_ms", "samples", "replayed_title_viewers", "skipped_candidates", "duration_ms",
	} {
		if _, ok := found[key]; !ok {
			t.Errorf("report log missing key %q, got %v", key, found)
		}
	}
}

// --- mix scenarios ---------------------------------------------------------

func countMasterHitsByType(st *serverState) (mkvgo, transcode int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, f := range st.fetches {
		if !strings.HasSuffix(f.path, "master.m3u8") {
			continue
		}
		if strings.Contains(f.rawQuery, "vd=transcode") {
			transcode++
		} else {
			mkvgo++
		}
	}
	return mkvgo, transcode
}

func TestRunner_BalancedMixAlternatesTypes(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(10)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioBalanced,
		MaxViewers: 6, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 4 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 4*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopMaxViewers, report.StopDetail)
	}
	if report.SustainedViewers != 6 {
		t.Fatalf("sustained viewers = %d, want 6", report.SustainedViewers)
	}
	if report.ViewerMix["mkvgo"] != 3 || report.ViewerMix["transcode"] != 3 {
		t.Fatalf("viewer mix = %v, want mkvgo=3 transcode=3", report.ViewerMix)
	}

	// Each new grid window replaces a few viewers by newcomers of the same
	// rank, so both types keep fetching masters - and only ever as many
	// distinct ranks as the mix gives each.
	mkvgoHits, transcodeHits := countMasterHitsByType(st)
	if mkvgoHits < 3 || transcodeHits < 3 {
		t.Fatalf("master hits by type = (mkvgo=%d, transcode=%d), want at least (3,3)", mkvgoHits, transcodeHits)
	}
}

func TestRunner_LightMixIsMostlyMkvgo(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(10)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioLight,
		MaxViewers: 5, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 4 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 4*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopMaxViewers, report.StopDetail)
	}
	if report.SustainedViewers != 5 {
		t.Fatalf("sustained viewers = %d, want 5", report.SustainedViewers)
	}
	if report.ViewerMix["mkvgo"] != 4 || report.ViewerMix["transcode"] != 1 {
		t.Fatalf("viewer mix = %v, want mkvgo=4 transcode=1", report.ViewerMix)
	}
}

// The scarce resource on a real box is the ENCODER, not the headcount:
// direct-play viewers are nearly free. A server with room for 2 transcodes must
// therefore be found to hold 3 viewers in the heavy mix (1 direct-play + 2
// transcodes), and the report must show that composition.
func TestRunner_HeavyMixAdmissionFullOnTranscodePool(t *testing.T) {
	st := newServerState()
	aud := &audience{}
	st.refuseTranscodeBeyond(aud, ScenarioHeavy, 2)
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(10)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioHeavy,
		MaxViewers: 8, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// heavy pattern is [mkvgo, transcode, transcode, transcode, transcode]:
	// rank1=mkvgo, rank2+rank3=the 2 encoder slots, rank4 would need a 3rd.
	report := waitForReport(t, r, 20*time.Second)
	if report.StopReason != StopAdmissionFull {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopAdmissionFull, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3", report.SustainedViewers)
	}
	if report.ViewerMix["mkvgo"] != 1 || report.ViewerMix["transcode"] != 2 {
		t.Fatalf("viewer mix = %v, want mkvgo=1 transcode=2", report.ViewerMix)
	}
}

// Shedding viewers WITHOUT a rebuild (an SLO breach leaves no holes: the
// bisection just shrinks the audience) must also reap - the shed viewers'
// sessions keep encoding for minutes and their CPU is billed to the smaller
// tier being judged. But only the media NO live viewer holds may be reaped:
// RemoveSession kills media-wide, and a survivor sharing a title with a shed
// viewer must keep its stream. So a shrink-time reap is always PARTIAL
// (candidates minus live media) while the end-of-run reap stays full.
func TestRunner_ShrinkWithoutRebuildReapsIdleMediaOnly(t *testing.T) {
	st := newServerState()
	st.targetDurRaw = "0.1" // 100ms auto SLO
	aud := &audience{}
	st.stallBeyond(aud, 3, 200*time.Millisecond)
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	candidates := manyCandidates(8)
	var mu sync.Mutex
	var partialReaps, fullReaps int
	r := New(&http.Client{}, fakeMediaLister{candidates: candidates}, zeroActive, testLogger())
	r.WithSessionReaper(func(mediaIDs []string) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case len(mediaIDs) == len(candidates):
			fullReaps++
		case len(mediaIDs) > 0:
			partialReaps++
		}
	})
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 8, RampEvery: 700 * time.Millisecond, FastHold: 700 * time.Millisecond, MeasureHold: 1200 * time.Millisecond, StepWindows: 2, MaxDuration: 30 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason != StopSLOBreach {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopSLOBreach, report.StopDetail)
	}
	mu.Lock()
	defer mu.Unlock()
	if partialReaps == 0 {
		t.Fatalf("no shrink-time reap: the shed viewers' sessions were left encoding over the smaller tier being judged")
	}
	if fullReaps == 0 {
		t.Fatalf("expected the end-of-run reap of every candidate")
	}
}

// The bisection must never judge a tier against the PREVIOUS tier's leftover
// encoder sessions. On the real server an admitted transcode outlives its
// viewer by minutes, so after a broken tier the admission pools are still
// full when the smaller tier spawns - and every tier down to 1 is refused in
// turn (measured live 2026-08-30: a box calibrated for 25 streams reported
// "holds 1"). The runner reaps its own leftovers when it rebuilds the
// audience, so each tier is admitted on its own load.
func TestRunner_BisectionReapsLeftoverSessionsBetweenTiers(t *testing.T) {
	st := newServerState()
	reap := st.refuseTranscodeSticky(2)
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(20)}, zeroActive, testLogger()).
		WithSessionReaper(reap)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioBalanced,
		MaxViewers: 8, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 20*time.Second)
	if report.StopReason != StopAdmissionFull {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopAdmissionFull, report.StopDetail)
	}
	// balanced is [mkvgo, transcode]: ranks 2 and 4 take the two encoder
	// slots, rank 6 would need a third - the true ceiling is 5 viewers.
	if report.SustainedViewers != 5 {
		t.Fatalf("sustained viewers = %d, want 5 (leftover sessions from a broken tier must not refuse the next one); stop %s %q, break %s %q",
			report.SustainedViewers, report.StopReason, report.StopDetail, report.BreakReason, report.BreakDetail)
	}
}

// A direct-play user has a capable client by definition. If mkvgo viewers
// negotiated with the default web profile (AAC-only audio), every AC3/DTS
// title would need an audio transcode and the packager would never be
// eligible: the scenario would measure the library's audio codecs instead of
// the packager. The capability params are the viewer's identity card.
func TestBuildMasterURL_MkvgoViewerAdvertisesACapableClient(t *testing.T) {
	u := buildMasterURL(Config{BaseURL: "http://x"}, viewerTypeMkvgo, MediaCandidate{ID: "m1"}, false, 0)
	for _, want := range []string{"videoCodecs=", "audioCodecs=", "hdr=1", "ac3"} {
		if !strings.Contains(u, want) {
			t.Errorf("mkvgo master URL missing %q: %s", want, u)
		}
	}
	if strings.Contains(u, "vd=transcode") {
		t.Errorf("mkvgo viewer must not force the transcode path: %s", u)
	}
}

// The "mkvgo+N" family pins N transcode viewers at the BOTTOM of the ramp -
// the household's fixed tax - then ramps pure mkvgo on top. Unlike the ratio
// mixes, the transcode share must not grow with the ramp.
func TestScenarioViewerType_PinnedFamily(t *testing.T) {
	for slot, want := range map[int]viewerType{
		1: viewerTypeTranscode,
		2: viewerTypeTranscode,
		3: viewerTypeMkvgo,
		9: viewerTypeMkvgo,
	} {
		if got := scenarioViewerType("mkvgo+2", slot); got != want {
			t.Errorf("mkvgo+2 slot %d = %s, want %s", slot, got, want)
		}
	}
	if got := scenarioViewerType("mkvgo+1", 1); got != viewerTypeTranscode {
		t.Errorf("mkvgo+1 slot 1 = %s, want transcode", got)
	}
	if got := scenarioViewerType("mkvgo+1", 2); got != viewerTypeMkvgo {
		t.Errorf("mkvgo+1 slot 2 = %s, want mkvgo", got)
	}
	// Degenerate spellings fall back to natural, like any unknown scenario.
	for _, bad := range []string{"mkvgo+0", "mkvgo+", "mkvgo+x", "mkvgo-1"} {
		if got := scenarioViewerType(bad, 1); got != viewerTypeNatural {
			t.Errorf("%q slot 1 = %s, want natural fallback", bad, got)
		}
	}
}

// End to end: one pinned transcode then mkvgo-only, mix reported accordingly.
func TestRunner_PinnedScenario_OneTranscodeThenMkvgo(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: "mkvgo+1",
		MaxViewers: 4, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 3 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 3*time.Second)
	if report.StopReason != StopMaxViewers {
		t.Fatalf("stop reason = %s (detail=%q)", report.StopReason, report.StopDetail)
	}
	if report.ViewerMix["transcode"] != 1 || report.ViewerMix["mkvgo"] != 3 {
		t.Fatalf("viewer mix = %v, want transcode:1 mkvgo:3", report.ViewerMix)
	}
}

// --- Mode: withDefaults ------------------------------------------------

func TestConfig_WithDefaults_UnknownModeErrors(t *testing.T) {
	_, err := Config{Mode: "bogus"}.withDefaults()
	if !errors.Is(err, ErrUnknownMode) {
		t.Fatalf("withDefaults error = %v, want ErrUnknownMode", err)
	}
}

func TestConfig_WithDefaults_EnduranceDefaultsToLongerMaxDuration(t *testing.T) {
	cfg, err := Config{Mode: ModeEndurance}.withDefaults()
	if err != nil {
		t.Fatalf("withDefaults: %v", err)
	}
	if cfg.MaxDuration != DefaultEnduranceMaxDuration {
		t.Fatalf("max duration = %v, want %v", cfg.MaxDuration, DefaultEnduranceMaxDuration)
	}

	cfg2, err := Config{Mode: ModeEndurance, MaxDuration: 90 * time.Second}.withDefaults()
	if err != nil {
		t.Fatalf("withDefaults: %v", err)
	}
	if cfg2.MaxDuration != 90*time.Second {
		t.Fatalf("explicit max duration overridden: got %v, want 90s", cfg2.MaxDuration)
	}

	cfg3, err := Config{}.withDefaults()
	if err != nil {
		t.Fatalf("withDefaults: %v", err)
	}
	if cfg3.MaxDuration != DefaultMaxDuration {
		t.Fatalf("ramp mode default max duration = %v, want %v", cfg3.MaxDuration, DefaultMaxDuration)
	}
}

func TestRunner_StartRefusesUnknownMode(t *testing.T) {
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(3)}, zeroActive, testLogger())
	err := r.Start(context.Background(), Config{BaseURL: "http://127.0.0.1:1", Token: "t", Mode: "bogus"})
	if !errors.Is(err, ErrUnknownMode) {
		t.Fatalf("Start error = %v, want ErrUnknownMode", err)
	}
	if r.Status().Running {
		t.Fatalf("must not start with an unknown mode")
	}
}

// --- Mode: flash ------------------------------------------------------

func TestRunner_FlashDoesNotStopOnAdmissionRefusalsAndCountsSurvivors(t *testing.T) {
	st := newServerState()
	st.masterHook = func(_ int, mediaID, _ string) (int, map[string]string) {
		if mediaID == "m1" || mediaID == "m3" {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(5)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeFlash,
		MaxViewers: 5, RampEvery: 100 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	start := time.Now()
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 5*time.Second)
	elapsed := time.Since(start)

	if report.StopReason != StopCompleted {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopCompleted, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3 (detail=%q)", report.SustainedViewers, report.StopDetail)
	}
	if !strings.Contains(report.StopDetail, "5") {
		t.Fatalf("stop detail = %q, want it to mention the 5 launched viewers", report.StopDetail)
	}
	// The run must have observed the full 3xRampEvery window (300ms), not
	// stopped the instant the first refusal arrived - proof that admission
	// refusal doesn't abort a flash run.
	if elapsed < 250*time.Millisecond {
		t.Fatalf("run ended after only %v, want it to have run the full observation window (~300ms)", elapsed)
	}
	if report.Mode != ModeFlash {
		t.Fatalf("report mode = %q, want %q", report.Mode, ModeFlash)
	}
}

func TestRunner_FlashLaunchesAllViewersInSameWindow(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(5)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeFlash,
		MaxViewers: 5, RampEvery: 300 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 5*time.Second)
	if report.StopReason != StopCompleted {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopCompleted, report.StopDetail)
	}
	if report.SustainedViewers != 5 {
		t.Fatalf("sustained viewers = %d, want 5", report.SustainedViewers)
	}

	st.mu.Lock()
	var masterTimes []time.Time
	for _, f := range st.fetches {
		if strings.HasSuffix(f.path, "master.m3u8") {
			masterTimes = append(masterTimes, f.at)
		}
	}
	st.mu.Unlock()

	if len(masterTimes) != 5 {
		t.Fatalf("master fetches = %d, want 5", len(masterTimes))
	}
	earliest, latest := masterTimes[0], masterTimes[0]
	for _, at := range masterTimes[1:] {
		if at.Before(earliest) {
			earliest = at
		}
		if at.After(latest) {
			latest = at
		}
	}
	spread := latest.Sub(earliest)
	if spread >= cfg.RampEvery/2 {
		t.Fatalf("master fetches spread over %v, want << RampEvery (%v): flash must launch all at once, not ramp", spread, cfg.RampEvery)
	}
}

// A flash viewer whose candidate isn't actually mkvgo-served must still be
// skipped, exactly like the ramp does today - flash changes WHEN viewers
// launch, never the eligibility rule itself. Because flash launches
// min(MaxViewers, available candidates) all at once, it always spends its
// entire candidate budget up front (unlike the ramp, which spends one
// candidate per tick and so still has slack left when an early tier gets
// skipped); the ineligible candidate here has nothing left to retry into,
// so the run ends as StopLibraryLimit - the point of this test is that it
// is never silently counted as served.
func TestRunner_FlashPlusMkvgoStillRequiresTheHeader(t *testing.T) {
	st := newServerState()
	st.servedByFn = func(mediaID, resource string, _ int) bool {
		return resource != "master" || mediaID != "m0" // m0 isn't actually mkvgo-served.
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(1)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo, Mode: ModeFlash,
		MaxViewers: 1, RampEvery: 30 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 3*time.Second)
	if report.SkippedCandidates != 1 {
		t.Fatalf("skipped candidates = %d, want 1 (m0 must be skipped for lacking the mkvgo header)", report.SkippedCandidates)
	}
	if report.SustainedViewers != 0 {
		t.Fatalf("sustained viewers = %d, want 0: an ineligible candidate must never count as served", report.SustainedViewers)
	}
	if report.StopReason != StopLibraryLimit {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopLibraryLimit, report.StopDetail)
	}
}

// --- Mode: seek ---------------------------------------------------------

func TestRunner_SeekModeJumpsNonSequentiallyAndRecordsTTFF(t *testing.T) {
	st := newServerState()
	st.targetDurRaw = "0.05" // 50ms
	st.segCount = 30
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{}, zeroActive, testLogger())
	cfg := Config{BaseURL: srv.URL, Token: "tkn", Mode: ModeSeek}
	events := make(chan viewerEvent, 1024)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go r.runViewer(ctx, cfg, 1, viewerTypeNatural, assignment{media: MediaCandidate{ID: "m0"}}, false, events, &wg)

	time.Sleep(2200 * time.Millisecond)
	cancel()
	wg.Wait()
	close(events)

	ttffCount := 0
	for ev := range events {
		if ev.kind == evTTFF {
			ttffCount++
		}
	}
	// 1 TTFF sample from the initial burst + at least 2 from seeks in ~2s
	// (a jump every 10x50ms=500ms).
	if ttffCount < 3 {
		t.Fatalf("ttff samples = %d, want >= 3 (1 initial + >= 2 seeks)", ttffCount)
	}

	st.mu.Lock()
	var segIdxs []int
	for _, f := range st.fetches {
		if idx, ok := segIndexFromPath(f.path); ok {
			segIdxs = append(segIdxs, idx)
		}
	}
	st.mu.Unlock()

	nonSequential := 0
	for i := 1; i < len(segIdxs); i++ {
		if segIdxs[i] != (segIdxs[i-1]+1)%st.segCount {
			nonSequential++
		}
	}
	if nonSequential < 2 {
		t.Fatalf("non-sequential segment fetches = %d, want >= 2 (the seeks): %v", nonSequential, segIdxs)
	}
}

func TestRunner_SeekModeIsDeterministicAcrossRuns(t *testing.T) {
	runOnce := func() []int {
		st := newServerState()
		st.targetDurRaw = "0.05"
		st.segCount = 30
		srv := httptest.NewServer(st.handler())
		defer srv.Close()

		r := New(&http.Client{}, fakeMediaLister{}, zeroActive, testLogger())
		cfg := Config{BaseURL: srv.URL, Token: "tkn", Mode: ModeSeek}
		events := make(chan viewerEvent, 1024)
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.Background())
		wg.Add(1)
		go r.runViewer(ctx, cfg, 3, viewerTypeNatural, assignment{media: MediaCandidate{ID: "m0"}}, false, events, &wg)

		time.Sleep(1500 * time.Millisecond)
		cancel()
		wg.Wait()
		close(events)

		st.mu.Lock()
		defer st.mu.Unlock()
		var jumps []int
		prev := -1
		for _, f := range st.fetches {
			idx, ok := segIndexFromPath(f.path)
			if !ok {
				continue
			}
			if prev >= 0 && idx != (prev+1)%st.segCount {
				jumps = append(jumps, idx)
			}
			prev = idx
		}
		return jumps
	}

	seqA := runOnce()
	seqB := runOnce()
	if len(seqA) < 2 || len(seqB) < 2 {
		t.Fatalf("not enough jumps recorded to compare determinism: A=%v B=%v", seqA, seqB)
	}
	n := min(len(seqA), len(seqB))
	for i := range n {
		if seqA[i] != seqB[i] {
			t.Fatalf("seek position %d diverged between runs: A=%d B=%d (full: A=%v B=%v)", i, seqA[i], seqB[i], seqA, seqB)
		}
	}
}

func TestRunner_SeekModeVerdictsUnchanged(t *testing.T) {
	st := newServerState()
	aud := &audience{}
	st.refuseBeyond(aud, 3)
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeSeek,
		MaxViewers: 6, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Seek mode keeps the normal climb's verdicts: the server refusing the 4th
	// stream still settles the answer at 3, exactly like ScenarioNatural with no
	// mode - scrubbing the timeline changes the load, not the arithmetic.
	report := waitForReport(t, r, 20*time.Second)
	if report.StopReason != StopAdmissionFull {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopAdmissionFull, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3", report.SustainedViewers)
	}
}

// --- Mode: endurance ------------------------------------------------------

func TestRunner_EnduranceHoldsPlateauThenCompletes(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(4)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeEndurance,
		MaxViewers: 2, RampEvery: 100 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 1500 * time.Millisecond,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 3*time.Second)
	if report.StopReason != StopCompleted {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopCompleted, report.StopDetail)
	}
	if report.SustainedViewers != 2 {
		t.Fatalf("sustained viewers = %d, want 2", report.SustainedViewers)
	}
	if !strings.Contains(report.StopDetail, "held 2 viewers") {
		t.Fatalf("stop detail = %q, want it to mention 'held 2 viewers'", report.StopDetail)
	}
	if report.Mode != ModeEndurance {
		t.Fatalf("report mode = %q, want %q", report.Mode, ModeEndurance)
	}
}

func TestRunner_EnduranceSLOBreachDuringHoldStillStops(t *testing.T) {
	st := newServerState()
	st.targetDurRaw = "0.1" // 100ms auto SLO
	var breaching atomic.Bool
	st.segmentHook = func(_ string, _ int) (int, map[string]string, time.Duration) {
		if breaching.Load() {
			return 0, nil, 200 * time.Millisecond // comfortably beyond the 100ms SLO
		}
		return 0, nil, 0
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(4)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeEndurance,
		MaxViewers: 2, RampEvery: 80 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the ramp reach its 2-viewer plateau cleanly, then start degrading
	// segment latency so the breach happens DURING the held plateau, not the
	// ramp-up. The plateau is when the run says it is holding: a fixed sleep
	// would land inside the answer's own measurement window, where a break is
	// retried once before it counts, and test something else.
	deadline := time.Now().Add(3 * time.Second)
	for r.Status().Phase != "holding" {
		if time.Now().After(deadline) {
			t.Fatalf("the run never reached its endurance plateau (phase %q)", r.Status().Phase)
		}
		time.Sleep(10 * time.Millisecond)
	}
	breaching.Store(true)

	report := waitForReport(t, r, 5*time.Second)
	if report.StopReason != StopSLOBreach {
		t.Fatalf("stop reason = %s, want %s (detail=%q): endurance must keep SLO verdicts active during the hold", report.StopReason, StopSLOBreach, report.StopDetail)
	}
}

// A server can keep its existing viewers perfectly fed and still take many
// seconds to let a NEW one start watching. To the person holding the remote,
// that IS the server being full - so it must end the climb.
func TestRunner_SlowStartupIsACapacityVerdict(t *testing.T) {
	st := newServerState()
	st.segCount = 40
	// Long segments (10s -> a 5s streaming budget), so the slow FIRST segment
	// below breaches the STARTUP budget and nothing else. This test is about a
	// server that feeds its regulars fine and still makes newcomers wait.
	st.targetDurRaw = "10"
	aud := &audience{}
	// Past 3 viewers, the FIRST segment of any newcomer crawls; everything
	// already streaming stays fast.
	st.segmentHook = func(_ string, segIdx int) (int, map[string]string, time.Duration) {
		if aud.size() > 3 && segIdx == 0 {
			return 0, nil, time.Duration(startupBudgetMs+500) * time.Millisecond
		}
		return 0, nil, 0
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	// Enough titles that every newcomer of the grid's churn starts a fresh one
	// at its first segment - the one this server makes crawl.
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(120)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		// The holds must outlast the slow startup they are meant to catch: a tier
		// judged before its newcomers have even received a first frame would be
		// declared healthy on no evidence.
		MaxViewers: 10, FastHold: 5 * time.Second, MeasureHold: 5 * time.Second, StepWindows: 3, MaxDuration: 120 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 120*time.Second)
	if report.StopReason != StopStartupTooSlow {
		t.Fatalf("stop reason = %s, want %s (detail=%q): a newcomer waiting seconds for its first frame is a full server", report.StopReason, StopStartupTooSlow, report.StopDetail)
	}
	if report.SustainedViewers >= 10 {
		t.Fatalf("sustained = %d: the climb ignored the startup budget", report.SustainedViewers)
	}
}

// EVERY 503 is the server saying "not right now" - a capacity answer, not a
// harness failure. Insisting on a particular header before believing it meant a
// real refusal (a rate limiter, a throttle, a load shed) got filed as "an error
// interrupted the test", throwing the measurement away.
func TestRunner_AnyServiceUnavailableIsACapacityAnswer(t *testing.T) {
	st := newServerState()
	aud := &audience{}
	// A 503 with NO Evey error header - what a generic throttle returns.
	st.masterHook = func(_ int, _, _ string) (int, map[string]string) {
		if aud.size() > 3 {
			return http.StatusServiceUnavailable, nil
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 8, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 20*time.Second)
	if report.StopReason == StopError {
		t.Fatalf("a 503 was reported as an error (%q): the server refusing IS the measurement", report.StopDetail)
	}
	if report.StopReason != StopAdmissionFull {
		t.Fatalf("stop reason = %s, want %s", report.StopReason, StopAdmissionFull)
	}
}

// A server that stops answering altogether is the loudest capacity signal there
// is. Reporting the client timeout as "an error interrupted the test" threw away
// the very number the run existed to find.
func TestRunner_ServerThatStopsAnsweringIsSaturatedNotBroken(t *testing.T) {
	st := newServerState()
	aud := &audience{}
	// Past 3 viewers the server simply never answers.
	st.masterHook = func(_ int, _, _ string) (int, map[string]string) {
		if aud.size() > 3 {
			time.Sleep(2 * time.Second) // longer than the client's timeout below
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{Timeout: 300 * time.Millisecond}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	aud.bind(r)
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 8, FastHold: 400 * time.Millisecond, MeasureHold: 800 * time.Millisecond, StepWindows: 2, MaxDuration: 30 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 30*time.Second)
	if report.StopReason == StopError {
		t.Fatalf("a server that stopped answering was reported as an error (%q): that IS saturation", report.StopDetail)
	}
	if report.StopReason != StopAdmissionFull {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopAdmissionFull, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained = %d, want 3", report.SustainedViewers)
	}
}

// A run whose viewers never touch the disk (every byte from the OS page cache)
// must SAY so: the viewer count stands, but it says nothing about a library too
// big to sit in RAM.
func TestReport_FlagsAServerThatNeverTouchedItsDisk(t *testing.T) {
	starved := diskReadStarved(ResourceStats{
		Samples:                10,
		ServedBytesPerSecAvg:   50 << 20,                        // 50 MiB/s out the door
		DiskReadBytesPerSecAvg: 1 << 20, DiskReadMeasured: true, // 1 MiB/s off the disk
	})
	if !starved {
		t.Fatalf("50 MiB/s served against 1 MiB/s read means the page cache is doing the work; the report must say so")
	}
	if diskReadStarved(ResourceStats{Samples: 10, ServedBytesPerSecAvg: 50 << 20, DiskReadBytesPerSecAvg: 40 << 20, DiskReadMeasured: true}) {
		t.Fatalf("a server reading most of what it serves off the disk is not starved")
	}
}

// A real player pulls the VIDEO and its AUDIO for every moment of playback -
// Evey's direct-play path demuxes them into separate renditions. A simulated
// viewer that only fetched the video was costing the server about half of what a
// real one does, and never exercised the packager's single-walk path: the run was
// measuring a client nobody runs.
func TestRunner_ViewerPullsItsAudioRenditionToo(t *testing.T) {
	st := newDemuxedServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 1, FastHold: time.Second, MeasureHold: time.Second, MaxDuration: 6 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	r.Cancel()
	waitForReport(t, r, 5*time.Second)

	st.mu.Lock()
	defer st.mu.Unlock()
	var videoSegs, audioSegs, audioPlaylists int
	for _, f := range st.fetches {
		switch {
		case strings.HasSuffix(f.path, "a1.m3u8"):
			audioPlaylists++
		case strings.Contains(f.path, "a1-seg"):
			audioSegs++
		case strings.HasSuffix(f.path, ".ts"):
			videoSegs++
		}
	}
	if audioPlaylists == 0 {
		t.Fatalf("the viewer never fetched the audio rendition's playlist: it is ignoring EXT-X-MEDIA")
	}
	if audioSegs == 0 {
		t.Fatalf("the viewer fetched %d video segments and NO audio segment: a real player pulls both", videoSegs)
	}
	// Both renditions, roughly in step: the server must produce them from the
	// same window, which is the whole point of measuring this way.
	if videoSegs == 0 || audioSegs < videoSegs/2 {
		t.Fatalf("video=%d audio=%d segments: the two renditions are not being pulled together", videoSegs, audioSegs)
	}
}

// A flash viewer the server refuses (or drops) stays in the launch count - the
// verdict is served-of-launched - but its encoder session, if one was admitted
// before the shed, keeps grinding for nobody and steals CPU from the viewers
// still under observation. It must be reaped DURING the window, not at the
// end, and only for the titles no surviving viewer holds.
func TestRunner_FlashReapsRefusedViewersMidWindow(t *testing.T) {
	st := newServerState()
	st.masterHook = func(_ int, mediaID, _ string) (int, map[string]string) {
		if mediaID == "m1" || mediaID == "m3" {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	var mu sync.Mutex
	var reaps [][]string
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(5)}, zeroActive, testLogger()).
		WithSessionReaper(func(ids []string) {
			mu.Lock()
			defer mu.Unlock()
			reaps = append(reaps, append([]string(nil), ids...))
		})
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeFlash,
		MaxViewers: 5, RampEvery: 100 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 600 * time.Millisecond, StepWindows: 2, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 5*time.Second)
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3: the reap must not touch the verdict", report.SustainedViewers)
	}

	mu.Lock()
	defer mu.Unlock()
	// The end-of-run reap covers every candidate; a PARTIAL reap can only come
	// from the window itself.
	partial := 0
	for _, ids := range reaps {
		if len(ids) >= 5 {
			continue
		}
		partial++
		set := map[string]bool{}
		for _, id := range ids {
			set[id] = true
		}
		if !set["m1"] && !set["m3"] {
			t.Errorf("mid-window reap %v names neither refused title", ids)
		}
		for _, survivor := range []string{"m0", "m2", "m4"} {
			if set[survivor] {
				t.Errorf("mid-window reap %v would kill survivor %s's stream", ids, survivor)
			}
		}
	}
	if partial == 0 {
		t.Fatalf("no mid-window reap: refused flash viewers' sessions were left grinding until the end (reaps=%v)", reaps)
	}
}

// A run raises the playback signal before anything else happens and lowers it
// when it ends: background workers yield for the storage probe too, not only
// once the first simulated viewer has made a request.
func TestRunner_HoldsThePlaybackSignalForTheWholeRun(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	var held, released atomic.Int32
	var viewersWhenRaised atomic.Int32
	viewersWhenRaised.Store(-1)
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(4)}, zeroActive, testLogger())
	r.WithPlaybackSignal(func() func() {
		held.Add(1)
		viewersWhenRaised.Store(int32(r.Status().Viewers))
		return func() { released.Add(1) }
	})
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 2, RampEvery: 50 * time.Millisecond, FastHold: 100 * time.Millisecond, MeasureHold: 200 * time.Millisecond, StepWindows: 2, MaxDuration: 10 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForReport(t, r, 10*time.Second)
	if held.Load() != 1 || released.Load() != 1 {
		t.Fatalf("signal raised %d time(s), released %d: want exactly once each", held.Load(), released.Load())
	}
	if v := viewersWhenRaised.Load(); v != 0 {
		t.Fatalf("signal raised with %d viewers already spawned, want 0: it must precede the first viewer", v)
	}
}

// Every request of a simulated viewer says it is one, so the server skips the
// work a real playback start pays for later (the subtitle prewarm).
func TestFetchOnce_MarksTheRequestAsTheLoadTest(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got = req.Header.Get(headerLoadTest)
	}))
	defer srv.Close()
	r := New(srv.Client(), fakeMediaLister{}, zeroActive, testLogger())
	if _, _, _, _, err := r.fetchOnce(context.Background(), srv.URL, "tok", false); err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatalf("the request carried no %s header", headerLoadTest)
	}
}

// The run logs what it added to the packager's window counters, and the share
// of window reads that read a window again.
func TestRunner_LogsTheRunsShareOfTheWindowCounters(t *testing.T) {
	var logs bytes.Buffer
	r := New(&http.Client{}, fakeMediaLister{}, zeroActive, slog.New(slog.NewTextHandler(&logs, nil)))
	r.logWindowStats(
		map[string]int64{"plans": 3, "window_builds": 100, "window_rebuilds": 10},
		map[string]int64{"plans": 5, "window_builds": 300, "window_rebuilds": 60},
	)
	out := logs.String()
	for _, want := range []string{"loadtest: packager windows", "window_builds=200", "window_rebuilds=50", "rebuild_pct=25", "plans=5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log lacks %q: %s", want, out)
		}
	}
}

// Each segment's own EXTINF is read in playlist order; a segment without one
// is unknown (0).
func TestSegmentDurationsMs_ReadsEachSegmentsOwnLength(t *testing.T) {
	body := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:12\n#EXTINF:6.006,\nseg0.m4s\n#EXTINF:11.5,\nseg1.m4s\nseg2.m4s\n#EXT-X-ENDLIST\n")
	got := segmentDurationsMs(body)
	if len(got) != 3 || got[0] != 6006 || got[1] != 11500 || got[2] != 0 {
		t.Fatalf("durations = %v, want [6006 11500 0]", got)
	}
}

// The budget no longer depends on the first stream's target duration: every
// fetch is scaled to the reference segment, so 4 s for a 12 s segment is the
// same verdict as 2 s for a 6 s one, whichever title a run drew first.
func TestSLOTracker_JudgesEachFetchAgainstItsOwnSegment(t *testing.T) {
	a, b := newSLOTracker(0), newSLOTracker(0)
	a.lockTarget(viewerTypeMkvgo, 17000)
	b.lockTarget(viewerTypeMkvgo, 4000)
	if a.budgets()[string(viewerTypeMkvgo)] != b.budgets()[string(viewerTypeMkvgo)] {
		t.Fatalf("budgets differ by first target: %v vs %v", a.budgets(), b.budgets())
	}
	if got := a.scaled(4000, 12000); got != 2000 {
		t.Fatalf("4 s for a 12 s segment scaled to %d ms, want 2000", got)
	}
	if got := a.scaled(4000, 0); got != 4000 {
		t.Fatalf("a segment of unknown length must be taken as it is, got %d", got)
	}
	if got := newSLOTracker(5000).scaled(4000, 12000); got != 4000 {
		t.Fatalf("a pinned budget is absolute, got %d", got)
	}
}

// A measured size is judged window by window on startup too: slow joins in
// one window do not condemn the next window, whose newcomers start in time.
func TestRunner_StartupIsJudgedOnTheWindowInProgress(t *testing.T) {
	r := New(&http.Client{}, fakeMediaLister{}, zeroActive, testLogger())
	pool := newCandidatePool(manyCandidates(3), time.Minute)
	slo := newSLOTracker(0)
	samples := newSampleState()
	flash := flashState{served: map[int]bool{}, shed: map[int]bool{}}
	ramp := rampState{tier: 30, viewerTypeBySlot: map[int]viewerType{}, cancelBySlot: map[int]context.CancelFunc{}, roundBySlot: map[int]int{}, replayedByTier: map[int]int{}, herdBySlot: map[int]bool{}, audienceIntact: true}
	cl := &climb{phase: phaseBisect, target: 30}
	res := newResourceState(0, time.Now().Add(-time.Minute))
	res.openWindow(30, cl.phase.String(), true, time.Now())
	events := make(chan viewerEvent, 8)
	var wg sync.WaitGroup
	slot := 0
	send := func(ms int) {
		slot++
		ramp.cancelBySlot[slot] = func() {}
		r.handleViewerEvent(viewerEvent{kind: evTTFF, vt: viewerTypeMkvgo, slot: slot, ms: ms}, false, context.Background(), Config{}, pool, events, &wg, &ramp, &flash, slo, &samples, cl, res)
	}
	for range startupMinSamples {
		send(startupBudgetMs * 2)
	}
	if cl.broke != StopStartupTooSlow {
		t.Fatalf("first window: broke = %q, want startup_too_slow", cl.broke)
	}
	cl.broke, cl.detail = "", ""
	res.nextWindow(30, time.Now(), cl.phase.String())
	for range startupMinSamples {
		send(startupBudgetMs / 4)
	}
	if cl.broke != "" {
		t.Fatalf("second window's newcomers started in time, yet broke = %q: the first window's startups were judged again", cl.broke)
	}
}

// The idle reap spares a title only for a live viewer that may encode it. An
// mkvgo viewer is served by the packager and holds no producer session, so a
// shed transcode viewer's title must be reaped even when a mkvgo survivor
// watches it too - sparing it kept the dead viewer's encoder slot, and the next
// newcomer was refused (the bisection test failed about 1 run in 7).
func TestRunner_IdleReapIgnoresMkvgoViewersOnASharedTitle(t *testing.T) {
	var reaped []string
	r := New(&http.Client{}, fakeMediaLister{}, zeroActive, testLogger()).
		WithSessionReaper(func(ids []string) { reaped = append(reaped, ids...) })
	pool := newCandidatePool(manyCandidates(3), time.Minute)
	ramp := rampState{
		mediaBySlot:      map[int]string{1: "m1", 2: "m2", 3: "m2"},
		viewerTypeBySlot: map[int]viewerType{1: viewerTypeMkvgo, 2: viewerTypeTranscode, 3: viewerTypeMkvgo},
	}
	r.reapIdleSessions(pool, &ramp)
	got := map[string]bool{}
	for _, id := range reaped {
		got[id] = true
	}
	if !got["m0"] || !got["m1"] {
		t.Fatalf("reaped %v: a title only a mkvgo viewer watches must be reaped", reaped)
	}
	if got["m2"] {
		t.Fatalf("reaped %v: a title a live transcode viewer watches must be spared", reaped)
	}
}
