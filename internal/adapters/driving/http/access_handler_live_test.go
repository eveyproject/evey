package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/pkg/network"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// The two proxy facts used to be read once from the environment, so the
// panel's look-alikes (External URL, LAN Networks) decided neither: an https
// External URL left cookies without Secure, and a proxy range could only be
// trusted by editing the compose file and restarting. Both now come from the
// config on every request; these tests drive them through the panel's PUT.

func setupLiveRemoteAccess(t *testing.T) (*http.ServeMux, *config.Config, string) {
	mux, cfg, tok, _ := setupLiveRemoteAccessStore(t)
	return mux, cfg, tok
}

func setupLiveRemoteAccessStore(t *testing.T) (*http.ServeMux, *config.Config, string, *fakeRemoteAccessStore) {
	t.Helper()
	gen, adminTok, _ := genTokens(t)
	envProxies, err := config.NormalizeTrustedProxies(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Server: config.ServerConfig{BaseURL: "http://localhost:9091", TrustedProxies: envProxies},
	}
	store := &fakeRemoteAccessStore{data: map[string]string{}}
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store, ratelimit.New(600))
	return mux, cfg, adminTok, store
}

func putRemoteAccess(t *testing.T, mux *http.ServeMux, tok string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestTrustedProxiesFromThePanelApplyToTheNextRequest(t *testing.T) {
	mux, cfg, tok, store := setupLiveRemoteAccessStore(t)
	trust := network.NewLiveProxyTrust(cfg.EffectiveTrustedProxies, "X-Forwarded-For", cfg.EffectiveHTTPS)

	viaProxy := func() string {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "172.18.0.5:41234"
		r.Header.Set("X-Forwarded-For", "203.0.113.9")
		return trust.ResolveClientIP(r)
	}
	if got := viaProxy(); got != "172.18.0.5" {
		t.Fatalf("before: client seen as %s, want the untrusted proxy 172.18.0.5", got)
	}

	if w := putRemoteAccess(t, mux, tok, map[string]any{"trustedProxies": []string{"172.18.0.0/16"}}); w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if got := viaProxy(); got != "203.0.113.9" {
		t.Fatalf("after: client seen as %s, want the forwarded 203.0.113.9", got)
	}
	// The boot reloads the saved row into cfg.RemoteAccess (cmd/evey/main.go):
	// a list missing from it would be live today and forgotten at the restart.
	var reloaded config.RemoteAccessConfig
	if err := json.Unmarshal([]byte(store.data[RemoteAccessConfigKey]), &reloaded); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.TrustedProxies) != 1 || reloaded.TrustedProxies[0] != "172.18.0.0/16" {
		t.Fatalf("persisted list %v, want [172.18.0.0/16]", reloaded.TrustedProxies)
	}

	if w := putRemoteAccess(t, mux, tok, map[string]any{"clearTrustedProxies": true}); w.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", w.Code, w.Body.String())
	}
	if got := viaProxy(); got != "172.18.0.5" {
		t.Fatalf("after clearing: client seen as %s, want the environment's answer again", got)
	}
}

func TestTrustedProxiesRefuseACatchAllRange(t *testing.T) {
	mux, cfg, tok := setupLiveRemoteAccess(t)
	for _, entry := range []string{"0.0.0.0/0", "::/0", "not-a-cidr"} {
		if w := putRemoteAccess(t, mux, tok, map[string]any{"trustedProxies": []string{entry}}); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", entry, w.Code)
		}
	}
	if list, fromPanel := cfg.TrustedProxiesView(); fromPanel || len(list) != 0 {
		t.Fatalf("a refused list was stored: %v (from panel %v)", list, fromPanel)
	}
}

func TestAnHTTPSExternalURLSecuresCookiesWithoutRestart(t *testing.T) {
	mux, cfg, tok := setupLiveRemoteAccess(t)
	h := CSRFMiddlewareLive(cfg.EffectiveHTTPS, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	secure := func() bool {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		for _, c := range w.Result().Cookies() {
			if c.Name == csrfCookieName {
				return c.Secure
			}
		}
		t.Fatal("no CSRF cookie set")
		return false
	}
	if secure() {
		t.Fatal("before: Secure cookie on an http-only setup")
	}
	if w := putRemoteAccess(t, mux, tok, map[string]any{"externalUrl": "https://media.example.com"}); w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if !secure() {
		t.Fatal("after an https External URL: cookie still not Secure")
	}
}
