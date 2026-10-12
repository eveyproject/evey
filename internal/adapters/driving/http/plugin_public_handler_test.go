package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// newPluginPublicTestMux sets up mux, admin token and user token for public plugin route tests.
func newPluginPublicTestMux(t *testing.T, repo domainplugin.PluginRepository) (mux *http.ServeMux, adminToken, userToken string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})
	mux = http.NewServeMux()
	registerPluginPublicRoutes(mux, gen, repo)
	return mux, adminToken, userToken
}

// runningPlugin returns an enabled, running plugin with frontend module.
func runningPlugin() *domainplugin.Plugin {
	return &domainplugin.Plugin{
		ID:      "weather-1",
		Name:    "Weather",
		Version: "1.0.0",
		Kind:    domainplugin.KindPlugin,
		WebRoot: "dist",
		Frontend: []domainplugin.FrontModule{
			{Slot: "home-widget", Src: "dist/index.js"},
		},
		ExternalFetch: []string{"api.open-meteo.com"},
		Enabled:       true,
		Status:        domainplugin.Running,
		// api_key is a password field + secret_url is flagged secret: both must
		// be withheld from the public/iframe config; city/units are exposed.
		ConfigSchema: map[string]any{
			"city":       map[string]any{"type": "string"},
			"units":      map[string]any{"type": "select"},
			"api_key":    map[string]any{"type": "password"},
			"secret_url": map[string]any{"type": "string", "secret": true},
		},
		Config: map[string]any{"city": "Paris", "units": "metric", "api_key": "SECRET123", "secret_url": "http://10.0.0.5/internal"},
		// Admin-only fields that must NOT appear in public responses:
		SourceHash:   "deadbeef",
		Language:     "js",
		Entrypoint:   "server.js",
		ErrorMessage: "",
		Permissions:  []string{"network"},
		Capabilities: []string{"weather"},
	}
}

// TestPluginPublicList_NoAuth_401 verifies unauthenticated requests are rejected.
func TestPluginPublicList_NoAuth_401(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	mux, _, _ := newPluginPublicTestMux(t, repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestPluginPublicList_UserToken_200 verifies non-admin users can list enabled+running plugins.
func TestPluginPublicList_UserToken_200(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	p := runningPlugin()
	if err := repo.Create(t.Context(), p); err != nil {
		t.Fatal(err)
	}

	mux, _, userToken := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(resp.Plugins))
	}
	got := resp.Plugins[0]

	// Verify public fields are present.
	for _, field := range []string{"id", "name", "version", "kind", "webRoot", "frontend", "externalFetch"} {
		if _, ok := got[field]; !ok {
			t.Errorf("expected field %q to be present", field)
		}
	}

	// Verify sensitive fields are ABSENT.
	for _, field := range []string{
		"analysisReport", "sourceHash", "state", "errorMessage",
		"entryPoint", "language", "installedAt", "permissions", "capabilities",
	} {
		if _, ok := got[field]; ok {
			t.Errorf("sensitive field %q MUST NOT appear in public response", field)
		}
	}
}

// TestPluginPublicList_AdminToken_200 verifies admin users also get the public list (rétrocompat).
func TestPluginPublicList_AdminToken_200(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	if err := repo.Create(t.Context(), runningPlugin()); err != nil {
		t.Fatal(err)
	}

	mux, adminToken, _ := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestPluginPublicList_DisabledPlugin_Excluded verifies disabled plugins do not appear.
func TestPluginPublicList_DisabledPlugin_Excluded(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	disabled := runningPlugin()
	disabled.ID = "disabled-plugin"
	disabled.Enabled = false
	if err := repo.Create(t.Context(), disabled); err != nil {
		t.Fatal(err)
	}

	mux, _, userToken := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Plugins) != 0 {
		t.Errorf("expected 0 plugins (disabled excluded), got %d", len(resp.Plugins))
	}
}

// TestPluginPublicList_NotRunning_Excluded verifies plugins with state != running are excluded.
func TestPluginPublicList_NotRunning_Excluded(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()

	for _, status := range []domainplugin.PluginStatus{
		domainplugin.Stopped,
		domainplugin.Initializing,
		domainplugin.Error,
		domainplugin.Installed,
	} {
		p := runningPlugin()
		p.ID = "plugin-" + string(status)
		p.Status = status
		if err := repo.Create(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}

	mux, _, userToken := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Plugins) != 0 {
		t.Errorf("expected 0 plugins (non-running excluded), got %d", len(resp.Plugins))
	}
}

// TestPluginPublicConfig_NoAuth_401 verifies unauthenticated config requests are rejected.
func TestPluginPublicConfig_NoAuth_401(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	mux, _, _ := newPluginPublicTestMux(t, repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/weather-1/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestPluginPublicConfig_UserToken_200 verifies non-admin can read config for enabled+running plugin.
func TestPluginPublicConfig_UserToken_200(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	if err := repo.Create(t.Context(), runningPlugin()); err != nil {
		t.Fatal(err)
	}

	mux, _, userToken := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/weather-1/config", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Values map[string]any `json:"values"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Values["city"] != "Paris" {
		t.Errorf("expected city=Paris, got %v", resp.Values["city"])
	}
	if resp.Values["units"] != "metric" {
		t.Errorf("expected units=metric, got %v", resp.Values["units"])
	}
	// Secrets must never reach the non-admin / iframe-facing endpoint.
	if _, ok := resp.Values["api_key"]; ok {
		t.Errorf("password field api_key leaked to public config: %v", resp.Values["api_key"])
	}
	if _, ok := resp.Values["secret_url"]; ok {
		t.Errorf("secret-flagged field secret_url leaked to public config: %v", resp.Values["secret_url"])
	}
}

// TestPluginPublicConfig_DisabledPlugin_404 verifies disabled plugin config returns 404.
func TestPluginPublicConfig_DisabledPlugin_404(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	p := runningPlugin()
	p.Enabled = false
	if err := repo.Create(t.Context(), p); err != nil {
		t.Fatal(err)
	}

	mux, _, userToken := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/weather-1/config", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for disabled plugin, got %d", w.Code)
	}
}

// TestPluginPublicConfig_NotRunning_404 verifies plugin with state != running config returns 404.
func TestPluginPublicConfig_NotRunning_404(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	p := runningPlugin()
	p.Status = domainplugin.Stopped
	if err := repo.Create(t.Context(), p); err != nil {
		t.Fatal(err)
	}

	mux, _, userToken := newPluginPublicTestMux(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/weather-1/config", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for stopped plugin, got %d", w.Code)
	}
}
