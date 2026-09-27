package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A direct-play request leaves one line saying who read which range, with
// what status and how many bytes - the trace the 2026-09-27 silent E-AC-3
// film never left.
func TestServeFileLoggedRecordsTheRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.Header.Set("Range", "bytes=100-299")
	req.Header.Set("User-Agent", "Evey/0.0.1-beta.69 (Linux;Android 14) AndroidXMedia3/1.8.0")
	rec := httptest.NewRecorder()
	serveFileLogged(rec, req, "m1", path)

	if rec.Code != http.StatusPartialContent || rec.Body.Len() != 200 {
		t.Fatalf("served %d with %d bytes, want 206 with 200", rec.Code, rec.Body.Len())
	}
	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log line %q: %v", logs.String(), err)
	}
	want := map[string]any{
		"msg": "stream: direct play served", "component": "stream", "media_id": "m1",
		"range": "bytes=100-299", "status": float64(206), "bytes": float64(200),
		"user_agent": "Evey/0.0.1-beta.69 (Linux;Android 14) AndroidXMedia3/1.8.0",
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v", k, line[k], v)
		}
	}
}

// A request the file server refuses is logged with its status and no bytes.
func TestServeFileLoggedRecordsARefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.Header.Set("Range", "bytes=900-999")
	serveFileLogged(httptest.NewRecorder(), req, "m1", path)

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log line %q: %v", logs.String(), err)
	}
	if line["status"] != float64(http.StatusRequestedRangeNotSatisfiable) {
		t.Fatalf("status %v, want 416", line["status"])
	}
}

// The master, its segments and the stream info read ?audioCodecs through the
// same correction: a silent-E-AC-3 build asking with its player's user agent
// loses eac3, so the decision transcodes the track it would have played mute.
func TestParseClientCapabilitiesDropsEAC3ForTheSilentBuilds(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?audioCodecs=aac,ac3,eac3", nil)
	req.Header.Set("User-Agent", "Evey/0.0.1-beta.69 (Linux;Android 14) AndroidXMedia3/1.8.0")
	if got := parseClientCapabilities(req, false).AudioCodecs; strings.Join(got, ",") != "aac,ac3" {
		t.Fatalf("beta.69 player: %v, want aac,ac3", got)
	}
	req.Header.Set("User-Agent", "Evey/0.0.1-beta.70 (Linux;Android 14) AndroidXMedia3/1.8.0")
	if got := parseClientCapabilities(req, false).AudioCodecs; strings.Join(got, ",") != "aac,ac3,eac3" {
		t.Fatalf("beta.70 player: %v, want aac,ac3,eac3", got)
	}
}
