package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A request that carries every WebSocket header except the hop-by-hop pair
// (Connection: Upgrade, Upgrade: websocket) - what reaches the server when a
// proxy on the way strips them - is refused by the handshake with a 400, never
// upgraded, even with a valid token. The refusal is the server's, not the
// reverse proxy's, and there is nothing to relax: an upgrade nobody asked for
// cannot be performed.
func TestDevicesWS_WithoutUpgradeHeader_IsRefused400(t *testing.T) {
	fx := newDevicesFixture(t)
	srv := httptest.NewServer(fx.mux)
	defer srv.Close()

	send := func(upgrade bool) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/devices/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Protocol", "Bearer."+fx.userTok)
		req.Header.Set("Origin", srv.URL)
		if upgrade {
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
		} else {
			req.Header.Set("Connection", "keep-alive")
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := send(true); got != http.StatusSwitchingProtocols {
		t.Fatalf("control with Upgrade: status %d, want 101", got)
	}
	if got := send(false); got != http.StatusBadRequest {
		t.Fatalf("without Upgrade: status %d, want 400", got)
	}
}
