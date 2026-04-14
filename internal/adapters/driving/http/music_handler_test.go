package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	musicfakes "github.com/evey-project/evey/internal/domain/music/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
)

func TestIsWithinDir_Inside(t *testing.T) {
	if !isWithinDir("/music", "/music/rock/song.mp3") {
		t.Error("expected true for path inside dir")
	}
}

func TestIsWithinDir_Outside(t *testing.T) {
	if isWithinDir("/music", "/other/song.mp3") {
		t.Error("expected false for path outside dir")
	}
}

func TestIsWithinDir_ExactMatch(t *testing.T) {
	if isWithinDir("/music", "/music") {
		t.Error("expected false for exact match")
	}
}

func TestTailFile_NotFound(t *testing.T) {
	_, err := tailFile("/nonexistent/path/evey.log", 10)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestExtractBearerToken_HeaderPriority(t *testing.T) {
	req := httptest.NewRequest("GET", "/?token=qparam", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	got := extractBearerToken(req)
	if got != "header-token" {
		t.Errorf("header should take priority, got %q", got)
	}
}

func newMusicTestMuxWithToken(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{
		UserID:  "user-1",
		IsAdmin: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	repo := musicfakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, nil)
	registerMusicRoutes(mux, svc, tokenGen, allowAll(), nil)
	return mux, token
}

func TestGetTrackArt_NotFound(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/nonexistent/art", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetTrackArt_Unauthenticated(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/some-id/art", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamTrack_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/track-1/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMusicStream_NoAuth_401(t *testing.T) { TestStreamTrack_NoAuth_401(t) }
