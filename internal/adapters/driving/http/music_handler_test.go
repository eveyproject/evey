package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainmusic "github.com/evey-project/evey/internal/domain/music"
	musicfakes "github.com/evey-project/evey/internal/domain/music/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
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
	registerMusicRoutes(mux, svc, tokenGen, allowAll(), nil, ratelimit.New(10000), nil)
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

// ---------- shared test helpers ----------

type fakeLibQueryRepo struct {
	libs []*domainlibrary.Library
	err  error
}

func (f *fakeLibQueryRepo) List(_ context.Context, _ domainlibrary.ListLibrariesFilter) ([]*domainlibrary.Library, error) {
	return f.libs, f.err
}
func (f *fakeLibQueryRepo) GetByID(_ context.Context, _ shared.LibraryID) (*domainlibrary.Library, error) {
	return nil, nil
}

func newMusicMux(t *testing.T, isAdmin bool, checker LibraryAccessChecker, repo *musicfakes.FakeMusicTrackRepository, libRepo domainlibrary.LibraryQueryRepo) (*http.ServeMux, string, *appmusic.Service) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{
		UserID:  "user-1",
		IsAdmin: isAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, libRepo, musicfakes.NewFakeMusicScanner())
	mux := http.NewServeMux()
	registerMusicRoutes(mux, svc, tokenGen, checker, nil, ratelimit.New(10000), nil)
	return mux, token, svc
}

func writeTempFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ---------- streamTrack ----------

func TestStreamTrack_TrackNotFound_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/missing/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestStreamTrack_AccessDenied_403(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		Title:     "Foo",
		FilePath:  "/tmp/whatever.mp3",
	}); err != nil {
		t.Fatal(err)
	}
	mux, token, _ := newMusicMux(t, false, denyAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestStreamTrack_Success(t *testing.T) {
	dir := t.TempDir()
	body := []byte("ID3\x04\x00\x00\x00\x00\x00\x00fake-audio-content")
	path := writeTempFile(t, dir, "song.mp3", body)

	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		Title:     "Foo",
		FilePath:  path,
		MimeType:  "audio/mpeg",
	}); err != nil {
		t.Fatal(err)
	}
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Errorf("Content-Type = %q, want audio/mpeg", got)
	}
	if w.Body.Len() != len(body) {
		t.Errorf("body length = %d, want %d", w.Body.Len(), len(body))
	}
}

func TestStreamTrack_FileMissingOnDisk_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		FilePath:  filepath.Join(t.TempDir(), "does-not-exist.mp3"),
		MimeType:  "audio/mpeg",
	}); err != nil {
		t.Fatal(err)
	}
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------- serveAlbumArt ----------

func TestServeAlbumArt_InvalidHash_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	// "zzz!" is not 16-128 hex chars
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/album-art/notavalidhash", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid hash, got %d", w.Code)
	}
}

func TestServeAlbumArt_NoAlbumArtDir_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	// AlbumArtDir is empty since WithAlbumArtFetcher was never called.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/album-art/abcdef0123456789", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServeAlbumArt_FileMissing_404(t *testing.T) {
	dir := t.TempDir()
	repo := musicfakes.NewFakeMusicTrackRepository()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1"})
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, musicfakes.NewFakeMusicScanner())
	svc.CommandService().WithAlbumArtFetcher(nil, dir)
	mux := http.NewServeMux()
	registerMusicRoutes(mux, svc, tokenGen, allowAll(), nil, ratelimit.New(10000), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/album-art/abcdef0123456789", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing file, got %d", w.Code)
	}
}

