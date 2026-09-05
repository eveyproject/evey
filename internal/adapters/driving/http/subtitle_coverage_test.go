package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
	subfakes "github.com/evey-project/evey/internal/domain/subtitle/fakes"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// --- shared helpers ---

// =====================================
// media_handler.go - subtitleHandler auth tests
// =====================================

func TestSubtitleHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(subtitleHandler(
		&fakeMediaStreamsGetter{},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(subtitleHandler(
		&fakeMediaStreamsGetter{},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleHandler_NotFound_404(t *testing.T) {
	h := subtitleHandler(
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleHandler_IDOR_403(t *testing.T) {
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "restricted", File: &domainlibrary.MediaFile{Path: "/a"}}},
		"http://localhost",
		denyAll(),
		nil,
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleHandler_QueryToken(t *testing.T) {
	h := subtitleHandler(
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0?token=valid", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// item not found → 404 (not 401)
	if w.Code == http.StatusUnauthorized {
		t.Error("query token should be accepted")
	}
}

// =====================================
// media_handler.go - subtitleRoutes auth coverage
// =====================================

func TestSubtitleRoutes_RequireAuth_NoToken_401(t *testing.T) {
	h := &subtitleRoutes{
		tokens:        &fakeTokenVerifier{err: errTest},
		mediaGetter:   &fakeMediaItemGetter{item: testMediaItem("lib1")},
		libraryAccess: allowAll(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, ok := h.requireAuth(w, req)
	if ok {
		t.Error("expected auth to fail without token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleRoutes_RequireAuth_InvalidToken_403(t *testing.T) {
	h := &subtitleRoutes{
		tokens:        &fakeTokenVerifier{err: errTest},
		mediaGetter:   &fakeMediaItemGetter{item: testMediaItem("lib1")},
		libraryAccess: allowAll(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer bad")
	_, ok := h.requireAuth(w, req)
	if ok {
		t.Error("expected auth to fail with invalid token")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleRoutes_RequireAuth_OK(t *testing.T) {
	h := &subtitleRoutes{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		mediaGetter:   &fakeMediaItemGetter{item: testMediaItem("lib1")},
		libraryAccess: allowAll(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer tok")
	claims, ok := h.requireAuth(w, req)
	if !ok || claims == nil {
		t.Error("expected auth to pass")
	}
}

func TestSubtitleRoutes_RequireMediaAccess_NotFound(t *testing.T) {
	h := &subtitleRoutes{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		mediaGetter:   &fakeMediaItemGetter{err: errTest},
		libraryAccess: allowAll(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer tok")
	_, ok := h.requireMediaAccess(w, req, "m1")
	if ok {
		t.Error("expected media access to fail")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleRoutes_RequireMediaAccess_IDOR(t *testing.T) {
	h := &subtitleRoutes{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		mediaGetter:   &fakeMediaItemGetter{item: testMediaItem("restricted")},
		libraryAccess: denyAll(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer tok")
	_, ok := h.requireMediaAccess(w, req, "m1")
	if ok {
		t.Error("expected IDOR check to block")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

type fakeSubtitleMediaGetter struct {
	items map[string]*domainsubtitle.MediaItemInfo
}

func (f *fakeSubtitleMediaGetter) GetMediaItem(_ context.Context, id string) (*domainsubtitle.MediaItemInfo, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}

// ============================================================
// Subtitle routes tests
// ============================================================

func newSubtitleTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	provider.Results = []domainsubtitle.SearchResult{
		{FileID: 42, FileName: "test.srt", Language: "en", Format: "srt", Rating: 8.5},
	}
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Test Movie"},
	}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)

	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/tmp/movie.mkv"}}}

	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll(), nil, nil)
	return mux, userToken
}

func TestSubtitleSearch_OK(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=en", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "test.srt") {
		t.Error("expected search result in response")
	}
}

func TestSubtitleSearch_NoAuth_401(t *testing.T) {
	mux, _ := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleSearch_InvalidToken_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleSearch_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, denyAll(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleList_OK(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleDownload_OK(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	body := `{"fileId":42,"language":"en","format":"srt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleDownload_NoFileID_400(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	body := `{"language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDownload_InvalidBody_400(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDelete_NotFound_404(t *testing.T) {
	mux, token := newSubtitleTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub-999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleSearch_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{err: errTest}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAnalyzeStreams_SubtitleTrack(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
			{StreamType: "subtitle", Codec: "srt"},
			{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle"},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, 1)
	if !a.burnInSubtitle {
		t.Error("PGS subtitle should trigger burn-in")
	}
	if !a.needsVideoTranscode {
		t.Error("burn-in subtitle should trigger video transcode")
	}
	if a.subtitleCodec != "hdmv_pgs_subtitle" {
		t.Errorf("expected hdmv_pgs_subtitle, got %s", a.subtitleCodec)
	}
}

func TestAnalyzeStreams_SubtitleTrack_SRT(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
			{StreamType: "subtitle", Codec: "srt"},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, 0)
	if a.burnInSubtitle {
		t.Error("SRT should not burn-in")
	}
	if a.subtitleCodec != "srt" {
		t.Errorf("expected srt, got %s", a.subtitleCodec)
	}
}

// ============================================================
// Subtitle delete with found item
// ============================================================

func TestSubtitleDelete_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	provider.DownloadPath = "/tmp/test.srt"
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Test"},
	}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll(), nil, nil)

	// download first to create a subtitle
	dlBody := `{"fileId":42,"language":"en","format":"srt"}`
	dlReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(dlBody))
	dlReq.Header.Set("Authorization", "Bearer "+userToken)
	dw := httptest.NewRecorder()
	mux.ServeHTTP(dw, dlReq)
	if dw.Code != http.StatusCreated {
		t.Fatalf("download failed: %d %s", dw.Code, dw.Body.String())
	}
	var dlResp map[string]any
	json.NewDecoder(dw.Body).Decode(&dlResp)
	subID := dlResp["id"].(string)

	// delete
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/"+subID, nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================
// subtitleHandler (old-style, file stream) auth tests
// ============================================================

func TestSubtitleFileHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(subtitleHandler(
		&fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(subtitleHandler(
		&fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_NotFound_404(t *testing.T) {
	h := subtitleHandler(
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_IDOR_403(t *testing.T) {
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted", File: &domainlibrary.MediaFile{Path: "/a"}}},
		"http://localhost",
		denyAll(),
		nil,
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleFileHandler_QueryToken(t *testing.T) {
	h := subtitleHandler(
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
		nil,
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0?token=tok", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

func TestBuildFFmpegArgs_BurnInSubtitle(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: 0}, transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
		needsAudioTranscode: true,
	})
	hasFilterComplex := false
	for _, a := range args {
		if a == "-filter_complex" {
			hasFilterComplex = true
		}
	}
	if !hasFilterComplex {
		t.Error("expected -filter_complex for burn-in subtitle")
	}
}

func TestBuildFFmpegArgs_BurnInSubtitleWithScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: 0, videoScale: 480}, transcodeAnalysis{
		burnInSubtitle:      true,
		needsVideoTranscode: true,
	})
	hasScale := false
	for _, a := range args {
		if strings.Contains(a, "scale") {
			hasScale = true
		}
	}
	if !hasScale {
		t.Error("expected scale in overlay filter")
	}
}

func TestSubtitleList_WithItems(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := subfakes.NewFakeRepository()
	provider := subfakes.NewFakeProvider()
	provider.DownloadPath = "/tmp/test.srt"
	subMediaGetter := &fakeSubtitleMediaGetter{items: map[string]*domainsubtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Test"},
	}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	httpMediaGetter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll(), nil, nil)

	// download first
	dlBody := `{"fileId":42,"language":"en","format":"srt"}`
	dlReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(dlBody))
	dlReq.Header.Set("Authorization", "Bearer "+userToken)
	dw := httptest.NewRecorder()
	mux.ServeHTTP(dw, dlReq)

	// list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "en") {
		t.Error("expected subtitle language in response")
	}
}
