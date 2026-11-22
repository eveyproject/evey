package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// The decoder check a TV posts reaches the memory the stream verdict reads,
// keyed by its user and its device.
func TestPlayerEvent_DecoderCheckFeedsTheMemory(t *testing.T) {
	const device = "EveyTV/0.2.11 (Android TV 7.1.1; Freebox Player Mini v2)"
	mem := apptranscoding.NewDeviceDecoderMemory()
	body := `{"type":"ac3go_check","details":"declared=ac3,eac3 blocked=no rtf=0.246 wall=1089ms"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(body))
	req.SetPathValue("id", "media-1")
	req.Header.Set("User-Agent", device)
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "usr-1"})
	w := httptest.NewRecorder()
	playerEventHandler(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), WithPlayerEventDecoderMemory(mem))(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}

	if _, added := mem.Complete("usr-1", device, []string{"aac", "opus"}); strings.Join(added, ",") != "ac3,eac3" {
		t.Fatalf("added = %v, want ac3,eac3", added)
	}
}
