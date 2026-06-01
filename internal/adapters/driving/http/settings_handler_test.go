package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/config"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// fakeHWModeController records SetHWMode calls so the settings test can assert
// the live re-bench was triggered with the parsed mode.
type fakeHWModeController struct {
	mu   sync.Mutex
	mode domaintranscoding.HWMode
	sets []domaintranscoding.HWMode
}

func (f *fakeHWModeController) HWMode() domaintranscoding.HWMode {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mode == "" {
		return domaintranscoding.HWModeAuto
	}
	return f.mode
}

func (f *fakeHWModeController) SetHWMode(_ context.Context, mode domaintranscoding.HWMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
	f.sets = append(f.sets, mode)
	return nil
}

func (f *fakeHWModeController) calls() []domaintranscoding.HWMode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domaintranscoding.HWMode(nil), f.sets...)
}

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
	hwMode   *fakeHWModeController
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
		hwMode:   &fakeHWModeController{},
		loginRL:  ratelimit.New(10),
		globalRL: ratelimit.New(300),
		writeRL:  ratelimit.New(1000),
	}
	registerServerSettingsRoutes(env.mux, env.cfg, env.store, verifier, nil, nil, env.hwMode, env.loginRL, env.globalRL, env.writeRL)
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

func TestServerSettings_Get_IncludesHWMode(t *testing.T) {
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
	v, ok := resp["hwMode"]
	if !ok {
		t.Fatalf("missing hwMode in response")
	}
	// Default env (accel off) resolves to off via the fake controller's auto
	// default only when the controller is asked; here the controller reports
	// auto, so the GET surfaces a concrete mode (never empty).
	if v.Value == "" || v.Value == nil {
		t.Errorf("hwMode value must be a concrete mode, got %v", v.Value)
	}
}

func TestServerSettings_Put_HWMode_PersistsAndRebenches(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	body := strings.NewReader(`{"hwMode":"vaapi"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", body)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	// Synchronous effects: configured intent + legacy master gate + persistence.
	if env.cfg.Transcoding.HWMode != "vaapi" {
		t.Errorf("cfg.Transcoding.HWMode = %q want vaapi", env.cfg.Transcoding.HWMode)
	}
	if !env.cfg.Transcoding.HWAccelEnabled {
		t.Errorf("a non-off mode must enable the legacy HW gate")
	}
	if got := env.store.sets[cfgKeyTranscodeHWMode]; got != "vaapi" {
		t.Errorf("store[%s] = %q want vaapi", cfgKeyTranscodeHWMode, got)
	}
	// Async effect: the live re-bench is fired detached. Poll briefly.
	var sawVAAPI bool
	for i := 0; i < 50 && !sawVAAPI; i++ {
		for _, m := range env.hwMode.calls() {
			if m == domaintranscoding.HWModeVAAPI {
				sawVAAPI = true
			}
		}
		if !sawVAAPI {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !sawVAAPI {
		t.Errorf("SetHWMode(vaapi) was not invoked on the controller")
	}
}

func TestServerSettings_Put_HWMode_Off_DisablesGate(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	env.cfg.Transcoding.HWAccelEnabled = true // start enabled
	body := strings.NewReader(`{"hwMode":"off"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", body)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Transcoding.HWAccelEnabled {
		t.Errorf("mode=off must disable the legacy HW gate")
	}
	if got := env.store.sets[cfgKeyTranscodeHWMode]; got != "off" {
		t.Errorf("store[%s] = %q want off", cfgKeyTranscodeHWMode, got)
	}
}

func TestServerSettings_Put_HWMode_Invalid_Ignored(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	body := strings.NewReader(`{"hwMode":"bogus"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", body)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := env.store.sets[cfgKeyTranscodeHWMode]; ok {
		t.Errorf("an unknown mode must not be persisted")
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
