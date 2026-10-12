package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appconfig "github.com/eveyproject/evey/internal/app/config"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// memServerConfigRepo is a state machine, not a mock: the test asserts what
// came back out, so a handler that "called Set" without a usable value fails.
type memServerConfigRepo struct {
	mu      sync.Mutex
	entries map[string]string
	setErr  error
}

func (r *memServerConfigRepo) Get(_ context.Context, key string) (*shared.ServerConfigEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.entries[key]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &shared.ServerConfigEntry{Key: key, Value: v, UpdatedAt: time.Now()}, nil
}

func (r *memServerConfigRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setErr != nil {
		return r.setErr
	}
	if r.entries == nil {
		r.entries = map[string]string{}
	}
	r.entries[key] = value
	return nil
}

func (r *memServerConfigRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
	return nil
}

func (r *memServerConfigRepo) List(_ context.Context) ([]*shared.ServerConfigEntry, error) {
	return nil, nil
}

// This route and the gRPC PlaybackSettings handler are bound to the same
// method+path, and Go 1.22+ ServeMux precedence hands every request to this
// one - the gateway is only a catch-all fallback. So the gRPC handler's
// persistence never ran, and the operator's change survived exactly until the
// next restart, when the boot-time read pulled the untouched row back.
//
// Asserting on the STORE (not on a spy call count) is what makes this test
// about the outcome: the setting is still there after the process is gone.
func TestPlaybackSettings_PutPersistsForTheNextBoot(t *testing.T) {
	gen, adminTok, _ := genTokens(t)
	cfg := &config.Config{Playback: config.PlaybackConfig{SkipForwardSeconds: 30, SkipBackwardSeconds: 10}}
	repo := &memServerConfigRepo{}
	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, gen, ratelimit.New(100), appconfig.NewService(repo))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/playback-settings",
		strings.NewReader(`{"skipForwardSeconds":45,"skipBackwardSeconds":15}`))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	// Applied in memory...
	if fwd, bwd := cfg.EffectivePlaybackSkipSeconds(); fwd != 45 || bwd != 15 {
		t.Errorf("in-memory config = %d/%d, want 45/15", fwd, bwd)
	}
	// ...AND written under the keys the boot-time read looks for.
	for key, want := range map[string]string{
		appconfig.KeyPlaybackSkipForward:  "45",
		appconfig.KeyPlaybackSkipBackward: "15",
	} {
		got, err := repo.Get(context.Background(), key)
		if err != nil {
			t.Errorf("%s was never persisted: the setting reverts on restart", key)
			continue
		}
		if got.Value != want {
			t.Errorf("%s = %q, want %q", key, got.Value, want)
		}
	}
}

// A persist failure must not be answered 200: the panel would show the value
// as saved while the next boot reads the old row back. Same contract as the
// generic settings PUT - applied in memory, reported as not saved.
func TestPlaybackSettings_PutPersistFailureIs500(t *testing.T) {
	gen, adminTok, _ := genTokens(t)
	cfg := &config.Config{Playback: config.PlaybackConfig{SkipForwardSeconds: 30, SkipBackwardSeconds: 10}}
	repo := &memServerConfigRepo{setErr: errors.New("db down")}
	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, gen, ratelimit.New(100), appconfig.NewService(repo))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/playback-settings",
		strings.NewReader(`{"skipForwardSeconds":45,"skipBackwardSeconds":15}`))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not persisted") {
		t.Errorf("body should say the value was not persisted: %s", rec.Body.String())
	}
	if cfg.Playback.SkipForwardSeconds != 45 {
		t.Errorf("live config should keep the applied value, got %d", cfg.Playback.SkipForwardSeconds)
	}
}
