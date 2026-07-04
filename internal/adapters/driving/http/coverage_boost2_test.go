package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appmedia "github.com/evey-project/evey/internal/app/media"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
	subfakes "github.com/evey-project/evey/internal/domain/subtitle/fakes"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
)

// ============================================================
// Fakes for subtitle media getter (subtitle domain)
// ============================================================

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
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())
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
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

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
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, denyAll())

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
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================
// Stream token handler tests
// ============================================================

func TestStreamToken_ParentalBlocked_NotFound(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	// Parental must gate token issuance too: a kid that learns an adult media-id
	// out of band must not be handed a stream token for it.
	h := httpAuthMiddleware(gen)(streamTokenHandler(gen, store, allowAll(), mg, alwaysBlockChecker{}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("parental-blocked stream-token must return 404 (existence hidden), got %d", w.Code)
	}
}

func TestStreamToken_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := httpAuthMiddleware(gen)(streamTokenHandler(gen, store, allowAll(), mg, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == "" {
		t.Error("expected token in response")
	}
	if resp["mediaId"] != "m1" {
		t.Errorf("expected mediaId=m1, got %s", resp["mediaId"])
	}
}

func TestStreamToken_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := httpAuthMiddleware(gen)(streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter(), nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamToken_InvalidToken_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := httpAuthMiddleware(gen)(streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter(), nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamToken_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, allowAll(), newFakeMediaGetter(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", IsAdmin: false})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestStreamToken_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	h := streamTokenHandler(gen, store, denyAll(), mg, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, &domainuser.Claims{UserID: "u1", IsAdmin: false})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================
// HLS cache tests
// ============================================================

func TestCachedMediaItemGetter_CacheHit(t *testing.T) {
	inner := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	cached := NewCachedMediaItemGetter(inner)
	defer cached.Close()

	item, err := cached.GetMediaItem(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != "m1" {
		t.Errorf("expected m1, got %s", item.ID)
	}
	// second call hits cache
	inner.item = nil
	inner.err = errors.New("should not be called")
	item2, err := cached.GetMediaItem(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}
	if item2.ID != "m1" {
		t.Errorf("expected m1 from cache, got %s", item2.ID)
	}
}

func TestCachedMediaItemGetter_Error(t *testing.T) {
	inner := &fakeMediaItemGetter{err: errors.New("not found")}
	cached := NewCachedMediaItemGetter(inner)
	defer cached.Close()

	_, err := cached.GetMediaItem(context.Background(), "m1")
	if err == nil {
		t.Error("expected error")
	}
}

func TestCachedLibraryAccessChecker_CacheHit(t *testing.T) {
	inner := allowAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || !ok {
		t.Fatalf("expected access, got ok=%v err=%v", ok, err)
	}
	// second call with inner changed to deny — should still return cached true
	inner.allowed = false
	ok2, _ := cached.HasAccess(context.Background(), "u1", "lib1")
	if !ok2 {
		t.Error("expected cached access=true")
	}
}

func TestCachedLibraryAccessChecker_DeniedCacheHit(t *testing.T) {
	inner := denyAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || ok {
		t.Fatalf("expected denied, got ok=%v err=%v", ok, err)
	}
}

func TestCachedLibraryAccessChecker_Error(t *testing.T) {
	inner := &fakeLibraryAccessChecker{err: errors.New("db error")}
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	_, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err == nil {
		t.Error("expected error")
	}
}

// ============================================================
// Trickplay generate handler tests
// ============================================================

type fakeTrickplayGenerator struct {
	err error
}

func (f *fakeTrickplayGenerator) GenerateTrickplay(_ context.Context, _ string) error {
	return f.err
}

func TestTrickplayGenerate_OK(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestTrickplayGenerate_NonAdmin_403(t *testing.T) {
	v := &fakeTokenVerifier{claims: testClaims(false)}
	h := httpAdminMiddleware(v)(trickplayGenerateHandler(v, &fakeTrickplayGenerator{}, context.Background()))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTrickplayGenerate_NoAuth_403(t *testing.T) {
	v := &fakeTokenVerifier{err: errTest}
	h := httpAdminMiddleware(v)(trickplayGenerateHandler(v, &fakeTrickplayGenerator{}, context.Background()))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTrickplayGenerate_QueryToken(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// ============================================================
// Available runtimes route test
// ============================================================

func TestAvailableRuntimes_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	mux := http.NewServeMux()
	registerAvailableRuntimesRoute(mux, gen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugin-runtimes/available", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "runtimes") {
		t.Error("expected runtimes key in response")
	}
}

func TestAvailableRuntimes_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)

	mux := http.NewServeMux()
	registerAvailableRuntimesRoute(mux, gen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugin-runtimes/available", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ============================================================
// clearOIDCIDTokenCookie test
// ============================================================

func TestClearOIDCIDTokenCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCIDTokenCookie(w, true)
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == oidcIDTokenCookieName {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("expected MaxAge -1, got %d", c.MaxAge)
			}
			if !c.Secure {
				t.Error("expected Secure=true")
			}
		}
	}
	if !found {
		t.Error("expected OIDC cookie to be set")
	}
}

func TestClearOIDCIDTokenCookie_HTTP(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCIDTokenCookie(w, false)
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == oidcIDTokenCookieName && c.Secure {
			t.Error("expected Secure=false for http")
		}
	}
}

func TestStreamTokenFromContext_Missing(t *testing.T) {
	_, ok := StreamTokenFromContext(context.Background())
	if ok {
		t.Error("expected false for missing stream token")
	}
}

func TestStreamTokenFromContext_Present(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1"}
	ctx := context.WithValue(context.Background(), streamTokenCtxKey, st)
	got, ok := StreamTokenFromContext(ctx)
	if !ok {
		t.Fatal("expected true")
	}
	if got.UserID != "u1" {
		t.Errorf("expected u1, got %s", got.UserID)
	}
}

// ============================================================
// TokenRedactMiddlewareWithStore test
// ============================================================

type fakeStreamTokenValidator struct {
	token *shared.StreamToken
}

func (f *fakeStreamTokenValidator) ValidateAny(_ string) (*shared.StreamToken, bool) {
	if f.token != nil {
		return f.token, true
	}
	return nil, false
}

func TestTokenRedactMiddlewareWithStore_OpaqueToken(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1"}
	store := &fakeStreamTokenValidator{token: st}

	var capturedCtx context.Context
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
	})
	handler := TokenRedactMiddlewareWithStore(store, false, inner)

	req := httptest.NewRequest(http.MethodGet, "/test?token=opaque-no-dots", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got, ok := StreamTokenFromContext(capturedCtx)
	if !ok {
		t.Fatal("expected stream token in context")
	}
	if got.MediaItemID != "m1" {
		t.Errorf("expected m1, got %s", got.MediaItemID)
	}
}

func TestTokenRedactMiddlewareWithStore_JWTFallback(t *testing.T) {
	store := &fakeStreamTokenValidator{token: nil}

	var capturedAuth string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
	})
	handler := TokenRedactMiddlewareWithStore(store, false, inner)

	req := httptest.NewRequest(http.MethodGet, "/test?token=jwt.with.dots", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedAuth != "Bearer jwt.with.dots" {
		t.Errorf("expected JWT in auth header, got %q", capturedAuth)
	}
}

// Plan B P1: the stream token also resolves from the evey_hls cookie (native
// <video>/hls.js) and from an opaque Bearer header (mobile expo-video), scoped
// to /api/v1/media/*, and an HLS request (re)issues the httpOnly cookie.
func TestTokenRedactMiddleware_CookieAndBearerTransports(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1", TTL: 5 * time.Minute}
	store := &fakeStreamTokenValidator{token: st}
	newHandler := func() (http.Handler, *context.Context) {
		var captured context.Context
		inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r.Context() })
		return TokenRedactMiddlewareWithStore(store, false, inner), &captured
	}
	hlsCookieOf := func(w *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range w.Result().Cookies() {
			if c.Name == "evey_hls" {
				return c
			}
		}
		return nil
	}

	t.Run("cookie resolves token and master (re)issues it", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
		req.AddCookie(&http.Cookie{Name: "evey_hls", Value: "opaque-cookie"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); !ok {
			t.Fatal("cookie transport: expected stream token in context")
		}
		c := hlsCookieOf(w)
		if c == nil {
			t.Fatal("expected evey_hls Set-Cookie on HLS response")
		}
		if c.Path != "/api/v1/media/m1/hls" {
			t.Errorf("cookie path = %q, want /api/v1/media/m1/hls", c.Path)
		}
		if !c.HttpOnly {
			t.Error("cookie must be HttpOnly")
		}
	})

	t.Run("opaque Bearer resolves token (mobile)", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg_00001.m4s", nil)
		req.Header.Set("Authorization", "Bearer opaque-bearer")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); !ok {
			t.Error("bearer transport: expected stream token in context")
		}
	})

	t.Run("real JWT Bearer is not treated as a stream token", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
		req.Header.Set("Authorization", "Bearer header.body.sig")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); ok {
			t.Error("JWT bearer must not resolve as an opaque stream token")
		}
	})

	t.Run("cookie is ignored off the media paths", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/me", nil)
		req.AddCookie(&http.Cookie{Name: "evey_hls", Value: "opaque-cookie"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); ok {
			t.Error("cookie must not authenticate non-media paths")
		}
	})
}

