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
	"github.com/evey-project/evey/internal/pkg/ratelimit"
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

// fakePhotoTranscoderWriter records every width it was invoked with and
// writes a stub file at dst, exercising the cache-hit path the same way the
// real ffmpeg-backed transcoder would (unlike fakePhotoTranscoder above,
// which only records the call).
type fakePhotoTranscoderWriter struct {
	widths []int
}

func (f *fakePhotoTranscoderWriter) Transcode(_ context.Context, _, dst string, width int, _ string) error {
	f.widths = append(f.widths, width)
	return os.WriteFile(dst, []byte("stub"), 0o644)
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
	registerPhotoRoutes(mux, svc, tokenGen, allowAll(), nil, context.Background(), transcoder, cacheDir, nil)
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

// --- tests: snapPhotoWidth ---

func TestSnapPhotoWidth(t *testing.T) {
	cases := []struct {
		raw       string
		wantOK    bool
		wantWidth int
	}{
		{"", false, 0},
		{"notanumber", false, 0},
		{"0", false, 0},
		{"-5", false, 0},
		{"400", true, 400},                   // exact match
		{"320", true, 400},                   // snaps up
		{"150", true, 200},                   // snaps down
		{"8192", true, 1600},                 // previously-max value now bounded
		{"1000000000000000000000", false, 0}, // Atoi overflow → invalid
	}
	for _, c := range cases {
		width, ok := snapPhotoWidth(c.raw)
		assert.Equal(t, c.wantOK, ok, "raw=%q", c.raw)
		if c.wantOK {
			assert.Equal(t, c.wantWidth, width, "raw=%q", c.raw)
			assert.Contains(t, photoAllowedWidths, width, "snapped width must be one of the allowed set")
		}
	}
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

// An arbitrary width (previously accepted verbatim up to 8192, one cache
// file per distinct value - the disk-fill vector) must be SNAPPED to the
// nearest entry in photoAllowedWidths rather than rejected or taken literally.
func TestServeTranscoded_OversizedWidth_SnapsToNearestAllowed(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph3", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoderWriter{}

	mux, token := newPhotoMux(t, repo, tc, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph3/serve?w=9000&fmt=webp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []int{1600}, tc.widths, "9000 must snap to the highest allowed tier (1600), not be served/rejected as-is")
}

// Many distinct arbitrary widths above the highest tier must all collapse
// onto the SAME cached variant instead of each writing its own file - the
// exact disk-fill vector this fix closes (w=1..8192, one file per value).
func TestServeTranscoded_ArbitraryWidths_CollapseOntoOneCachedVariant(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	seedPhoto(t, repo, "ph8", "lib-1", filepath.Join(dir, "nonexistent.jpg"), "image/jpeg")
	tc := &fakePhotoTranscoderWriter{}

	mux, token := newPhotoMux(t, repo, tc, dir)
	for _, wq := range []string{"5000", "8000", "1601", "8192"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph8/serve?w="+wq+"&fmt=webp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Every request above snapped onto 1600 - the transcoder must have run
	// exactly ONCE (the other 3 requests hit the cache), not once per distinct
	// raw width.
	assert.Equal(t, []int{1600}, tc.widths)

	photoDir := filepath.Join(dir, "photos")
	entries, err := os.ReadDir(photoDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "arbitrary widths must collapse onto one cached variant file, not one per width")
	assert.Equal(t, "ph8-w1600-webp.webp", entries[0].Name())
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

	// Compute the expected cache filename: p.ID + "-w" + width + "-webp.webp".
	// width is one of photoAllowedWidths verbatim (800) so the request below
	// (?w=800) maps to it with no snapping ambiguity.
	photoID := "ph6"
	width := 800
	dstName := photoID + "-w" + strconv.Itoa(width) + "-webp.webp"
	dstPath := filepath.Join(photoDir, dstName)
	// Write a minimal stub file > 0 bytes so os.Stat sees size > 0.
	require.NoError(t, os.WriteFile(dstPath, []byte("WEBPDATA"), 0o644))

	seedPhoto(t, repo, photoID, "lib-1", "/does/not/matter.jpg", "image/jpeg")
	tc := &fakePhotoTranscoder{}

	mux, token := newPhotoMux(t, repo, tc, cacheDir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph6/serve?w=800&fmt=webp", nil)
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

// --- tests: the rate limit is charged to forks, not to cache hits ---

// A television opening a photo grid asks for about twenty thumbnails at once.
// Gating the whole route charged every one of them to the ffmpeg ceiling, so
// the burst emptied and the rest of the grid came back empty (measured on
// device, 2026-08-03). Only a cache MISS forks, and only that is charged.
func TestServeTranscoded_CacheHitsAreNotRateLimited(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	src := filepath.Join(t.TempDir(), "shot.jpg")
	require.NoError(t, os.WriteFile(src, []byte("jpeg"), 0o644))
	seedPhoto(t, repo, "photo-1", "lib-1", src, "image/jpeg")

	cacheDir := t.TempDir()
	transcoder := &fakePhotoTranscoderWriter{}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1"})
	require.NoError(t, err)
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	mux := http.NewServeMux()
	// Budget of exactly one fork.
	rl := ratelimit.NewWithBurst(1, 60.0)
	defer rl.Stop()
	registerPhotoRoutes(mux, svc, tokenGen, allowAll(), nil, context.Background(), transcoder, cacheDir, rl)

	get := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-1/serve?w=400&fmt=webp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, get(), "the first request transcodes and spends the only token")
	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusOK, get(), "a cached thumbnail must not be charged to the fork budget")
	}
	require.Len(t, transcoder.widths, 1, "only the first request may fork")
}

func TestServeTranscoded_UncachedRequestsStillHitTheCeiling(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	dir := t.TempDir()
	for _, id := range []string{"photo-1", "photo-2"} {
		src := filepath.Join(dir, id+".jpg")
		require.NoError(t, os.WriteFile(src, []byte("jpeg"), 0o644))
		seedPhoto(t, repo, id, "lib-1", src, "image/jpeg")
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1"})
	require.NoError(t, err)
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, photofakes.NewFakePhotoScanner())
	mux := http.NewServeMux()
	rl := ratelimit.NewWithBurst(1, 60.0)
	defer rl.Stop()
	registerPhotoRoutes(mux, svc, tokenGen, allowAll(), nil, context.Background(), &fakePhotoTranscoderWriter{}, t.TempDir(), rl)

	get := func(id string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/"+id+"/serve?w=400&fmt=webp", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, get("photo-1"))
	require.Equal(t, http.StatusTooManyRequests, get("photo-2"), "a second fork must still be refused")
}
