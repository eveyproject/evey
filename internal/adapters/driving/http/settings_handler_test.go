package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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
	mu      sync.Mutex
	mode    domaintranscoding.HWMode
	sets    []domaintranscoding.HWMode
	av1Max  int
	av1Sets []int
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

func (f *fakeHWModeController) SoftwareAV1MaxOverride() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.av1Max
}

func (f *fakeHWModeController) SetSoftwareAV1MaxOverride(h int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.av1Max = h
	f.av1Sets = append(f.av1Sets, h)
}

type fakeSettingsStore struct {
	mu        sync.Mutex
	sets      map[string]string
	deleteErr error
	setErr    error
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
	if f.setErr != nil {
		return f.setErr
	}
	f.sets[key] = value
	return nil
}

func (f *fakeSettingsStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.sets, key)
	return nil
}

// A Delete failure on Reset must leave BOTH the live config and the store row
// on the override - never a live-vs-persisted divergence that a restart would
// undo. This pins the delete-first ordering.
func TestServerSettings_Put_ResetDeleteFailure_KeepsOverride(t *testing.T) {
	store := newFakeSettingsStore()
	store.sets[cfgKeyQoERetention] = "14"
	store.deleteErr = errors.New("db down")
	env := newSettingsTestEnvWithStore(t, validUserValidator(true), store)
	env.cfg.Metrics.QoERetentionDays = 14

	clear := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"qoeRetentionDays":null}`))
	clear.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, clear)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear PUT = %d", rec.Code)
	}
	// The Delete failed, so the override must survive on BOTH sides.
	if env.cfg.Metrics.QoERetentionDays != 14 {
		t.Errorf("cfg must keep the override when the store delete fails, got %d", env.cfg.Metrics.QoERetentionDays)
	}
	if env.store.sets[cfgKeyQoERetention] != "14" {
		t.Errorf("store row must survive a failed delete")
	}
}

// The Set twin of the Delete test above: a persist failure must NOT be answered
// 200 with `source: "override"`. The setting applies live and then vanishes at
// the next restart, so a success response is a lie the operator only discovers
// after a reboot.
func TestServerSettings_Put_PersistFailure_ReportsError(t *testing.T) {
	store := newFakeSettingsStore()
	store.setErr = errors.New("db down")
	env := newSettingsTestEnvWithStore(t, validUserValidator(true), store)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"qoeRetentionDays":21}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT with a failing store = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), cfgKeyQoERetention) {
		t.Errorf("the response must name the key that could not be saved, got %q", rec.Body.String())
	}
	// Nothing was written, and the live value did change - that is the state the
	// 500 is reporting, not a rollback.
	if _, ok := env.store.sets[cfgKeyQoERetention]; ok {
		t.Errorf("store must hold nothing after a failed Set")
	}
	if env.cfg.Metrics.QoERetentionDays != 21 {
		t.Errorf("live value = %d, want the applied 21", env.cfg.Metrics.QoERetentionDays)
	}
}

// A successful PUT must still answer 200 with the settings payload - the 500
// path above must not fire when the store is healthy.
func TestServerSettings_Put_PersistOK_StillReturnsSettings(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"qoeRetentionDays":21}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if env.store.sets[cfgKeyQoERetention] != "21" {
		t.Errorf("store = %q, want 21", env.store.sets[cfgKeyQoERetention])
	}
}

// An empty serverName used to blank the live name and then skip the persist,
// because "" doubles as the loop's "field absent" signal. The name must survive
// untouched, and nothing must be written.
func TestServerSettings_Put_EmptyServerName_KeepsCurrentName(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	env.cfg.Server.Name = "Salon"

	for _, body := range []string{`{"serverName":""}`, `{"serverName":"   "}`} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		env.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s = %d", body, rec.Code)
		}
		if env.cfg.Server.Name != "Salon" {
			t.Errorf("PUT %s emptied the live server name: %q", body, env.cfg.Server.Name)
		}
		if _, ok := env.store.sets[cfgKeyServerName]; ok {
			t.Errorf("PUT %s persisted an empty name", body)
		}
	}
}

// A real rename still works, and is trimmed on the way in.
func TestServerSettings_Put_ServerName_TrimsAndPersists(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"serverName":"  Salon  "}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d", rec.Code)
	}
	if env.cfg.Server.Name != "Salon" {
		t.Errorf("live name = %q, want %q", env.cfg.Server.Name, "Salon")
	}
	if env.store.sets[cfgKeyServerName] != "Salon" {
		t.Errorf("store = %q, want %q", env.store.sets[cfgKeyServerName], "Salon")
	}
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
	registerServerSettingsRoutes(env.mux, env.cfg, env.store, verifier, nil, env.hwMode, env.loginRL, env.globalRL, env.writeRL)
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

func TestServerSettings_Put_SoftwareAV1MaxHeight_PersistsAndApplies(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"softwareAV1MaxHeight":720}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Transcoding.SoftwareAV1MaxHeightOverride != 720 {
		t.Errorf("cfg override = %d want 720", env.cfg.Transcoding.SoftwareAV1MaxHeightOverride)
	}
	if got := env.store.sets[cfgKeyTranscodeSWAV1]; got != "720" {
		t.Errorf("store[%s] = %q want 720", cfgKeyTranscodeSWAV1, got)
	}
	// Applied live on the controller (no re-bench) so the next request sees it.
	if env.hwMode.SoftwareAV1MaxOverride() != 720 {
		t.Errorf("controller live override = %d want 720", env.hwMode.SoftwareAV1MaxOverride())
	}
}

func TestServerSettings_Put_SoftwareAV1MaxHeight_Invalid_Ignored(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"softwareAV1MaxHeight":999}`)) // not on the ladder
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if _, ok := env.store.sets[cfgKeyTranscodeSWAV1]; ok {
		t.Errorf("an off-ladder AV1 height must not be persisted")
	}
	if env.hwMode.SoftwareAV1MaxOverride() != 0 {
		t.Errorf("an off-ladder AV1 height must not be applied, got %d", env.hwMode.SoftwareAV1MaxOverride())
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
		strings.NewReader(`{"loginRateLimit":42,"globalRateLimit":99}`))
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
	if got := env.store.sets[cfgKeyRateLimitLogin]; got != "42" {
		t.Errorf("store login = %q", got)
	}
	if got := env.store.sets[cfgKeyRateLimitGlobal]; got != "99" {
		t.Errorf("store global = %q", got)
	}
}

