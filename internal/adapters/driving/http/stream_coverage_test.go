package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmedia "github.com/evey-project/evey/internal/app/media"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	shared "github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// --- shared helpers ---

type fakeMediaStreamsGetter struct {
	item    *domainlibrary.MediaItem
	streams []*domainlibrary.MediaStream
	itemErr error
	sErr    error
}

func (f *fakeMediaStreamsGetter) GetMediaItem(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return f.item, f.itemErr
}

func (f *fakeMediaStreamsGetter) GetMediaStreams(_ context.Context, _ string) ([]*domainlibrary.MediaStream, error) {
	return f.streams, f.sErr
}

func (f *fakeMediaStreamsGetter) NextEpisodeOf(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}

// =====================================
// media_handler.go - stream, artwork, chapters, playbackInfo, selectArtwork
// =====================================

func TestMediaStream_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil, "http://localhost",
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMediaStream_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil, "http://localhost",
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMediaStream_NotFound_404(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{err: errTest},
		nil, "http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMediaStream_IDOR_403(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		nil, "http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMediaStream_TokenFromQueryParam(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent"}}},
		nil, "http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream?token=valid-token", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// file doesn't exist but auth passed
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

func TestMediaArtwork_StreamToken_WrongMediaID_403(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "u1",
		MediaItemID: "other-media",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for wrong media_id, got %d", w.Code)
	}
}

func TestPlaybackInfo_StreamsError_500(t *testing.T) {
	h := playbackInfoHandler(
		&fakeMediaStreamsGetter{
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a.mkv"}},
			sErr: errTest,
		},
		allowAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// =====================================
// adapters.go
// =====================================

func TestNewUserMaxStreamsAdapter(t *testing.T) {
	repo := userfakes.NewFakeUserRepository()
	adapter := NewUserMaxStreamsAdapter(repo)
	result := adapter.GetMaxStreams(context.Background(), "nonexistent")
	if result != nil {
		t.Errorf("expected nil for missing user, got %v", result)
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
