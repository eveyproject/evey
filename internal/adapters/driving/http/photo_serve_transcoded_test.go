package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	domainphoto "github.com/evey-project/evey/internal/domain/photo"
	photofakes "github.com/evey-project/evey/internal/domain/photo/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePhotoTranscoder implements PhotoTranscoder for tests.
type fakePhotoTranscoder struct {
	err error
	// called records whether Transcode was invoked.
	called bool
}

func (f *fakePhotoTranscoder) Transcode(_ context.Context, _, _ string, _ int, _ string) error {
	f.called = true
	return f.err
}

// newPhotoMux builds a minimal mux with the photo serve route registered.
func newPhotoMux(t *testing.T, repo *photofakes.FakeRepository, transcoder PhotoTranscoder, cacheDir string) (*http.ServeMux, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{
		UserID:    "user-1",
		ProfileID: "prof-1",
		IsAdmin:   false,
	})
	require.NoError(t, err)
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	mux := http.NewServeMux()
	registerPhotoRoutes(mux, svc, tokenGen, allowAll(), nil, context.Background(), transcoder, cacheDir)
	return mux, token
}

// seedPhoto inserts a photo into the fake repo and returns its ID.
func seedPhoto(t *testing.T, repo *photofakes.FakeRepository, id, libID, filePath, mimeType string) {
	t.Helper()
	err := repo.Upsert(context.Background(), domainphoto.Photo{
		ID:        id,
		LibraryID: shared.LibraryID(libID),
		FilePath:  filePath,
		MimeType:  mimeType,
	})
	require.NoError(t, err)
}

// --- tests: serveTranscoded (via servePhoto ?w= / ?fmt= params) ---

// Invalid width param (non-numeric): serveTranscoded returns false → fallthrough
// to raw file serving → file not found → 404.
func TestServeTranscoded_InvalidWidth_FallsThrough(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph1", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoder{}

	mux, token := newPhotoMux(t, repo, tc, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph1/serve?w=notanumber", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// serveTranscoded returned false (invalid w), servePhoto fell through to
	// os.Open(filePath) which failed → 404. Transcoder must NOT have been called.
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, tc.called, "transcoder must not be invoked for invalid width")
}

// Zero width: serveTranscoded returns false (v <= 0).
func TestServeTranscoded_ZeroWidth_FallsThrough(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph2", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoder{}

	mux, token := newPhotoMux(t, repo, tc, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph2/serve?w=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, tc.called)
}

// Width > 8192: serveTranscoded returns false (v > 8192).
func TestServeTranscoded_OversizedWidth_FallsThrough(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph3", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoder{}

	mux, token := newPhotoMux(t, repo, tc, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph3/serve?w=9000", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, tc.called)
}

// Unknown fmt param: serveTranscoded returns false.
func TestServeTranscoded_UnknownFormat_FallsThrough(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph4", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoder{}

	mux, token := newPhotoMux(t, repo, tc, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph4/serve?fmt=avif", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, tc.called)
}

// Transcoder returns an error: serveTranscoded returns false (fallthrough).
func TestServeTranscoded_TranscoderError_FallsThrough(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph5", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoder{err: errors.New("ffmpeg failed")}

	mux, token := newPhotoMux(t, repo, tc, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph5/serve?w=320&fmt=webp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// transcoder was called but returned error → serveTranscoded returned false
	assert.True(t, tc.called, "transcoder must have been invoked")
	// fell through to raw file serving → file not found → 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Cache hit: cached file already exists on disk → serveTranscoded returns true
// without calling the transcoder, and serves the cached file (200).
func TestServeTranscoded_CacheHit_ServesFile(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	cacheDir := t.TempDir()
	photoDir := filepath.Join(cacheDir, "photos")
	require.NoError(t, os.MkdirAll(photoDir, 0o755))

	// Compute the expected cache filename: p.ID + "-w" + width + "-webp.webp"
	photoID := "ph6"
	width := 640
	dstName := photoID + "-w" + strconv.Itoa(width) + "-webp.webp"
	dstPath := filepath.Join(photoDir, dstName)
	// Write a minimal stub file > 0 bytes so os.Stat sees size > 0.
	require.NoError(t, os.WriteFile(dstPath, []byte("WEBPDATA"), 0o644))

	seedPhoto(t, repo, photoID, "lib-1", "/does/not/matter.jpg", "image/jpeg")
	tc := &fakePhotoTranscoder{}

	mux, token := newPhotoMux(t, repo, tc, cacheDir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph6/serve?w=640&fmt=webp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/webp", w.Header().Get("Content-Type"))
	assert.False(t, tc.called, "transcoder must NOT be called on cache hit")
}

// Transcoder nil or cacheDir empty: serveTranscoded branch is not entered at all.
func TestServeTranscoded_NoTranscoder_NoBranch(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph7", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")

	// transcoder=nil → the if-guard in servePhoto is false, branch skipped
	mux, token := newPhotoMux(t, repo, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph7/serve?w=320&fmt=webp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Falls through directly to raw file serving → file not found → 404.
	assert.Equal(t, http.StatusNotFound, w.Code)
}