// TestServerSettings_ConcurrentReadWrite_NoRace hammers the handler with
// concurrent PUTs (which write cfg fields under cfg's write lock) and GETs,
// alongside the live-reader path (Effective* accessors, as used by the
// transcode/segments handlers). Run under `go test -race` it proves the admin
// server-settings write no longer races the live config readers.
func TestServerSettings_ConcurrentReadWrite_NoRace(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))

	var live sync.WaitGroup
	stop := make(chan struct{})
	// Live readers: exactly the accessors the request path calls per stream.
	for range 4 {
		live.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = env.cfg.EffectiveServerName()
				_ = env.cfg.EffectiveMaxConcurrent()
				_ = env.cfg.EffectiveHWAccelEnabled()
				_ = env.cfg.EffectiveAV1Enabled()
				_ = env.cfg.EffectiveAV1MaxHeight()
				_ = env.cfg.EffectiveSkipPreviewEnabled()
			}
		})
	}

	// Concurrent GET (snapshot) + PUT (mutate) requests.
	var reqs sync.WaitGroup
	for w := range 3 {
		reqs.Add(1)
		go func(w int) {
			defer reqs.Done()
			for j := range 60 {
				put := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
					strings.NewReader(`{"serverName":"n","maxConcurrentTranscodes":4,"hwMode":"auto","av1Enabled":true,"av1MaxHeight":1080,"skipPreviewEnabled":true}`))
				put.Header.Set("Authorization", "Bearer tok")
				env.mux.ServeHTTP(httptest.NewRecorder(), put)

				get := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
				get.Header.Set("Authorization", "Bearer tok")
				env.mux.ServeHTTP(httptest.NewRecorder(), get)
				_ = j
				_ = w
			}
		}(w)
	}
	reqs.Wait()
	close(stop)
	live.Wait()
}

