package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestClassifyLog(t *testing.T) {
	cases := []struct {
		name      string
		component string
		msg       string
		where     string
		source    string
	}{
		{"ffmpeg hls start", "", "hls: ffmpeg start", wherePlayer, sourceFFmpeg},
		{"software producer", "software_producer", "software: copy seek landed before keyframe", wherePlayer, sourceFFmpeg},
		{"segment orchestrator", "segment_orchestrator", "segment ready", wherePlayer, sourceFFmpeg},
		{"transcode decision", "", "transcode: decision", wherePlayer, sourceFFmpeg},
		{"seek-index worker snake", "seek_index", "seek-index job finished", whereSeekIndex, sourceMkvgo},
		{"seek-index svc kebab", "seek-index", "reindex accepted", whereSeekIndex, sourceMkvgo},
		{"mkvgo remux", "", "mkvgo remux: dropped track", whereSeekIndex, sourceMkvgo},
		{"subtitle", "", "subtitle: mkvgo WebVTT extraction failed", whereSubtitle, sourceMkvgo},
		{"remux", "", "remux: produce failed", whereRemux, sourceOther},
		{"hwdetect", "", "hwdetect: encoder present but probe failed", whereHWDetect, sourceFFmpeg},
		{"benchmark", "", "benchmark: HW feature supported", whereHWDetect, sourceFFmpeg},
		{"qoe", "qoe_sink", "qoe ingest", whereQoE, sourceQoE},
		{"chromecast", "", "chromecast: session started", whereChromecast, sourceOther},
		{"auth token reuse", "", "refresh rejected: token reuse detected", whereAuth, sourceOther},
		{"infra scheduler", "scheduler", "tick", whereInfra, sourceOther},
		{"infra grpc", "grpc", "call", whereInfra, sourceGRPC},
		{"unknown", "", "something random happened", whereOther, sourceOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			where, source := classifyLog(c.component, c.msg, "")
			if where != c.where {
				t.Errorf("where: want %q, got %q", c.where, where)
			}
			if source != c.source {
				t.Errorf("source: want %q, got %q", c.source, source)
			}
		})
	}
}

func TestParseLogLine_PromotesAndNormalizes(t *testing.T) {
	line := `{"time":"2026-07-07T19:22:01.9Z","level":"INFO","msg":"hls: ffmpeg start","media_path":"/media/x.mkv","sessionID":"ps-abc","user_id":"usr-1","variant":0,"request_id":"req-9"}`
	e, ok := parseLogLine(line)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if e.Level != "INFO" || e.Msg != "hls: ffmpeg start" {
		t.Fatalf("bad top fields: %+v", e)
	}
	if e.Where != wherePlayer || e.Source != sourceFFmpeg {
		t.Fatalf("bad classify: where=%s source=%s", e.Where, e.Source)
	}
	if e.SessionID != "ps-abc" {
		t.Fatalf("sessionID normalization failed: %q", e.SessionID)
	}
	if e.UserID != "usr-1" {
		t.Fatalf("user_id failed: %q", e.UserID)
	}
	if e.RequestID != "req-9" {
		t.Fatalf("request_id failed: %q", e.RequestID)
	}
	if e.Origin != "server" {
		t.Fatalf("origin: want server, got %q", e.Origin)
	}
	// variant + media_path land in Fields (not promoted).
	if e.Fields["variant"] == nil || e.Fields["media_path"] == nil {
		t.Fatalf("expected variant/media_path in Fields: %+v", e.Fields)
	}
	// promoted keys must NOT leak into Fields.
	if _, ok := e.Fields["level"]; ok {
		t.Fatal("level should be promoted, not in Fields")
	}
}

func TestParseLogLine_NonJSONDropped(t *testing.T) {
	if _, ok := parseLogLine("panic: not json at all"); ok {
		t.Fatal("expected non-JSON line to be dropped")
	}
}

