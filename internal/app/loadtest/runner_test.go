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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- fake media pool ------------------------------------------------------

type fakeMediaLister struct{ candidates []MediaCandidate }

func (f fakeMediaLister) ListLoadTestCandidates(_ context.Context, limit int) ([]MediaCandidate, error) {
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

func testLogger() *slog.Logger {
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
	at          time.Time
}

type serverState struct {
	mu sync.Mutex

	segCount     int
	targetDurRaw string

	masterHits int

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

	fetches []fetchRecord
}

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

func (s *serverState) record(r *http.Request) {
	s.mu.Lock()
	s.fetches = append(s.fetches, fetchRecord{
		path:        r.URL.Path,
		audioOffset: r.URL.Query().Get("audioOffsetMs"),
		rawQuery:    r.URL.RawQuery,
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
	idx := strings.LastIndex(base, "seg")
	if idx < 0 {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSuffix(base[idx+3:], ".ts"))
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
		s.record(r)
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			s.serveMaster(w, r)
		case strings.HasSuffix(r.URL.Path, ".m3u8"):
			s.serveVariant(w, r)
		case strings.HasSuffix(r.URL.Path, ".ts"):
			s.serveSegment(w, r)
		default:
			http.NotFound(w, r)
		}
	}
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
	bandwidths := []int{100_000, 200_000, 300_000}
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i, bw := range bandwidths {
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d\n%sv%d.m3u8\n", bw, dir, i)
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
	s.mu.Unlock()

	path := r.URL.Path
	mediaID := mediaIDFromPath(path)
	dir := path[:strings.LastIndex(path, "/")+1]
	variant := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".m3u8")

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%s\n", targetDur)
	for i := range segCount {
		b.WriteString("#EXTINF:1.0,\n")
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
	if idx := strings.LastIndex(base, "seg"); idx >= 0 {
		if v, err := strconv.Atoi(strings.TrimSuffix(base[idx+3:], ".ts")); err == nil {
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

func TestRunner_AdmissionFullStopsRampAtSustainedTier(t *testing.T) {
	st := newServerState()
	st.masterHook = func(hitNum int, _, _ string) (int, map[string]string) {
		if hitNum == 4 {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 6, RampEvery: 40 * time.Millisecond, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 3*time.Second)
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

func TestRunner_SLOBreachStopsAtPriorTier(t *testing.T) {
	st := newServerState()
	st.targetDurRaw = "0.1" // 100ms auto SLO
	var tier atomic.Int32
	st.masterHook = func(hitNum int, _, _ string) (int, map[string]string) {
		tier.Store(int32(hitNum))
		return 0, nil
	}
	st.segmentHook = func(_ string, _ int) (int, map[string]string, time.Duration) {
		n := tier.Load()
		if n <= 3 {
			return 0, nil, time.Duration(n) * 10 * time.Millisecond
		}
		// Comfortably beyond the 100ms SLO. RampEvery below is large enough
		// that even two such fetches (the SLO breach check can require the
		// 2nd-highest of a full 20-sample window) land well before the next
		// tier's ramp tick, so the breach is unambiguously attributed to
		// this tier and not the next one.
		return 0, nil, 200 * time.Millisecond
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(8)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 8, RampEvery: 700 * time.Millisecond, MaxDuration: 8 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 8*time.Second)
	if report.StopReason != StopSLOBreach {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopSLOBreach, report.StopDetail)
	}
	if report.SustainedViewers != 3 {
		t.Fatalf("sustained viewers = %d, want 3", report.SustainedViewers)
	}
	if report.SegmentSLOMs != 100 {
		t.Fatalf("segment SLO ms = %d, want 100 (auto-detected)", report.SegmentSLOMs)
	}
}

func TestRunner_NaturalScenarioLibraryLimitStopsCleanly(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 5, RampEvery: 40 * time.Millisecond, MaxDuration: 3 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 3*time.Second)
	if report.StopReason != StopLibraryLimit {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopLibraryLimit, report.StopDetail)
	}
	if report.SustainedViewers != 2 {
		t.Fatalf("sustained viewers = %d, want 2", report.SustainedViewers)
	}
	if report.SharedCacheViewers != 0 {
		t.Fatalf("natural scenario must never share media, got %d shared viewers", report.SharedCacheViewers)
	}
}

func TestRunner_TranscodeScenarioForksBeyondLibrary(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioTranscode,
		MaxViewers: 4, RampEvery: 40 * time.Millisecond, MaxDuration: 3 * time.Second,
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
	if report.SharedCacheViewers != 2 {
		t.Fatalf("shared cache viewers = %d, want 2", report.SharedCacheViewers)
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
	if forked != 2 {
		t.Fatalf("master fetches carrying audioOffsetMs = %d, want 2", forked)
	}
}

func TestRunner_CancelStopsAndReportsPartial(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(20)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural,
		MaxViewers: 20, RampEvery: 60 * time.Millisecond, MaxDuration: 10 * time.Second,
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
		MaxViewers: 3, RampEvery: 50 * time.Millisecond, MaxDuration: 2 * time.Second,
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
		MaxViewers: 5, RampEvery: 500 * time.Millisecond, MaxDuration: 5 * time.Second,
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
		MaxViewers: 1, RampEvery: 5 * time.Second, MaxDuration: 2 * time.Second,
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

// --- mkvgo scenario -------------------------------------------------------

func TestRunner_MkvgoSkipsIneligibleCandidateAndCountsIt(t *testing.T) {
	st := newServerState()
	st.servedByFn = func(mediaID, resource string, _ int) bool {
		if resource == "master" && mediaID == "m1" {
			return false // m1 isn't actually mkvgo-served, sandwiched between two that are.
		}
		return true
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(3)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioMkvgo,
		MaxViewers: 5, RampEvery: 40 * time.Millisecond, MaxDuration: 3 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	report := waitForReport(t, r, 3*time.Second)
	if report.StopReason != StopLibraryLimit {
		t.Fatalf("stop reason = %s, want %s (detail=%q)", report.StopReason, StopLibraryLimit, report.StopDetail)
	}
	if report.SustainedViewers != 2 {
		t.Fatalf("sustained viewers = %d, want 2", report.SustainedViewers)
	}
	if report.SkippedCandidates != 1 {
		t.Fatalf("skipped candidates = %d, want 1", report.SkippedCandidates)
	}
	if report.StopDetail == "" {
		t.Fatalf("expected an explicit stop detail")
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
		MaxViewers: 1, RampEvery: 5 * time.Second, MaxDuration: 2 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
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
		MaxViewers: 2, RampEvery: 40 * time.Millisecond, MaxDuration: 3 * time.Second,
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
		"segment_slo_ms", "samples", "shared_cache_viewers", "skipped_candidates", "duration_ms",
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
		MaxViewers: 6, RampEvery: 40 * time.Millisecond, MaxDuration: 4 * time.Second,
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

	mkvgoHits, transcodeHits := countMasterHitsByType(st)
	if mkvgoHits != 3 || transcodeHits != 3 {
		t.Fatalf("master hits by type = (mkvgo=%d, transcode=%d), want (3,3)", mkvgoHits, transcodeHits)
	}
}

func TestRunner_LightMixIsMostlyMkvgo(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(10)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioLight,
		MaxViewers: 5, RampEvery: 40 * time.Millisecond, MaxDuration: 4 * time.Second,
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

func TestRunner_HeavyMixAdmissionFullOnTranscodePool(t *testing.T) {
	st := newServerState()
	var transcodeHits atomic.Int32
	st.masterHook = func(_ int, _, rawQuery string) (int, map[string]string) {
		if !strings.Contains(rawQuery, "vd=transcode") {
			return 0, nil
		}
		if transcodeHits.Add(1) == 3 {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(10)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioHeavy,
		MaxViewers: 8, RampEvery: 40 * time.Millisecond, MaxDuration: 4 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// heavy pattern is [mkvgo, transcode, transcode, transcode, transcode]:
	// slot1=mkvgo, slot2=transcode(1st), slot3=transcode(2nd),
	// slot4=transcode(3rd, denied) -> sustained=3 (slots 1-3).
	report := waitForReport(t, r, 4*time.Second)
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
		MaxViewers: 4, RampEvery: 40 * time.Millisecond, MaxDuration: 3 * time.Second,
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
		MaxViewers: 5, RampEvery: 100 * time.Millisecond, MaxDuration: 5 * time.Second,
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
		MaxViewers: 5, RampEvery: 300 * time.Millisecond, MaxDuration: 5 * time.Second,
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
		MaxViewers: 1, RampEvery: 30 * time.Millisecond, MaxDuration: 3 * time.Second,
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
	go r.runViewer(ctx, cfg, 1, viewerTypeNatural, MediaCandidate{ID: "m0"}, false, events, &wg)

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
		go r.runViewer(ctx, cfg, 3, viewerTypeNatural, MediaCandidate{ID: "m0"}, false, events, &wg)

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
	st.masterHook = func(hitNum int, _, _ string) (int, map[string]string) {
		if hitNum == 4 {
			return http.StatusServiceUnavailable, map[string]string{headerEveyError: errCodeTooManyStreams}
		}
		return 0, nil
	}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: ScenarioNatural, Mode: ModeSeek,
		MaxViewers: 6, RampEvery: 40 * time.Millisecond, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Seek mode keeps the normal ramp's verdicts: an admission refusal still
	// stops the run at the prior sustained tier, exactly like ScenarioNatural
	// without a mode.
	report := waitForReport(t, r, 3*time.Second)
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
		MaxViewers: 2, RampEvery: 100 * time.Millisecond, MaxDuration: 1500 * time.Millisecond,
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
		MaxViewers: 2, RampEvery: 80 * time.Millisecond, MaxDuration: 5 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the ramp reach its 2-viewer plateau cleanly, then start degrading
	// segment latency so the breach happens DURING the held plateau, not the
	// ramp-up.
	time.Sleep(400 * time.Millisecond)
	breaching.Store(true)

	report := waitForReport(t, r, 5*time.Second)
	if report.StopReason != StopSLOBreach {
		t.Fatalf("stop reason = %s, want %s (detail=%q): endurance must keep SLO verdicts active during the hold", report.StopReason, StopSLOBreach, report.StopDetail)
	}
}