// Retention windows + transcode cache budget joined the panel on 2026-07-23
// (project rule: operator knobs never live in env-only). The PUT must mutate
// cfg AND persist; negative day counts and a sub-1GB cache budget are form
// bugs, not intents, and must be ignored.
func TestServerSettings_Put_RetentionAndCache_RoundTrip(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"auditRetentionDays":365,"metricsRetentionDays":60,"qoeRetentionDays":14,"playSessionRetentionDays":90,"transcodeCacheMaxGB":20}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Log.AuditRetentionDays != 365 || env.cfg.Metrics.RetentionDays != 60 ||
		env.cfg.Metrics.QoERetentionDays != 14 || env.cfg.Playback.SessionRetentionDays != 90 ||
		env.cfg.Transcoding.CacheMaxGB != 20 {
		t.Errorf("cfg not updated: audit=%d metrics=%d qoe=%d session=%d cache=%d",
			env.cfg.Log.AuditRetentionDays, env.cfg.Metrics.RetentionDays,
			env.cfg.Metrics.QoERetentionDays, env.cfg.Playback.SessionRetentionDays,
			env.cfg.Transcoding.CacheMaxGB)
	}
	for key, want := range map[string]string{
		cfgKeyAuditRetention:   "365",
		cfgKeyMetricsRetention: "60",
		cfgKeyQoERetention:     "14",
		cfgKeySessionRetention: "90",
		cfgKeyTranscodeCacheGB: "20",
	} {
		if got := env.store.sets[key]; got != want {
			t.Errorf("store[%s] = %q want %q", key, got, want)
		}
	}
}

func TestServerSettings_Put_RetentionAndCache_RejectsNonsense(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	env.cfg.Metrics.QoERetentionDays = 30
	env.cfg.Transcoding.CacheMaxGB = 50
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"qoeRetentionDays":-1,"transcodeCacheMaxGB":0}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Metrics.QoERetentionDays != 30 {
		t.Errorf("negative days must be ignored, got %d", env.cfg.Metrics.QoERetentionDays)
	}
	if env.cfg.Transcoding.CacheMaxGB != 50 {
		t.Errorf("cache budget < 1GB must be ignored, got %d", env.cfg.Transcoding.CacheMaxGB)
	}
	if _, ok := env.store.sets[cfgKeyQoERetention]; ok {
		t.Errorf("ignored value must not be persisted")
	}
}

// GET must expose the new settings with their real defaults so the panel can
// render them without hardcoding.
func TestServerSettings_Get_ExposesRetentionAndCache(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]settingValue
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, key := range []string{"auditRetentionDays", "metricsRetentionDays", "qoeRetentionDays", "playSessionRetentionDays", "transcodeCacheMaxGB"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("GET response missing %q", key)
		}
	}
}

// Live-check 2026-07-24: the UI's Reset button PUTs {field: null}, which the
// handler silently ignored - the override survived in cfg, in the store and
// in `overridden`. An explicit null must CLEAR: cfg back to the default, the
// DB row deleted, the source back to "default".
func TestServerSettings_Put_ExplicitNullClearsOverride(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))

	set := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"qoeRetentionDays":14,"loginRateLimit":42}`))
	set.Header.Set("Authorization", "Bearer tok")
	env.mux.ServeHTTP(httptest.NewRecorder(), set)
	if env.cfg.Metrics.QoERetentionDays != 14 {
		t.Fatalf("override PUT did not land")
	}

	clear := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"qoeRetentionDays":null}`))
	clear.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, clear)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear PUT = %d", rec.Code)
	}

	if env.cfg.Metrics.QoERetentionDays != 30 {
		t.Errorf("cfg must return to the default (30), got %d", env.cfg.Metrics.QoERetentionDays)
	}
	if _, ok := env.store.sets[cfgKeyQoERetention]; ok {
		t.Errorf("the DB override row must be deleted on clear")
	}
	// The untouched sibling override must survive.
	if env.cfg.Server.LoginRateLimitMax != 42 {
		t.Errorf("clearing one key must not touch the others")
	}

	get := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	get.Header.Set("Authorization", "Bearer tok")
	grec := httptest.NewRecorder()
	env.mux.ServeHTTP(grec, get)
	var resp map[string]settingValue
	if err := json.Unmarshal(grec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp["qoeRetentionDays"].Source != "default" {
		t.Errorf("source must read default after clear, got %q", resp["qoeRetentionDays"].Source)
	}
	if resp["loginRateLimit"].Source != "override" {
		t.Errorf("sibling must stay override, got %q", resp["loginRateLimit"].Source)
	}
}

// After a restart the handler used to show source=default for EVERY setting
// even when a DB override was live (the `overridden` map started empty): the
// panel lied about what was in effect. Registration must seed it from the store.
func TestServerSettings_Get_SeedsOverriddenFromStoreAtBoot(t *testing.T) {
	store := newFakeSettingsStore()
	store.sets[cfgKeyQoERetention] = "14"
	env := newSettingsTestEnvWithStore(t, validUserValidator(true), store)

	get := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	get.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, get)
	var resp map[string]settingValue
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp["qoeRetentionDays"].Source != "override" {
		t.Errorf("a persisted override must read source=override after a restart, got %q", resp["qoeRetentionDays"].Source)
	}
	if resp["serverName"].Source != "default" {
		t.Errorf("keys without a stored row must stay default, got %q", resp["serverName"].Source)
	}
}