func writeLog(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSearchLogs_FilterAndOrder(t *testing.T) {
	dir := t.TempDir()
	older := writeLog(t, dir, "evey-2026-07-06.log",
		`{"time":"2026-07-06T10:00:00Z","level":"INFO","msg":"hls: ffmpeg start","media_id":"m1"}`,
		`{"time":"2026-07-06T10:00:01Z","level":"WARN","msg":"subtitle: fallback","media_id":"m1"}`,
	)
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T11:00:00Z","level":"ERROR","msg":"hls: ffmpeg exited with error","media_id":"m2"}`,
		`{"time":"2026-07-07T11:00:05Z","level":"INFO","msg":"seek-index job finished","component":"seek_index","media_id":"m2"}`,
	)

	// No filter → all 4, newest first.
	all, err := searchLogs(dir, logSearchFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 4 || len(all.Entries) != 4 {
		t.Fatalf("want 4 entries, got total=%d len=%d", all.Total, len(all.Entries))
	}
	if all.Entries[0].Time != "2026-07-07T11:00:05Z" {
		t.Fatalf("newest-first broken: first=%s", all.Entries[0].Time)
	}

	// Level filter.
	errs, _ := searchLogs(dir, logSearchFilter{Levels: map[string]bool{"ERROR": true}, Limit: 100})
	if errs.Total != 1 || errs.Entries[0].Msg != "hls: ffmpeg exited with error" {
		t.Fatalf("level filter failed: %+v", errs)
	}

	// where=player
	pl, _ := searchLogs(dir, logSearchFilter{Where: wherePlayer, Limit: 100})
	if pl.Total != 2 {
		t.Fatalf("where=player want 2, got %d", pl.Total)
	}

	// source=mkvgo (only the seek-index line; "subtitle: fallback" has no mkvgo)
	mk, _ := searchLogs(dir, logSearchFilter{Source: sourceMkvgo, Limit: 100})
	if mk.Total != 1 {
		t.Fatalf("source=mkvgo want 1, got %d", mk.Total)
	}

	// media_id
	m2, _ := searchLogs(dir, logSearchFilter{MediaID: "m2", Limit: 100})
	if m2.Total != 2 {
		t.Fatalf("media_id=m2 want 2, got %d", m2.Total)
	}

	// free text (case-insensitive)
	q, _ := searchLogs(dir, logSearchFilter{Q: "EXITED", Limit: 100})
	if q.Total != 1 {
		t.Fatalf("q=EXITED want 1, got %d", q.Total)
	}
}

func TestSearchLogs_TimeWindow(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T10:00:00Z","level":"INFO","msg":"a"}`,
		`{"time":"2026-07-07T12:00:00Z","level":"INFO","msg":"b"}`,
	)
	since, _ := time.Parse(time.RFC3339, "2026-07-07T11:00:00Z")
	res, _ := searchLogs(dir, logSearchFilter{Since: since, Limit: 100})
	if res.Total != 1 || res.Entries[0].Msg != "b" {
		t.Fatalf("time window failed: %+v", res)
	}
}

func TestSearchLogs_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T10:00:00Z","level":"INFO","msg":"a"}`,
		`{"time":"2026-07-07T10:00:01Z","level":"INFO","msg":"b"}`,
		`{"time":"2026-07-07T10:00:02Z","level":"INFO","msg":"c"}`,
	)
	// newest first: c,b,a ; offset 1 limit 1 → b
	res, _ := searchLogs(dir, logSearchFilter{Limit: 1, Offset: 1})
	if res.Total != 3 || len(res.Entries) != 1 || res.Entries[0].Msg != "b" {
		t.Fatalf("offset/limit failed: total=%d entries=%+v", res.Total, res.Entries)
	}
}