func TestServeAlbumArt_Success(t *testing.T) {
	dir := t.TempDir()
	artDir := filepath.Join(dir, "music_album_art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := "abcdef0123456789"
	imgBytes := []byte("\xff\xd8\xff\xe0fakejpeg")
	if err := os.WriteFile(filepath.Join(artDir, hash+".jpg"), imgBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := musicfakes.NewFakeMusicTrackRepository()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1"})
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, musicfakes.NewFakeMusicScanner())
	// WithAlbumArtFetcher sets albumArtDir = filepath.Join(dataDir, "music_album_art")
	svc.CommandService().WithAlbumArtFetcher(nil, dir)
	mux := http.NewServeMux()
	registerMusicRoutes(mux, svc, tokenGen, allowAll(), nil, ratelimit.New(10000), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/album-art/"+hash, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "max-age=2592000") {
		t.Errorf("Cache-Control missing max-age=2592000: %q", w.Header().Get("Cache-Control"))
	}
	if w.Body.Len() != len(imgBytes) {
		t.Errorf("body length = %d, want %d", w.Body.Len(), len(imgBytes))
	}
}

func TestServeAlbumArt_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/album-art/abcdef0123456789", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------- fetchMissingArt ----------

func TestFetchMissingArt_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/music/fetch-missing-art", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFetchMissingArt_NonAdmin_403(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/music/fetch-missing-art", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestFetchMissingArt_Success_NoLibRepo(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	// libRepo == nil → service returns (0, nil)
	mux, token, _ := newMusicMux(t, true, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/music/fetch-missing-art", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// The scan now runs detached (it can far outlast the proxy timeout on a large
	// library), so the endpoint accepts the work and returns 202 immediately.
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestFetchMissingArt_Success_WithLibRepo(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	// non-music libs → skipped, count = 0
	libRepo := &fakeLibQueryRepo{
		libs: []*domainlibrary.Library{
			{ID: shared.LibraryID("lib-1"), Type: domainlibrary.Movie},
		},
	}
	mux, token, _ := newMusicMux(t, true, allowAll(), repo, libRepo)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/music/fetch-missing-art", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Detached scan → 202 accepted immediately; the per-library iteration is
	// covered by the service-level test (FetchMissingAlbumArtAllLibraries).
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// ---------- getTrackArt access denial ----------

// ---------- downloadTrack ----------

// newMusicMuxWithLister creates a mux with a rate-limiter and a libLister for
// testing path confinement on music download routes.
func newMusicMuxWithLister(t *testing.T, repo *musicfakes.FakeMusicTrackRepository, checker LibraryAccessChecker, libLister MediaArchiveSource) (*http.ServeMux, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})
	if err != nil {
		t.Fatal(err)
	}
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, musicfakes.NewFakeMusicScanner())
	mux := http.NewServeMux()
	registerMusicRoutes(mux, svc, tokenGen, checker, nil, ratelimit.New(10000), libLister)
	return mux, token
}

func TestDownloadTrack_PathOutsideRoots_404(t *testing.T) {
	dir := t.TempDir()
	trackPath := writeTempFile(t, dir, "song.mp3", []byte("audiodata"))

	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		Title:     "Song",
		FilePath:  trackPath,
	}); err != nil {
		t.Fatal(err)
	}
	// roots does NOT include dir
	otherDir := t.TempDir()
	libLister := &fakeMediaItemGetter{
		libraries: []*domainlibrary.Library{
			{ID: "lib-A", Paths: []string{otherDir}},
		},
	}
	mux, token := newMusicMuxWithLister(t, repo, allowAll(), libLister)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when track path is outside roots, got %d", w.Code)
	}
}

func TestDownloadTrack_PathInsideRoots_Serves(t *testing.T) {
	dir := t.TempDir()
	trackPath := writeTempFile(t, dir, "song.mp3", []byte("audiodata"))

	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		Title:     "Song",
		FilePath:  trackPath,
	}); err != nil {
		t.Fatal(err)
	}
	// roots includes dir
	libLister := &fakeMediaItemGetter{
		libraries: []*domainlibrary.Library{
			{ID: "lib-A", Paths: []string{dir}},
		},
	}
	mux, token := newMusicMuxWithLister(t, repo, allowAll(), libLister)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusPartialContent {
		t.Errorf("expected 200 when track path is inside roots, got %d", w.Code)
	}
}

func TestDownloadTrack_NilLister_FailOpen(t *testing.T) {
	dir := t.TempDir()
	trackPath := writeTempFile(t, dir, "song.mp3", []byte("audiodata"))

	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		Title:     "Song",
		FilePath:  trackPath,
	}); err != nil {
		t.Fatal(err)
	}
	// nil libLister → no confinement → fail open
	mux, token := newMusicMuxWithLister(t, repo, allowAll(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusPartialContent {
		t.Errorf("nil lister should fail open and serve the file, got %d", w.Code)
	}
}

func TestGetTrackArt_AccessDenied_403(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	if _, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        "t1",
		LibraryID: shared.LibraryID("lib-A"),
		FilePath:  "/tmp/whatever.mp3",
	}); err != nil {
		t.Fatal(err)
	}
	mux, token, _ := newMusicMux(t, false, denyAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/art", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