func newSettingsTestEnvWithStore(t *testing.T, verifier TokenVerifier, store *fakeSettingsStore) *settingsTestEnv {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.Name = "Evey"
	cfg.Server.LoginRateLimitMax = 10
	cfg.Server.GlobalRateLimitMax = 300
	cfg.Transcoding.MaxConcurrent = 2
	env := &settingsTestEnv{
		mux:      http.NewServeMux(),
		cfg:      cfg,
		store:    store,
		hwMode:   &fakeHWModeController{},
		loginRL:  ratelimit.New(10),
		globalRL: ratelimit.New(300),
		writeRL:  ratelimit.New(1000),
	}
	registerServerSettingsRoutes(env.mux, env.cfg, env.store, verifier, nil, env.hwMode, env.loginRL, env.globalRL, env.writeRL)
	t.Cleanup(env.cleanup)
	return env
}

// The five transcoding knobs that were env-only until 2026-07-30. They are the
// operator's levers on weak hardware (the 720p-HDR ceiling was diagnosed in
// production through EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT), so the panel has
// to be able to set them - and to say that they need a restart, because wire.go
// snapshots every one of them at boot.
func TestServerSettings_Put_TranscodingKnobs_RoundTrip(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"transcodeMaxSessions":3,"transcodeSingleVariant":true,"transcodeHWPerVariant":true,"softwareMaxHeight":1080,"softwareHDRMaxHeight":720}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.cfg.Transcoding.MaxSessions != 3 || !env.cfg.Transcoding.SingleVariant ||
		!env.cfg.Transcoding.HWPerVariant || env.cfg.Transcoding.SoftwareMaxHeightOverride != 1080 ||
		env.cfg.Transcoding.SoftwareHDRMaxHeightOverride != 720 {
		t.Errorf("cfg not updated: sessions=%d single=%v perVariant=%v swMax=%d swHDRMax=%d",
			env.cfg.Transcoding.MaxSessions, env.cfg.Transcoding.SingleVariant,
			env.cfg.Transcoding.HWPerVariant, env.cfg.Transcoding.SoftwareMaxHeightOverride,
			env.cfg.Transcoding.SoftwareHDRMaxHeightOverride)
	}
	// Persistence is the whole point: these are read back by main's loadOverride
	// at the next boot, which is the moment they take effect.
	for key, want := range map[string]string{
		cfgKeyTranscodeMaxSessions:   "3",
		cfgKeyTranscodeSingleVariant: "true",
		cfgKeyTranscodeHWPerVariant:  "true",
		cfgKeyTranscodeSWMaxHeight:   "1080",
		cfgKeyTranscodeSWHDRMaxHt:    "720",
	} {
		if got := env.store.sets[key]; got != want {
			t.Errorf("store[%s] = %q want %q", key, got, want)
		}
	}
}

// 0 is a MEANINGFUL value on the three numeric knobs ("no override"), so it must
// be accepted and persisted - unlike transcodeCacheMaxGB, where 0 is nonsense.
func TestServerSettings_Put_TranscodingKnobs_ZeroIsAValidOverride(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	env.cfg.Transcoding.SoftwareHDRMaxHeightOverride = 1080
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"softwareHDRMaxHeight":0}`))
	req.Header.Set("Authorization", "Bearer tok")
	env.mux.ServeHTTP(httptest.NewRecorder(), req)
	if env.cfg.Transcoding.SoftwareHDRMaxHeightOverride != 0 {
		t.Errorf("0 must be applied, got %d", env.cfg.Transcoding.SoftwareHDRMaxHeightOverride)
	}
	if got := env.store.sets[cfgKeyTranscodeSWHDRMaxHt]; got != "0" {
		t.Errorf("store[%s] = %q want %q", cfgKeyTranscodeSWHDRMaxHt, got, "0")
	}
}

func TestServerSettings_Put_TranscodingKnobs_RejectsNegative(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	env.cfg.Transcoding.MaxSessions = 2
	env.cfg.Transcoding.SoftwareMaxHeightOverride = 1080
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"transcodeMaxSessions":-1,"softwareMaxHeight":-720}`))
	req.Header.Set("Authorization", "Bearer tok")
	env.mux.ServeHTTP(httptest.NewRecorder(), req)
	if env.cfg.Transcoding.MaxSessions != 2 {
		t.Errorf("negative sessions must be ignored, got %d", env.cfg.Transcoding.MaxSessions)
	}
	if env.cfg.Transcoding.SoftwareMaxHeightOverride != 1080 {
		t.Errorf("negative height must be ignored, got %d", env.cfg.Transcoding.SoftwareMaxHeightOverride)
	}
	if _, ok := env.store.sets[cfgKeyTranscodeMaxSessions]; ok {
		t.Errorf("ignored value must not be persisted")
	}
}