func TestSearchLogs_MissingDir(t *testing.T) {
	if _, err := searchLogs(filepath.Join(t.TempDir(), "nope"), logSearchFilter{}); err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestParseLevels(t *testing.T) {
	got := parseLevels("error, warn ,bogus")
	if !got["ERROR"] || !got["WARN"] || got["BOGUS"] {
		t.Fatalf("bad level set: %+v", got)
	}
	if parseLevels("") != nil || parseLevels("nonsense") != nil {
		t.Fatal("empty/invalid should be nil")
	}
}

func TestParseLogTime(t *testing.T) {
	if !parseLogTime("").IsZero() {
		t.Fatal("empty should be zero")
	}
	rfc := parseLogTime("2026-07-07T10:00:00Z")
	if rfc.IsZero() {
		t.Fatal("rfc3339 should parse")
	}
	ms := parseLogTime("1783451578048")
	if ms.IsZero() {
		t.Fatal("unix ms should parse")
	}
}

func TestHandleSearchLogs_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T10:00:00Z","level":"ERROR","msg":"hls: ffmpeg exited with error","media_id":"m2"}`,
	)
	h := &adminHandler{logDir: dir}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/search?level=error&where=player", nil)
	h.handleSearchLogs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("want json, got %q", ct)
	}
	if body := rec.Body.String(); !contains(body, "ffmpeg exited") {
		t.Fatalf("expected match in body: %s", body)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// A verbose recent file must not make an OLDER explicit time window
// unreachable: lines outside [Since,Until] are not the work the operator
// asked for, so they must not spend the scan budget. Without this, a busy
// instance (300k+ lines/day) makes `since=<yesterday>` return nothing at
// all - which is exactly what blocked the 2026-07-13 playback incident.
func TestSearchLogs_WindowSurvivesVerboseRecentLines(t *testing.T) {
	dir := t.TempDir()
	// The target: one line inside the wanted window.
	target := `{"time":"2026-07-07T02:00:00Z","level":"WARN","msg":"the line we need"}`
	// Noise: far more recent lines than the scan budget allows.
	noise := make([]string, 0, logScanLineBudget+10)
	noise = append(noise, target)
	for range logScanLineBudget + 5 {
		noise = append(noise, `{"time":"2026-07-07T09:00:00Z","level":"INFO","msg":"grpc unary"}`)
	}
	writeLog(t, dir, "evey-2026-07-07.log", noise...)

	since, _ := time.Parse(time.RFC3339, "2026-07-07T01:00:00Z")
	until, _ := time.Parse(time.RFC3339, "2026-07-07T03:00:00Z")
	res, err := searchLogs(dir, logSearchFilter{Since: since, Until: until, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Entries) != 1 || res.Entries[0].Msg != "the line we need" {
		t.Fatalf("windowed search drowned by recent noise: total=%d entries=%d", res.Total, len(res.Entries))
	}
}

// buildLogLines makes n distinct, time-ordered matching lines (oldest first,
// which is the on-disk append order) so a test can assert exactly WHICH page
// it got back, not merely how many rows.
func buildLogLines(n int) []string {
	out := make([]string, 0, n)
	base, _ := time.Parse(time.RFC3339, "2026-07-01T00:00:00Z")
	for i := range n {
		out = append(out, `{"time":"`+base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano)+
			`","level":"INFO","msg":"needle","component":"grpc"}`)
	}
	return out
}

// Deep pagination must reach PAST the retain cap. This is the failure that
// made a real prod investigation impossible on 2026-07-28: the search reported
// total=N but every page beyond the cap came back EMPTY, so a line that was
// genuinely on disk could not be produced as evidence. An offset the API
// accepts (parseLogSearchInt allows up to 1e6) must actually return rows.
func TestSearchLogs_DeepOffsetIsReachable(t *testing.T) {
	dir := t.TempDir()
	const n = logRetainCap + 250
	writeLog(t, dir, "evey-2026-07-01.log", buildLogLines(n)...)

	res, err := searchLogs(dir, logSearchFilter{Q: "needle", Offset: logRetainCap + 100, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	// The scan stops once the page is settled: the total is then a lower
	// bound, and hasMore says older matches remain.
	if res.Total < logRetainCap+150 || !res.HasMore {
		t.Fatalf("total = %d, hasMore = %v: want at least %d and more to come", res.Total, res.HasMore, logRetainCap+150)
	}
	if len(res.Entries) != 50 {
		t.Fatalf("deep page came back with %d entries, want 50 - offsets past the retain cap are unreachable", len(res.Entries))
	}
}

// Paging through the whole result set must yield every distinct line exactly
// once: a page that silently repeats or skips rows is worse than an empty one,
// because the operator cannot tell.
func TestSearchLogs_PagesCoverEveryMatchExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	const n = 1200
	writeLog(t, dir, "evey-2026-07-01.log", buildLogLines(n)...)

	seen := map[string]int{}
	for off := 0; off < n; off += 100 {
		res, err := searchLogs(dir, logSearchFilter{Q: "needle", Offset: off, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range res.Entries {
			seen[e.Time]++
		}
	}
	if len(seen) != n {
		t.Fatalf("paging saw %d distinct lines, want %d", len(seen), n)
	}
	for ts, c := range seen {
		if c != 1 {
			t.Fatalf("line %s returned %d times across pages, want exactly 1", ts, c)
		}
	}
}

// `truncated` means "the SCAN was cut short, counts are a lower bound" - it is
// the signal that tells an operator to narrow the query. Having more matches
// than fit on one page is ordinary pagination and must NOT raise it, or the
// flag cries wolf on every broad-but-complete search.
func TestSearchLogs_TruncatedOnlyMeansScanBudgetHit(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-01.log", buildLogLines(logRetainCap+500)...)

	res, err := searchLogs(dir, logSearchFilter{Q: "needle", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated {
		t.Fatal("truncated set on a complete scan that simply had more matches than one page")
	}
}

// The scan walks files by mtime and lines bottom-up, so arrivals are only
// ALMOST newest-first: around a rotation an older-by-mtime file can still hold
// a newer line. A keeper that simply took the first `cap` arrivals would drop
// it. This pins that the newest entries win regardless of arrival order.
func TestSearchLogs_LateArrivingNewerEntryDisplacesAnOlderOne(t *testing.T) {
	dir := t.TempDir()
	// Written first so it ends up OLDER by mtime, hence scanned LAST - yet it
	// holds the newest line of the whole set.
	writeLog(t, dir, "evey-2026-06-30.log", `{"time":"2026-07-01T09:00:00Z","level":"INFO","msg":"needle","component":"grpc"}`)
	time.Sleep(10 * time.Millisecond)
	writeLog(t, dir, "evey-2026-07-01.log", buildLogLines(5)...)

	res, err := searchLogs(dir, logSearchFilter{Q: "needle", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 {
		t.Fatalf("total = %d, want 6", res.Total)
	}
	if len(res.Entries) != 3 || res.Entries[0].Time != "2026-07-01T09:00:00Z" {
		t.Fatalf("newest line lost to arrival order: got %d entries, first=%q", len(res.Entries), res.Entries[0].Time)
	}
}

// The counterpart to TestSearchLogs_TruncatedOnlyMeansScanBudgetHit: the flag
// must still RISE when the scan really was cut short, otherwise the operator
// reads a lower-bound total as if it were exact.
func TestSearchLogs_TruncatedStillRisesOnScanBudget(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-01.log", buildLogLines(logScanLineBudget+10)...)

	// A filter nothing matches: the page never fills, so the scan runs to the
	// budget instead of stopping early.
	res, err := searchLogs(dir, logSearchFilter{Q: "absent", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("scan budget was hit but truncated stayed false - total would read as exact")
	}
}

// Four lines in five on a live instance are the gRPC interceptor's access log
// of heartbeats and dashboard polls. hide_access keeps the application log and
// the FAILED calls, which are the ones worth reading.
func TestSearchLogs_HideAccessKeepsFailuresAndAppLogs(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-09-03.log",
		`{"time":"2026-09-03T18:00:00Z","level":"INFO","msg":"grpc unary","component":"grpc","method":"/evey.v1.PresenceService/Heartbeat","status":"OK"}`,
		`{"time":"2026-09-03T18:00:01Z","level":"INFO","msg":"grpc stream","component":"grpc","method":"/evey.v1.X/Y","status":"OK"}`,
		`{"time":"2026-09-03T18:00:02Z","level":"INFO","msg":"grpc unary","component":"grpc","method":"/evey.v1.UserService/Login","status":"Unauthenticated","error":"bad password"}`,
		`{"time":"2026-09-03T18:00:03Z","level":"INFO","msg":"intro_detection: segment stored","media_id":"m1"}`,
	)

	all, err := searchLogs(dir, logSearchFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 4 {
		t.Fatalf("without the flag every line must come back, got %d", all.Total)
	}

	res, err := searchLogs(dir, logSearchFilter{Limit: 100, HideAccess: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("hide_access want 2 (app line + failed RPC), got %d: %+v", res.Total, res.Entries)
	}
	for _, e := range res.Entries {
		if st, _ := e.Fields["status"].(string); st == "OK" {
			t.Fatalf("a successful access line leaked through: %s", e.Raw)
		}
	}
}

func TestParseLogSearchFilter_HideAccess(t *testing.T) {
	for raw, want := range map[string]bool{"": false, "0": false, "1": true, "true": true, "yes": true} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/search?hide_access="+raw, nil)
		if got := parseLogSearchFilter(r).HideAccess; got != want {
			t.Fatalf("hide_access=%q: got %v want %v", raw, got, want)
		}
	}
}

// A search that matches nothing must still answer an array. The keeper's
// slice is only allocated by add(), so a miss serialised "entries":null, and
// both admin log screens read .length/.map off it: the page crashed on the
// first query that found nothing.
func TestSearchLogs_NoMatchEncodesAnEmptyArray(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "evey-2026-07-07.log",
		`{"time":"2026-07-07T11:00:00Z","level":"INFO","msg":"hls: ffmpeg start","media_id":"m1"}`,
	)
	res, err := searchLogs(dir, logSearchFilter{Limit: 100, Q: "no-such-needle"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"entries":[]`) {
		t.Fatalf("a miss must encode an empty array, got %s", b)
	}
}

// The early stop must return exactly what a full scan would, page by page:
// three rotated files whose boundaries overlap, lines a few milliseconds out
// of order inside a file (two loggers racing for the writer), every filter the
// explorer sends. A complete answer (no hasMore, no truncation) carries the
// exact total.
func TestSearchLogs_EarlyStopMatchesAFullScan(t *testing.T) {
	dir := t.TempDir()
	base, _ := time.Parse(time.RFC3339, "2026-07-01T00:00:00Z")
	levels := []string{"INFO", "INFO", "INFO", "WARN", "DEBUG", "ERROR"}
	var all []string
	for fi := range 3 {
		var lines []string
		// Each file starts 30 s before the previous one ended: the rotation
		// overlap the keeper already copes with.
		start := 300 + fi*3700
		for i := range 4000 {
			n := start + i
			ts := base.Add(time.Duration(n) * 100 * time.Millisecond)
			if i%7 == 3 {
				ts = ts.Add(-170 * time.Millisecond) // written after a newer line
			}
			if i%10 == 4 || i%10 == 5 {
				// Two lines in a row written after a newer one: the page can
				// settle on the second while the line above both still
				// belongs in it.
				ts = ts.Add(-350 * time.Millisecond)
			}
			msg, status := "app event", ""
			if n%5 == 0 {
				msg, status = "grpc unary", `,"status":"OK"`
			}
			lines = append(lines, fmt.Sprintf(`{"time":%q,"level":%q,"msg":%q,"media_id":"m%d"%s}`,
				ts.Format(time.RFC3339Nano), levels[n%len(levels)], msg, n%13, status))
		}
		p := writeLog(t, dir, fmt.Sprintf("evey-%d.log", fi), lines...)
		mt := base.Add(time.Duration(start+4000) * 100 * time.Millisecond)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		all = append(all, lines...)
	}
	full := func(f logSearchFilter) []logSearchEntry {
		var out []logSearchEntry
		for _, l := range all {
			if e, ok := parseLogLine(l); ok && matchLog(e, f) {
				out = append(out, e)
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339Nano, out[i].Time)
			tj, _ := time.Parse(time.RFC3339Nano, out[j].Time)
			return ti.After(tj)
		})
		return out
	}
	for _, f := range []logSearchFilter{
		{},
		{HideAccess: true},
		{Levels: map[string]bool{"ERROR": true}},
		{Levels: map[string]bool{"WARN": true, "ERROR": true}, HideAccess: true},
		{MediaID: "m7"},
	} {
		want := full(f)
		// Small pages at every offset put the page boundary on each kind of
		// line, the late-written ones included; large ones cross the files.
		type page struct{ off, limit int }
		var pages []page
		for off := range 40 {
			pages = append(pages, page{off, 3})
		}
		for _, off := range []int{0, 100, 250, 3900, 11000} {
			pages = append(pages, page{off, 100})
		}
		for _, pg := range pages {
			off := pg.off
			f.Offset, f.Limit = off, pg.limit
			res, err := searchLogs(dir, f)
			if err != nil {
				t.Fatal(err)
			}
			lo, hi := min(off, len(want)), min(off+pg.limit, len(want))
			if len(res.Entries) != hi-lo {
				t.Fatalf("%+v: %d entries, want %d", f, len(res.Entries), hi-lo)
			}
			for i, e := range res.Entries {
				if e.Raw != want[lo+i].Raw {
					t.Fatalf("%+v: entry %d = %s, want %s", f, i, e.Time, want[lo+i].Time)
				}
			}
			if !res.HasMore && !res.Truncated && res.Total != len(want) {
				t.Fatalf("%+v: complete answer with total %d, want %d", f, res.Total, len(want))
			}
			if res.HasMore && hi == len(want) && res.Total > len(want) {
				t.Fatalf("%+v: total %d past the %d matches", f, res.Total, len(want))
			}
		}
	}
}

// The level is read off the raw line before the JSON parse, upper-cased as
// parseLogLine sets it, so a rare level is not starved by the scan budget.
func TestLogLineLevel(t *testing.T) {
	if lv, ok := logLineLevel(`{"time":"x","level":"warn","msg":"m"}`); !ok || lv != "WARN" {
		t.Fatalf("got %q %v", lv, ok)
	}
	if _, ok := logLineLevel(`{"time":"x","msg":"m"}`); ok {
		t.Fatal("a line without a level must be left to the parse")
	}
}