// ============================================================
// discardResponseWriter test (helpers.go line 253)
// ============================================================

func TestDiscardResponseWriter_AllMethods(t *testing.T) {
	d := &discardResponseWriter{header: make(http.Header)}
	h := d.Header()
	if h == nil {
		t.Fatal("expected non-nil header")
	}
	h.Set("X-Test", "value")
	if d.Header().Get("X-Test") != "value" {
		t.Error("expected header to be set")
	}
	n, err := d.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("expected 5 nil, got %d %v", n, err)
	}
	d.WriteHeader(http.StatusCreated)
	if d.status != http.StatusCreated {
		t.Errorf("expected 201, got %d", d.status)
	}
}

// ============================================================
// extractBlockedIssues test (plugin_asset_handler.go)
// ============================================================

func TestExtractBlockedIssues_WithBrackets(t *testing.T) {
	msg := "plugin blocked: [uses eval()] [accesses localStorage]"
	issues := extractBlockedIssues(msg)
	if len(issues) < 2 {
		t.Fatalf("expected at least 2 issues, got %d: %v", len(issues), issues)
	}
}

func TestExtractBlockedIssues_NoBrackets(t *testing.T) {
	msg := "some error"
	issues := extractBlockedIssues(msg)
	if len(issues) != 1 || issues[0] != msg {
		t.Errorf("expected [%q], got %v", msg, issues)
	}
}

