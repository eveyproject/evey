package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	domainsyncplay "github.com/evey-project/evey/internal/domain/syncplay"
	syncplayfakes "github.com/evey-project/evey/internal/domain/syncplay/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
)

type devicesFixture struct {
	mux      *http.ServeMux
	registry *appsyncplay.DeviceRegistry
	userTok  string
	otherTok string
}

func newDevicesFixture(t *testing.T) *devicesFixture {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1"})
	otherTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-2"})

	registry := appsyncplay.NewDeviceRegistry()
	mux := http.NewServeMux()
	registerDeviceRoutes(mux, registry, gen)
	return &devicesFixture{mux: mux, registry: registry, userTok: userTok, otherTok: otherTok}
}

func TestDevices_List_NoAuth_401(t *testing.T) {
	f := newDevicesFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDevices_List_OnlyOwnDevices(t *testing.T) {
	f := newDevicesFixture(t)
	_, _ = f.registry.Register("user-1", "dev-tv", "Living room TV", domainsyncplay.DeviceTypeTV, syncplayfakes.NewFakeMessageSender())
	_, _ = f.registry.Register("user-1", "dev-phone", "Pixel", domainsyncplay.DeviceTypeMobile, syncplayfakes.NewFakeMessageSender())
	// user-2's device must never appear in user-1's list.
	_, _ = f.registry.Register("user-2", "dev-other", "Other", domainsyncplay.DeviceTypeWeb, syncplayfakes.NewFakeMessageSender())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Devices []struct {
			ID, Name, Type string
		} `json:"devices"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(resp.Devices))
	}
	// Sorted by name: "Living room TV" < "Pixel".
	if resp.Devices[0].Name != "Living room TV" || resp.Devices[0].ID != "dev-tv" {
		t.Errorf("unexpected first device: %+v", resp.Devices[0])
	}
}

func TestDevices_Command_ReachesDevice(t *testing.T) {
	f := newDevicesFixture(t)
	sender := syncplayfakes.NewFakeMessageSender()
	_, _ = f.registry.Register("user-1", "dev-tv", "TV", domainsyncplay.DeviceTypeTV, sender)

	body := `{"type":"loadAndPlay","mediaId":"media-42","positionMs":90000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/dev-tv/command", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	got := sender.Received()
	if len(got) != 1 {
		t.Fatalf("expected 1 pushed command, got %d", len(got))
	}
	var cmd domainsyncplay.DeviceCommand
	if err := json.Unmarshal(got[0], &cmd); err != nil {
		t.Fatalf("decode pushed command: %v", err)
	}
	if cmd.Type != domainsyncplay.DeviceCmdLoadAndPlay || cmd.MediaID != "media-42" || cmd.PositionMs != 90000 {
		t.Errorf("unexpected command pushed: %+v", cmd)
	}
}

func TestDevices_Command_UnknownDevice_404(t *testing.T) {
	f := newDevicesFixture(t)
	_, _ = f.registry.Register("user-1", "dev-tv", "TV", domainsyncplay.DeviceTypeTV, syncplayfakes.NewFakeMessageSender())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/dev-missing/command",
		strings.NewReader(`{"type":"pause"}`))
	req.Header.Set("Authorization", "Bearer "+f.userTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown device, got %d", w.Code)
	}
}

// A caller must not be able to command another user's device — the
// registry is per-user, so user-2 addressing user-1's device gets a 404.
func TestDevices_Command_OtherUsersDevice_404(t *testing.T) {
	f := newDevicesFixture(t)
	_, _ = f.registry.Register("user-1", "dev-tv", "TV", domainsyncplay.DeviceTypeTV, syncplayfakes.NewFakeMessageSender())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/dev-tv/command",
		strings.NewReader(`{"type":"pause"}`))
	req.Header.Set("Authorization", "Bearer "+f.otherTok)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 cross-user, got %d", w.Code)
	}
}

func TestDevices_Command_BadBody_400(t *testing.T) {
	f := newDevicesFixture(t)
	_, _ = f.registry.Register("user-1", "dev-tv", "TV", domainsyncplay.DeviceTypeTV, syncplayfakes.NewFakeMessageSender())

	for _, body := range []string{`not json`, `{}`, `{"mediaId":"x"}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/dev-tv/command", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+f.userTok)
		w := httptest.NewRecorder()
		f.mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, w.Code)
		}
	}
}
