package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The 2026-07-15 incident: a viewer watched a rainbow-garbled screen for
// minutes, then an endless spinner - and the server logs carried NOT ONE line
// about it. Everything the player knew (decode errors, recovery attempts,
// element-level MediaError) stayed in the browser console. This beacon is how
// a client-side playback failure becomes a server-side, searchable log line.
func postPlayerEvent(t *testing.T, logger *slog.Logger, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(body))
	req.SetPathValue("id", "media-1")
	rec := httptest.NewRecorder()
	playerEventHandler(logger)(rec, req)
	return rec
}

func TestPlayerEvent_LogsAClientErrorWithItsContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	rec := postPlayerEvent(t, logger, `{
		"type": "hls_media_error",
		"details": "bufferAppendError",
		"fatal": true,
		"code": 3,
		"variant": 2,
		"segment": 1023,
		"posSec": 3066.5
	}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("no parseable log line was written: %v (raw=%q)", err, buf.String())
	}
	for field, want := range map[string]any{
		"media_id":  "media-1",
		"component": "player_client",
		"type":      "hls_media_error",
		"details":   "bufferAppendError",
		"fatal":     true,
		"level":     "WARN",
	} {
		if got := line[field]; got != want {
			t.Fatalf("log field %q = %v, want %v (line=%v)", field, got, want, line)
		}
	}
	if got := line["seg"]; got != float64(1023) {
		t.Fatalf("log field seg = %v, want 1023", line["seg"])
	}
}

// Garbage in must never crash or pollute: no type -> 400, an unparseable body
// -> 400, and free-text fields are clamped so a hostile client cannot pump
// megabytes into the log files through this door.
func TestPlayerEvent_RejectsAndClamps(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	if rec := postPlayerEvent(t, logger, `{"details":"no type"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing type: status = %d, want 400", rec.Code)
	}
	if rec := postPlayerEvent(t, logger, `{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: status = %d, want 400", rec.Code)
	}
	if rec := postPlayerEvent(t, logger, `{"type":"weird type; DROP TABLE"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("type outside [a-z0-9_]: status = %d, want 400", rec.Code)
	}
	if rec := postPlayerEvent(t, logger, `{"type":"x","details":"`+strings.Repeat("A", 5000)+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized body: status = %d, want 400", rec.Code)
	}

	buf.Reset()
	long := strings.Repeat("d", 500)
	if rec := postPlayerEvent(t, logger, `{"type":"hls_fatal","details":"`+long+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("long-but-in-budget details: status = %d, want 204", rec.Code)
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line: %v", err)
	}
	if details, _ := line["details"].(string); len(details) > 200 {
		t.Fatalf("details logged at %d chars, want clamped to 200", len(details))
	}
}

// The admin log search must file these under where=player: that is where an
// operator investigating a playback incident already looks.
func TestClassify_PlayerClientComponentIsPlayer(t *testing.T) {
	where, _ := classifyLog("player_client", "player-client: playback error reported", "")
	if where != wherePlayer {
		t.Fatalf("where = %q, want %q", where, wherePlayer)
	}
}