// claimsFromContext tests removed (share_handler migrated to gRPC)

// ============================================================
// transcode parseParams tests
// ============================================================

func TestTranscodeParseParams_Default(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := h.parseParams(req)
	if p.audioTrack != 0 {
		t.Errorf("expected audioTrack=0, got %d", p.audioTrack)
	}
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0, got %d", p.videoScale)
	}
	if p.subtitleTrack != -1 {
		t.Errorf("expected subtitleTrack=-1, got %d", p.subtitleTrack)
	}
	if p.remuxOnly {
		t.Error("expected remuxOnly=false")
	}
	if p.startTimeSec != 0 {
		t.Errorf("expected startTimeSec=0, got %f", p.startTimeSec)
	}
}

func TestTranscodeParseParams_AllSet(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?audioTrack=2&videoScale=720&subtitleTrack=1&remux=1&startTime=30.5", nil)
	p := h.parseParams(req)
	if p.audioTrack != 2 {
		t.Errorf("expected audioTrack=2, got %d", p.audioTrack)
	}
	if p.videoScale != 720 {
		t.Errorf("expected videoScale=720, got %d", p.videoScale)
	}
	if p.subtitleTrack != 1 {
		t.Errorf("expected subtitleTrack=1, got %d", p.subtitleTrack)
	}
	if !p.remuxOnly {
		t.Error("expected remuxOnly=true")
	}
	if p.startTimeSec != 30.5 {
		t.Errorf("expected startTimeSec=30.5, got %f", p.startTimeSec)
	}
}

func TestTranscodeParseParams_InvalidValues(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?audioTrack=abc&videoScale=-1&subtitleTrack=-5&startTime=bad", nil)
	p := h.parseParams(req)
	if p.audioTrack != 0 {
		t.Errorf("expected audioTrack=0 for invalid, got %d", p.audioTrack)
	}
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0 for invalid, got %d", p.videoScale)
	}
	if p.subtitleTrack != -1 {
		t.Errorf("expected subtitleTrack=-1 for invalid, got %d", p.subtitleTrack)
	}
}

