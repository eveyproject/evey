package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const tvRefusal = `A playback exception has occurred: Source error Invalid integer size: 30 {contentIsMalformed=true, dataType=1}`

func postRefusal(t *testing.T, body string) (recorded []string) {
	t.Helper()
	rec := func(_ context.Context, id string) error {
		recorded = append(recorded, id)
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(body))
	req.SetPathValue("id", "media-1")
	w := httptest.NewRecorder()
	playerEventHandler(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), WithPlayerEventDirectPlayRefusal(rec))(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	return recorded
}

// A television's demuxer refused the raw file: the catalog keeps it, so no
// later device is handed that file whole.
func TestPlayerEvent_DirectPlayRefusal_RecordedOnTheFile(t *testing.T) {
	got := postRefusal(t, `{"type":"native_direct_play_refused","details":"`+tvRefusal+`","diag":{"playMethod":"direct_play"}}`)
	if len(got) != 1 || got[0] != "media-1" {
		t.Fatalf("recorded = %v, want [media-1]", got)
	}
}

// The server judges the refusal itself: a beacon is only a claim.
func TestPlayerEvent_DirectPlayRefusal_JudgedByTheServer(t *testing.T) {
	for name, body := range map[string]string{
		"not out of a direct play": `{"type":"native_direct_play_refused","details":"` + tvRefusal + `","diag":{"playMethod":"direct_stream"}}`,
		"no container parse error": `{"type":"native_direct_play_refused","details":"Source error Response code: 410","diag":{"playMethod":"direct_play"}}`,
		"a manifest parse error":   `{"type":"native_direct_play_refused","details":"Source error {contentIsMalformed=true, dataType=4}","diag":{"playMethod":"direct_play"}}`,
		"no diagnosis":             `{"type":"native_direct_play_refused","details":"` + tvRefusal + `"}`,
		"another event":            `{"type":"native_player_error","details":"` + tvRefusal + `","diag":{"playMethod":"direct_play"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := postRefusal(t, body); len(got) != 0 {
				t.Fatalf("recorded = %v, want nothing", got)
			}
		})
	}
}
