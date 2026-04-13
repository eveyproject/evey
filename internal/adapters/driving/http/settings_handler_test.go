package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

type fakeSettingsStore struct {
	mu   sync.Mutex
	sets map[string]string
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{sets: make(map[string]string)}
}

func (f *fakeSettingsStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sets[key], nil
}

func (f *fakeSettingsStore) Set(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets[key] = value
	return nil
}

type settingsTestEnv struct {
	mux      *http.ServeMux
	cfg      *config.Config
	store    *fakeSettingsStore
	loginRL  *ratelimit.Limiter
	globalRL *ratelimit.Limiter
	writeRL  *ratelimit.Limiter
}

func (e *settingsTestEnv) cleanup() {
	e.loginRL.Stop()
	e.globalRL.Stop()
	e.writeRL.Stop()
}

func newSettingsTestEnv(t *testing.T, verifier TokenVerifier) *settingsTestEnv {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.Name = "Evey"
	cfg.Server.LoginRateLimitMax = 10
	cfg.Server.GlobalRateLimitMax = 300
	cfg.Transcoding.MaxConcurrent = 2
	cfg.Transcoding.HWAccelEnabled = false

	env := &settingsTestEnv{
		mux:      http.NewServeMux(),
		cfg:      cfg,
		store:    newFakeSettingsStore(),
		loginRL:  ratelimit.New(10),
		globalRL: ratelimit.New(300),
		writeRL:  ratelimit.New(1000),
	}
	registerServerSettingsRoutes(env.mux, env.cfg, env.store, verifier, nil, nil, env.loginRL, env.globalRL, env.writeRL)
	t.Cleanup(env.cleanup)
	return env
}

func TestServerSettings_Get_Admin(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]settingValue
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"serverName", "maxConcurrentTranscodes", "hwAccelEnabled", "loginRateLimit", "globalRateLimit"} {
		v, ok := resp[k]
		if !ok {
			t.Errorf("missing key %s", k)
			continue
		}
		if v.Source != "default" {
			t.Errorf("%s: expected source default, got %s", k, v.Source)
		}
	}
	if resp["serverName"].Value != "Evey" {
		t.Errorf("unexpected serverName value: %v", resp["serverName"].Value)
	}
}

func TestServerSettings_Get_NonAdmin_Forbidden(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(false))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestServerSettings_Get_NoToken_Unauthorized(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestServerSettings_Put_Admin_PartialBody(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	body := strings.NewReader(`{"serverName":"My Evey"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", body)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Server.Name != "My Evey" {
		t.Errorf("cfg.Server.Name = %q want %q", env.cfg.Server.Name, "My Evey")
	}
	if got := env.store.sets[cfgKeyServerName]; got != "My Evey" {
		t.Errorf("store[%s] = %q want %q", cfgKeyServerName, got, "My Evey")
	}
	var resp map[string]settingValue
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["serverName"].Source != "override" {
		t.Errorf("expected serverName source override, got %s", resp["serverName"].Source)
	}
}

func TestServerSettings_Put_InvalidBody_BadRequest(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", strings.NewReader(`not-json`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestServerSettings_Put_NonAdmin_Forbidden(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(false))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", strings.NewReader(`{"serverName":"x"}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestServerSettings_Put_MaxConcurrent_NilTranscodingSafe(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", strings.NewReader(`{"maxConcurrentTranscodes":7}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Transcoding.MaxConcurrent != 7 {
		t.Errorf("cfg.Transcoding.MaxConcurrent = %d want 7", env.cfg.Transcoding.MaxConcurrent)
	}
	if got := env.store.sets[cfgKeyTranscodeMaxConc]; got != "7" {
		t.Errorf("store[%s] = %q want %q", cfgKeyTranscodeMaxConc, got, "7")
	}
}

func TestServerSettings_Put_RateLimits_UpdatesLimiters(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"loginRateLimit":42,"globalRateLimit":99,"hwAccelEnabled":true}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Server.LoginRateLimitMax != 42 {
		t.Errorf("LoginRateLimitMax = %d want 42", env.cfg.Server.LoginRateLimitMax)
	}
	if env.cfg.Server.GlobalRateLimitMax != 99 {
		t.Errorf("GlobalRateLimitMax = %d want 99", env.cfg.Server.GlobalRateLimitMax)
	}
	if !env.cfg.Transcoding.HWAccelEnabled {
		t.Errorf("HWAccelEnabled should be true")
	}
	if got := env.store.sets[cfgKeyRateLimitLogin]; got != "42" {
		t.Errorf("store login = %q", got)
	}
	if got := env.store.sets[cfgKeyRateLimitGlobal]; got != "99" {
		t.Errorf("store global = %q", got)
	}
	if got := env.store.sets[cfgKeyTranscodeHWAccel]; got != "true" {
		t.Errorf("store hwaccel = %q", got)
	}
}