func TestTranscodeParseParams_VideoScaleTooHigh(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?videoScale=9999", nil)
	p := h.parseParams(req)
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0 for >4320, got %d", p.videoScale)
	}
}

// ============================================================
// transcode analyzeStreams tests
// ============================================================

func TestAnalyzeStreams_H264_AAC_DirectPlay(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.needsVideoTranscode {
		t.Error("h264 should not need video transcode")
	}
	if a.needsAudioTranscode {
		t.Error("aac should not need audio transcode")
	}
}

func TestAnalyzeStreams_HEVC_NeedsTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if !a.needsVideoTranscode {
		t.Error("hevc should need video transcode")
	}
}

func TestAnalyzeStreams_RemuxOnly_NoVideoTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", true, domaintranscoding.DefaultWebProfile, -1)
	if a.needsVideoTranscode {
		t.Error("remux mode should not trigger video transcode")
	}
}

func TestAnalyzeStreams_NonAACAudio_NeedsTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "dts", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if !a.needsAudioTranscode {
		t.Error("dts should need audio transcode")
	}
}

func TestAnalyzeStreams_OpusAudio_NoCopy(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "opus", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.needsAudioTranscode {
		t.Error("opus should not need audio transcode")
	}
}

func TestAnalyzeStreams_MP3Audio_NoCopy(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "mp3", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.needsAudioTranscode {
		t.Error("mp3 should not need audio transcode")
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

func TestAnalyzeStreams_StreamsError(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		sErr: errors.New("db error"),
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.streams != nil {
		t.Error("expected nil streams on error")
	}
	if !a.needsAudioTranscode {
		t.Error("default should be needsAudioTranscode=true")
	}
}

func TestAnalyzeStreams_HEVC_EAC3_Firefox_TranscodeBoth(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}}
	firefox := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	}
	a := h.analyzeStreams(context.Background(), "m1", false, firefox, -1)
	if !a.needsVideoTranscode {
		t.Error("HEVC should need transcode on Firefox")
	}
	if !a.needsAudioTranscode {
		t.Error("EAC3 should need transcode on Firefox")
	}
}

func TestAnalyzeStreams_HEVC_EAC3_Safari_CopyBoth(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}}
	safari := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "hevc", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac", "ac3", "eac3"},
	}
	a := h.analyzeStreams(context.Background(), "m1", false, safari, -1)
	if a.needsVideoTranscode {
		t.Error("HEVC should copy on Safari")
	}
	if a.needsAudioTranscode {
		t.Error("EAC3 should copy on Safari")
	}
}

func TestAnalyzeStreams_HEVC_EAC3_PartialSupport(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}}
	client := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "hevc"},
		AudioCodecs: []string{"aac", "mp3", "opus"},
	}
	a := h.analyzeStreams(context.Background(), "m1", false, client, -1)
	if a.needsVideoTranscode {
		t.Error("HEVC should copy when client supports it")
	}
	if !a.needsAudioTranscode {
		t.Error("EAC3 should transcode when client doesn't support it")
	}
}

// ============================================================
// transcode resolveSourceDimensions tests
// ============================================================

// ============================================================
// transcode resolveSubtitle tests
// ============================================================

// ============================================================
// parseClientProfile tests
// ============================================================

func TestParseClientProfile_Default(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := h.parseClientProfile(req)
	if p.MaxWidth == 0 || p.MaxHeight == 0 {
		t.Error("expected non-zero default dimensions")
	}
}

func TestParseClientProfile_AllParams(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?maxWidth=1280&maxHeight=720&maxBitrate=5000000&videoCodecs=h264,hevc&audioCodecs=aac,opus&containers=mp4,mkv&hdr=true", nil)
	p := h.parseClientProfile(req)
	if p.MaxWidth != 1280 {
		t.Errorf("expected 1280, got %d", p.MaxWidth)
	}
	if p.MaxHeight != 720 {
		t.Errorf("expected 720, got %d", p.MaxHeight)
	}
	if p.MaxBitrate != 5000000 {
		t.Errorf("expected 5000000, got %d", p.MaxBitrate)
	}
	if len(p.VideoCodecs) != 2 || p.VideoCodecs[0] != "h264" {
		t.Errorf("unexpected video codecs: %v", p.VideoCodecs)
	}
	if len(p.AudioCodecs) != 2 {
		t.Errorf("unexpected audio codecs: %v", p.AudioCodecs)
	}
	if len(p.Containers) != 2 {
		t.Errorf("unexpected containers: %v", p.Containers)
	}
	if !p.SupportsHDR {
		t.Error("expected SupportsHDR=true")
	}
}

