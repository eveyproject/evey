package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// --- shared helpers ---

// --- playbackInfoHandler ---

func TestPlaybackInfo_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(playbackInfoHandler(
		&fakeMediaStreamsGetter{},
		allowAll(),
		nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfo_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(playbackInfoHandler(
		&fakeMediaStreamsGetter{},
		allowAll(),
		nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfo_NotFound_404(t *testing.T) {
	h := playbackInfoHandler(
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfo_IDOR_403(t *testing.T) {
	h := playbackInfoHandler(
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", File: &domainlibrary.MediaFile{Path: "/a"}}},
		denyAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfo_OK(t *testing.T) {
	w1 := 1920
	h1 := 1080
	h := playbackInfoHandler(
		&fakeMediaStreamsGetter{
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a.mkv", ContainerFormat: "matroska"}},
			streams: []*domainlibrary.MediaStream{
				{StreamType: "video", Codec: "h264", Width: &w1, Height: &h1},
				{StreamType: "audio", Codec: "aac", IsDefault: true},
			},
		},
		allowAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// --- playback settings ---

func newPlaybackSettingsMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{Playback: config.PlaybackConfig{SkipForwardSeconds: 30, SkipBackwardSeconds: 10}}
	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, gen, ratelimit.New(100), nil)
	return mux, adminTok, userTok
}

func TestPlaybackSettings_Get_NoAuth_401(t *testing.T) {
	mux, _, _ := newPlaybackSettingsMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playback-settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackSettings_Get_Auth_200(t *testing.T) {
	mux, _, userTok := newPlaybackSettingsMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playback-settings", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "skipForwardSeconds") {
		t.Error("expected skipForwardSeconds in response")
	}
}

func TestPlaybackSettings_Put_Admin_OK(t *testing.T) {
	mux, adminTok, _ := newPlaybackSettingsMux(t)
	body := `{"skipForwardSeconds":15}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/playback-settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPlaybackSettings_Put_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newPlaybackSettingsMux(t)
	body := `{"skipForwardSeconds":15}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/playback-settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

type fakeBitrateSvc struct{}

func (f *fakeBitrateSvc) GetUserBitrateLimit(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func TestNewUserBitrateAdapter(t *testing.T) {
	adapter := NewUserBitrateAdapter(&fakeBitrateSvc{})
	limit, err := adapter.GetUserBitrateLimit(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if limit != 0 {
		t.Errorf("expected 0 for missing user, got %d", limit)
	}
}
