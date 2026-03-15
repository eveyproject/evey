package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmusic "github.com/gravity-zero/evey/internal/app/music"
	musicdomain "github.com/gravity-zero/evey/internal/domain/music"
	musicfakes "github.com/gravity-zero/evey/internal/domain/music/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
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
	// exact match is NOT within (it IS the dir, not inside it)
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
	svc := appmusic.NewService(repo, bus, nil)
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
	// no Authorization header
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListTracks_IDOR_DeniedLibrary(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-x", IsAdmin: false})

	mux := http.NewServeMux()
	repo := musicfakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)
	registerMusicRoutes(mux, svc, tokenGen, denyAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks?library_id=lib-other", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied library, got %d", w.Code)
	}
}

func TestListTracks_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks?library_id=lib-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListTracks_Auth_200(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks?library_id=lib-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
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

func TestListAlbums_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums?library_id=lib-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListAlbums_Auth_200(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums?library_id=lib-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListAlbums_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-x", IsAdmin: false})

	mux := http.NewServeMux()
	repo := musicfakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)
	registerMusicRoutes(mux, svc, tokenGen, denyAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums?library_id=lib-other", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListArtists_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/artists?library_id=lib-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListArtists_Auth_200(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/artists?library_id=lib-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListArtists_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-x", IsAdmin: false})

	mux := http.NewServeMux()
	repo := musicfakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)
	registerMusicRoutes(mux, svc, tokenGen, denyAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/artists?library_id=lib-other", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDeleteTrack_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/music/tracks/track-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// Aliases matching TEST-NEW-4 required names
func TestMusicTracks_NoAuth_401(t *testing.T)      { TestListTracks_NoAuth_401(t) }
func TestMusicTracks_Auth_200(t *testing.T)        { TestListTracks_Auth_200(t) }
func TestMusicAlbums_NoAuth_401(t *testing.T)      { TestListAlbums_NoAuth_401(t) }
func TestMusicAlbums_Auth_200(t *testing.T)        { TestListAlbums_Auth_200(t) }
func TestMusicArtists_NoAuth_401(t *testing.T)     { TestListArtists_NoAuth_401(t) }
func TestMusicArtists_Auth_200(t *testing.T)       { TestListArtists_Auth_200(t) }
func TestMusicStream_NoAuth_401(t *testing.T)      { TestStreamTrack_NoAuth_401(t) }
func TestMusicDeleteTrack_NoAuth_401(t *testing.T) { TestDeleteTrack_NoAuth_401(t) }

func TestDeleteTrack_NoLibraryAccess_Forbidden(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-x", IsAdmin: false})

	mux := http.NewServeMux()
	repo := musicfakes.NewFakeMusicTrackRepository()
	// seed a track so it's found (not 404)
	ctx := context.TODO()
	_ = repo.Upsert(ctx, musicdomain.Track{ID: "track-1", LibraryID: "lib-1", FilePath: "/music/song.mp3"})
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil)
	registerMusicRoutes(mux, svc, tokenGen, denyAll(), nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/music/tracks/track-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied library access, got %d", w.Code)
	}
}