func TestParseClientProfile_InvalidValues(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?maxWidth=abc&maxHeight=-1&maxBitrate=bad", nil)
	p := h.parseClientProfile(req)
	assert.NotZero(t, p.MaxWidth, "invalid should keep default, which is non-zero")
	if p.SupportsHDR {
		t.Error("expected SupportsHDR=false without hdr=true")
	}
}

// ============================================================
// Plugin handleListThemes test
// ============================================================

func TestListThemes_OK(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	_ = repo.Create(context.Background(), &plugin.Plugin{
		ID: "theme1", Name: "Dark Theme", Kind: plugin.KindTheme, Enabled: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Dark Theme") {
		t.Error("expected theme name in response")
	}
}

func TestListThemes_NoAuth_401(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListThemes_IncludesParsedTokens(t *testing.T) {
	mux, token, repo, dir := setupPluginAssetTest(t)
	_ = repo.Create(context.Background(), &plugin.Plugin{
		ID: "themeWithCSS", Name: "Mid Theme", Kind: plugin.KindTheme, Enabled: true,
	})
	themeDir := filepath.Join(dir, "themeWithCSS")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	css := []byte("/* hi */\n:root {\n  --bg: #112233;\n  --accent: #ff00ff;\n  --text: #eee;\n}\n")
	if err := os.WriteFile(filepath.Join(themeDir, "theme.css"), css, 0o644); err != nil {
		t.Fatalf("write theme.css: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"--bg":"#112233"`, `"--accent":"#ff00ff"`, `"--text":"#eee"`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

func TestParseRootCSSVars(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "basic",
			in:   ":root { --a: red; --b: #fff; }",
			want: map[string]string{"--a": "red", "--b": "#fff"},
		},
		{
			name: "with comments",
			in:   "/* skip */ :root { /* inner */ --x: blue; }",
			want: map[string]string{"--x": "blue"},
		},
		{
			name: "ignores non-vars",
			in:   ":root { color: red; --keep: yellow; }",
			want: map[string]string{"--keep": "yellow"},
		},
		{
			name: "no root block",
			in:   "body { color: red; }",
			want: nil,
		},
		{
			name: "rgba value preserved",
			in:   ":root { --shadow: rgba(0, 0, 0, 0.4); }",
			want: map[string]string{"--shadow": "rgba(0, 0, 0, 0.4)"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRootCSSVars([]byte(c.in))
			if len(got) != len(c.want) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(c.want), got)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("key %s: got %q want %q", k, got[k], v)
				}
			}
		})
	}
}

// ============================================================
// Plugin upload route test
// ============================================================

func TestPluginUpload_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPluginUpload_NonAdmin_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPluginUpload_NoFile_400(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "a1", IsAdmin: true})
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
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
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

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
// Subtitle list with items
// ============================================================

// ============================================================
// HLS segment handler auth tests
// ============================================================

// HLSSegmentHandler_QueryToken skipped — requires non-nil HLSManager to avoid panic after auth

// ============================================================
// Transcode handler auth tests
// ============================================================