// A saved value the running process ignores, with nothing saying so, reads as a
// broken setting. The panel needs the flag to tell the operator to restart.
func TestServerSettings_Get_TranscodingKnobsRequireRestart(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]settingValue
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, key := range []string{
		"transcodeMaxSessions", "transcodeSingleVariant", "transcodeHWPerVariant",
		"softwareMaxHeight", "softwareHDRMaxHeight",
		// Pre-existing boot-snapshotted settings whose contract was only ever a
		// Go comment until now.
		"transcodeCacheMaxGB", "libraryWatcherEnabled", "periodicScanInterval",
	} {
		v, ok := resp[key]
		if !ok {
			t.Errorf("GET response missing %q", key)
			continue
		}
		if !v.RequiresRestart {
			t.Errorf("%q must be flagged requiresRestart", key)
		}
	}
	// A live-applied setting must NOT be flagged, or the panel would tell the
	// operator to restart for a change that already took effect.
	for _, key := range []string{"logLevel", "loginRateLimit", "qoeRetentionDays", "skipPreviewEnabled"} {
		if v, ok := resp[key]; ok && v.RequiresRestart {
			t.Errorf("%q applies live and must not be flagged requiresRestart", key)
		}
	}
}

// Off-ladder heights are refused rather than clamped, same as the AV1 knobs:
// these values build the encoder ladder, and an accidental extra digit that
// looks saved is worse than a rejected save.
func TestServerSettings_Put_TranscodingKnobs_RejectsOffLadderValues(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	env.cfg.Transcoding.SoftwareMaxHeightOverride = 1080
	env.cfg.Transcoding.SoftwareHDRMaxHeightOverride = 1080
	env.cfg.Transcoding.MaxSessions = 4

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-settings",
		strings.NewReader(`{"softwareMaxHeight":10800,"softwareHDRMaxHeight":999,"transcodeMaxSessions":2000000}`))
	req.Header.Set("Authorization", "Bearer tok")
	env.mux.ServeHTTP(httptest.NewRecorder(), req)

	if env.cfg.Transcoding.SoftwareMaxHeightOverride != 1080 {
		t.Errorf("off-ladder height must be ignored, got %d", env.cfg.Transcoding.SoftwareMaxHeightOverride)
	}
	if env.cfg.Transcoding.SoftwareHDRMaxHeightOverride != 1080 {
		t.Errorf("off-ladder HDR height must be ignored, got %d", env.cfg.Transcoding.SoftwareHDRMaxHeightOverride)
	}
	if env.cfg.Transcoding.MaxSessions != 4 {
		t.Errorf("absurd session cap must be ignored, got %d", env.cfg.Transcoding.MaxSessions)
	}
	for _, key := range []string{cfgKeyTranscodeSWMaxHeight, cfgKeyTranscodeSWHDRMaxHt, cfgKeyTranscodeMaxSessions} {
		if _, ok := env.store.sets[key]; ok {
			t.Errorf("rejected value must not be persisted (%s)", key)
		}
	}
}

// The admin panel carries its OWN copy of each default (defaultVal + reset),
// separate from config.go's. They have to agree, or "Reset to default" in the
// panel would quietly move the server off the shipped behaviour - here, back to
// encoding the whole ladder for a viewer watching one rung.
func TestServerSettings_SingleVariantDefaultsToOn(t *testing.T) {
	env := newSettingsTestEnv(t, validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]settingValue
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	v, ok := resp["transcodeSingleVariant"]
	if !ok {
		t.Fatal("transcodeSingleVariant must be exposed")
	}
	if v.Default != true {
		t.Errorf("panel default = %v, want true - it must match config.go's EVEY_TRANSCODING_SINGLE_VARIANT default", v.Default)
	}
}
