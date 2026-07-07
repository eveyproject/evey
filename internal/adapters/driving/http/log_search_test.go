package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
			where, source := classifyLog(c.component, c.msg)
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