func TestTranscodeHandler_NoAuth_401(t *testing.T) {
	v := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(v)(transcodeHandler(transcodeServe{
		tokens: v, libSvc: &fakeMediaStreamsGetter{}, baseURL: "http://localhost", libraryAccess: allowAll(),
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeHandler_InvalidToken_401(t *testing.T) {
	v := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(v)(transcodeHandler(transcodeServe{
		tokens: v, libSvc: &fakeMediaStreamsGetter{}, baseURL: "http://localhost", libraryAccess: allowAll(),
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeHandler_NotFound_404(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		libSvc:        &fakeMediaStreamsGetter{itemErr: errTest},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTranscodeHandler_IDOR_403(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		libSvc:        &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", File: &domainlibrary.MediaFile{Path: "/a"}}},
		baseURL:       "http://localhost",
		libraryAccess: denyAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTranscodeHandler_QueryToken(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		libSvc:        &fakeMediaStreamsGetter{itemErr: errTest},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

func TestTranscodeHandler_MaxStreams_429(t *testing.T) {
	throttle := apptranscoding.NewTranscodeThrottle()
	maxStreams := 1
	fakeMaxStreams := &fakeUserMaxStreams{max: &maxStreams}
	throttle.Acquire("u1", 0) // occupy one slot
	h := transcodeHandler(transcodeServe{
		tokens:        &fakeTokenVerifier{claims: testClaims(false)},
		libSvc:        &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a"}}},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(), throttle: throttle, userMaxStreams: fakeMaxStreams,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	throttle.Release("u1")
}

type fakeUserMaxStreams struct {
	max *int
}

func (f *fakeUserMaxStreams) GetMaxStreams(_ context.Context, _ string) *int {
	return f.max
}

// ============================================================
// HLS master handler auth tests
// ============================================================

// ============================================================
// Photo handler additional tests
// ============================================================

func TestServePhoto_NotFound(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-nonexistent/serve", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass, got %d", w.Code)
	}
}

// ============================================================
// Backup handler additional tests
// ============================================================

func TestCreateBackup_Admin_201(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetBackup_NotFound_404(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetBackup_NoAuth_401(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/b1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDeleteBackup_NotFound_404(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteBackup_NoAuth_401(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/b1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRestoreBackup_NoAuth_401(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRestoreBackup_NoFile_400(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDeleteBackup_AfterCreate(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	// create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	backupID := created["id"].(string)

	// delete
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+backupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestGetBackup_AfterCreate(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	backupID := created["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/"+backupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/gzip" {
		t.Errorf("expected gzip content type, got %s", w.Header().Get("Content-Type"))
	}
}

// ============================================================
// subtitleHandler (old-style, file stream) auth tests
// ============================================================

func TestSubtitleFileHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(subtitleHandler(
		verifier,
		&fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
		nil,
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
		verifier,
		&fakeMediaStreamsGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
		nil,
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
		nil,
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted", File: &domainlibrary.MediaFile{Path: "/a"}}},
		"http://localhost",
		denyAll(),
		nil,
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
		nil,
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

// ============================================================
// buildFFmpegArgs test (transcode_handler.go — pure logic, no I/O)
// ============================================================

func TestBuildFFmpegArgs_CopyAll(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{})
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}
	found := false
	for _, a := range args {
		if a == "-c:v" {
			found = true
		}
	}
	if !found {
		t.Error("expected -c:v in args")
	}
}

func TestBuildFFmpegArgs_VideoTranscode(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsVideoTranscode: true, needsAudioTranscode: true})
	hasLibx264 := false
	hasAAC := false
	for _, a := range args {
		if a == "libx264" {
			hasLibx264 = true
		}
		if a == "aac" {
			hasAAC = true
		}
	}
	if !hasLibx264 {
		t.Error("expected libx264 for video transcode")
	}
	if !hasAAC {
		t.Error("expected aac for audio transcode")
	}
}

func TestBuildFFmpegArgs_VideoScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{videoScale: 720, subtitleTrack: -1}, transcodeAnalysis{})
	hasScale := false
	for _, a := range args {
		if strings.Contains(a, "scale=-2:720") {
			hasScale = true
		}
	}
	if !hasScale {
		t.Error("expected scale filter for videoScale=720")
	}
}

func TestBuildFFmpegArgs_WithStartTime(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{startTimeSec: 30.5, subtitleTrack: -1}, transcodeAnalysis{})
	hasSS := false
	for _, a := range args {
		if a == "-ss" {
			hasSS = true
		}
	}
	if !hasSS {
		t.Error("expected -ss for startTimeSec>0")
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

func TestBuildFFmpegArgs_AudioCopy(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsAudioTranscode: false})
	audioCopy := false
	for i, a := range args {
		if a == "-c:a" && i+1 < len(args) && args[i+1] == "copy" {
			audioCopy = true
		}
	}
	if !audioCopy {
		t.Error("expected -c:a copy when no audio transcode needed")
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
	registerSubtitleRoutes(mux, svc, gen, httpMediaGetter, allowAll())

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
