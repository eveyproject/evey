package httpapi

import (
	"bytes"
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

	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	shared "github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// --- shared helpers ---

func genTokens(t *testing.T) (*auth.JWTGenerator, string, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	admin, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true, ProfileID: "prof-admin"})
	user, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false, ProfileID: "prof-1"})
	return gen, admin, user
}

// --- fakes for media handler tests ---

type fakeArtworkRepo struct {
	records map[string][]*domainmeta.ArtworkRecord
}

func (f *fakeArtworkRepo) GetByMediaItem(_ context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error) {
	return f.records[mediaItemID], nil
}

func (f *fakeArtworkRepo) UpdateSortOrder(_ context.Context, _ string, _ int) error { return nil }

func (f *fakeArtworkRepo) UpdateWebPPath(_ context.Context, _ string, _ string) error { return nil }

type fakeChaptersGetter struct {
	chapters []*domainlibrary.Chapter
	err      error
}

func (f *fakeChaptersGetter) GetChapters(_ context.Context, _ string) ([]*domainlibrary.Chapter, error) {
	return f.chapters, f.err
}

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

type fakeRemoteAccessStore struct {
	data map[string]string
}

func (f *fakeRemoteAccessStore) Get(_ context.Context, key string) (string, error) {
	v, ok := f.data[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeRemoteAccessStore) Set(_ context.Context, key, value string) error {
	f.data[key] = value
	return nil
}

// =====================================
// media_handler.go — stream, artwork, chapters, playbackInfo, selectArtwork
// =====================================

func TestMediaStream_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(mediaHandler(
		verifier,
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
		verifier,
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
		&fakeTokenVerifier{claims: testClaims(false)},
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		nil, "http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestMediaStream_TokenFromQueryParam(t *testing.T) {
	h := mediaHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
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

func TestMediaArtwork_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(mediaHandler(
		verifier,
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		"http://localhost",
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMediaArtwork_NotFound_404(t *testing.T) {
	h := mediaHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMediaArtwork_IDOR_403(t *testing.T) {
	h := mediaHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		&fakeArtworkRepo{},
		"http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestMediaArtwork_StreamToken_WrongMediaID_403(t *testing.T) {
	h := mediaHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
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

// --- chaptersHandler ---

func TestChaptersHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(chaptersHandler(
		verifier,
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestChaptersHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(chaptersHandler(
		verifier,
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestChaptersHandler_NotFound_404(t *testing.T) {
	h := chaptersHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{err: errTest},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestChaptersHandler_IDOR_403(t *testing.T) {
	h := chaptersHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeChaptersGetter{},
		"http://localhost",
		denyAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestChaptersHandler_OK(t *testing.T) {
	h := chaptersHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeChaptersGetter{chapters: []*domainlibrary.Chapter{
			{Title: "Ch1", StartTimeMs: 0, EndTimeMs: 60000},
		}},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Ch1") {
		t.Error("expected chapter title in response")
	}
}

func TestChaptersHandler_ServiceError_500(t *testing.T) {
	h := chaptersHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeChaptersGetter{err: errTest},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- playbackInfoHandler ---

func TestPlaybackInfo_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(playbackInfoHandler(
		verifier,
		&fakeMediaStreamsGetter{},
		allowAll(),
		nil,
	))
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
		verifier,
		&fakeMediaStreamsGetter{},
		allowAll(),
		nil,
	))
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
		nil,
	)
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", File: &domainlibrary.MediaFile{Path: "/a"}}},
		denyAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPlaybackInfo_OK(t *testing.T) {
	w1 := 1920
	h1 := 1080
	h := playbackInfoHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a.mkv", ContainerFormat: "matroska"}},
			streams: []*domainlibrary.MediaStream{
				{StreamType: "video", Codec: "h264", Width: &w1, Height: &h1},
				{StreamType: "audio", Codec: "aac", IsDefault: true},
			},
		},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPlaybackInfo_StreamsError_500(t *testing.T) {
	h := playbackInfoHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a.mkv"}},
			sErr: errTest,
		},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- selectArtworkHandler ---

func TestSelectArtwork_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAdminMiddleware(verifier)(selectArtworkHandler(
		verifier,
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSelectArtwork_NonAdmin_403(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	h := httpAdminMiddleware(verifier)(selectArtworkHandler(
		verifier,
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSelectArtwork_MediaNotFound_404(t *testing.T) {
	h := selectArtworkHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{err: errTest},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtwork_NoRecords_404(t *testing.T) {
	h := selectArtworkHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtwork_ArtworkIDNotFound_404(t *testing.T) {
	h := selectArtworkHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "other-art", MediaItemID: "m1", Type: "poster", SortOrder: 0}},
		}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtwork_OK(t *testing.T) {
	h := selectArtworkHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {
				{ID: "art1", MediaItemID: "m1", Type: "poster", SortOrder: 1},
				{ID: "art2", MediaItemID: "m1", Type: "poster", SortOrder: 0},
			},
		}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// --- downloadHandler extra tests ---

func TestDownloadHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(downloadHandler(
		verifier,
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDownloadHandler_StreamTokenContext(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent"}}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		Token: "tok", UserID: "u1", MediaItemID: "m1",
	})
	req = req.WithContext(ctx)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Error("stream token in context should be accepted")
	}
}

func TestDownloadHandler_EmptyFilePath_404(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =====================================
// cookie_middleware.go
// =====================================

func TestAuthCookieMiddleware_SetsCookieAndKeepsBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt-value"}`))
	})
	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "refreshToken") {
		t.Error("refreshToken should remain in body for native clients")
	}
	var foundCookie bool
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.Value == "rt-value" {
			foundCookie = true
		}
	}
	if !foundCookie {
		t.Error("expected refresh cookie to be set")
	}
}

func TestAuthCookieMiddleware_NoRefreshToken_PassThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"at"}`))
	})
	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "accessToken") {
		t.Error("body should be passed through")
	}
}

func TestAuthCookieMiddleware_NonJSON_PassThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	})
	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Body.String() != "not json" {
		t.Error("non-json should pass through")
	}
}

func TestInjectRefreshTokenFromCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-rt"})
	r2 := injectRefreshTokenFromCookie(req)
	var body map[string]any
	if err := json.NewDecoder(r2.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["refreshToken"] != "cookie-rt" {
		t.Errorf("expected refreshToken=cookie-rt, got %v", body["refreshToken"])
	}
}

func TestInjectRefreshTokenFromCookie_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	r2 := injectRefreshTokenFromCookie(req)
	if r2 != req {
		t.Error("without cookie, should return same request")
	}
}

func TestClearRefreshCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearRefreshCookie(w, false)
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.MaxAge < 0 {
			return
		}
	}
	t.Error("expected refresh cookie to be cleared")
}

func TestContainsJSON(t *testing.T) {
	if !containsJSON("application/json") {
		t.Error("should match application/json")
	}
	if !containsJSON("application/json; charset=utf-8") {
		t.Error("should match with charset")
	}
	if containsJSON("text/plain") {
		t.Error("should not match text/plain")
	}
}

// --- remote access routes ---

func newRemoteAccessMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{}
	store := &fakeRemoteAccessStore{data: make(map[string]string)}
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store, ratelimit.New(100))
	return mux, adminTok, userTok
}

func TestRemoteAccess_Get_NoAuth_401(t *testing.T) {
	mux, _, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRemoteAccess_Get_Admin_200(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_Admin_OK(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	body := `{"enabled":true,"externalUrl":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newRemoteAccessMux(t)
	body := `{"enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_BadBody_400(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader("bad"))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_InvalidExternalURL_400(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	body := `{"externalUrl":"not-a-url"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNetworkInfo_Admin_200(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/network-info", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- playback settings ---

func newPlaybackSettingsMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{Playback: config.PlaybackConfig{SkipForwardSeconds: 30, SkipBackwardSeconds: 10}}
	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, gen, ratelimit.New(100))
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

// =====================================
// auth_handler.go — parental routes, push unregister, listDevices/deleteDevice happy path
// =====================================

func newParentalMux(t *testing.T) (*http.ServeMux, *auth.JWTGenerator) {
	t.Helper()
	gen, _, _ := genTokens(t)
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-1",
		UserID: "user-1",
		Name:   "User Profile",
	})
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-admin",
		UserID: "admin-1",
		Name:   "Admin Profile",
	})
	hasher := userfakes.NewFakePasswordHasher()
	userRepo := userfakes.NewFakeUserRepository()
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "user-1", Username: "u1", PasswordHash: "hashed:secret"})
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "admin-1", Username: "a1", IsAdmin: true, PasswordHash: "hashed:secret"})
	mux := http.NewServeMux()
	registerParentalRoutes(mux, profileRepo, userRepo, hasher, gen)
	return mux, gen
}

func TestParentalGetSettings_NoAuth_401(t *testing.T) {
	mux, _ := newParentalMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/prof-1/parental", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestParentalGetSettings_IDOR_403(t *testing.T) {
	mux, gen := newParentalMux(t)
	// user-1 trying to access admin profile
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/prof-admin/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestParentalGetSettings_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/prof-1/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalGetSettings_NotFound_404(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", ProfileID: "prof-admin", IsAdmin: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/nonexistent/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestParentalUpdateSettings_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	body := `{"maxRating":"PG-13","blockedTags":["violence"],"enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalUpdateSettings_IDOR_403(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	body := `{"maxRating":"PG"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-admin/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestParentalVerifyPIN_NoPIN_Valid(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":"1234"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "true") {
		t.Error("expected valid=true when no PIN set")
	}
}

func TestParentalVerifyPIN_IDOR_403(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "attacker", ProfileID: "prof-attack", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":"1234"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================================
// helpers.go — middleware tests
// =====================================

func TestApiCacheMiddleware_SetsNoStore(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiCacheMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected no-store, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestApiCacheMiddleware_LibrariesCacheHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiCacheMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !strings.Contains(w.Header().Get("Cache-Control"), "max-age=300") {
		t.Errorf("expected max-age=300, got %q", w.Header().Get("Cache-Control"))
	}
}

// =====================================
// browse_handler.go — listDir coverage
// =====================================

func TestBrowseDirs_ValidMediaPath_OK(t *testing.T) {
	handler := browseDirsHandler(validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/media", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	// /media may not exist on test system; either 200 or 400 (symlink resolution)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400, got %d", w.Code)
	}
}

// =====================================
// helpers.go — safeJoin, sanitizeFilename, jsonEncodeResponse, setCORSHeaders, serveFileWithCache
// =====================================

func TestSafeJoin_PathTraversal(t *testing.T) {
	_, err := safeJoin("/tmp/safe", "../../etc/passwd")
	if err == nil {
		t.Error("expected path traversal error")
	}
}

func TestSafeJoin_Valid(t *testing.T) {
	path, err := safeJoin("/tmp/safe", "file.txt")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "file.txt") {
		t.Errorf("expected path ending with file.txt, got %q", path)
	}
}

func TestSanitizeFilename_PathChars(t *testing.T) {
	got := sanitizeFilename("../../etc/passwd")
	if strings.Contains(got, "/") {
		t.Errorf("should sanitize / chars, got %q", got)
	}
	if got == "" {
		t.Error("should not produce empty string")
	}
}

func TestSanitizeFilename_Empty(t *testing.T) {
	got := sanitizeFilename("")
	if got != "download" {
		t.Errorf("expected 'download', got %q", got)
	}
}

func TestJsonEncodeResponse_OK(t *testing.T) {
	w := httptest.NewRecorder()
	jsonEncodeResponse(w, map[string]string{"key": "val"})
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}
	if !strings.Contains(w.Body.String(), "key") {
		t.Error("expected key in response")
	}
}

func TestSetCORSHeaders_SameOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	setCORSHeaders(w, req, "http://localhost:3000")
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Error("expected CORS header for same origin")
	}
}

func TestSetCORSHeaders_DifferentOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	setCORSHeaders(w, req, "http://localhost:3000")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should not set CORS for different origin")
	}
}

func TestSetCORSHeaders_NoOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	setCORSHeaders(w, req, "http://localhost:3000")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should not set CORS without origin")
	}
}

func TestServeFileWithCache_NotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	serveFileWithCache(w, req, "/nonexistent_file_xyz")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDiscardResponseWriter(t *testing.T) {
	d := &discardResponseWriter{header: make(http.Header)}
	d.WriteHeader(http.StatusOK)
	n, err := d.Write([]byte("test"))
	if err != nil || n != 4 {
		t.Errorf("expected 4 bytes written, got %d err=%v", n, err)
	}
	if d.status != http.StatusOK {
		t.Errorf("expected 200, got %d", d.status)
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

func TestNewServerConfigStore(t *testing.T) {
	repo := sharedfakes.NewFakeServerConfigRepository()
	store := NewServerConfigStore(repo)
	ctx := context.Background()

	// Set
	if err := store.Set(ctx, "key1", "val1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Get
	val, err := store.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val != "val1" {
		t.Errorf("expected val1, got %q", val)
	}
	// Get missing
	_, err = store.Get(ctx, "missing")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

// =====================================
// libraryStatsHandler — extra coverage
// =====================================

func TestLibraryStatsHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(libraryStatsHandler(
		verifier,
		&fakeLibraryLister{},
		nil,
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_ServiceError_500(t *testing.T) {
	h := libraryStatsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeLibraryLister{err: errTest},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_QueryToken(t *testing.T) {
	h := libraryStatsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeLibraryLister{},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats?token=valid", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// =====================================
// helpers.go — exported middleware functions, tokenRedactMiddleware, securityHeaders
// =====================================

func TestTokenRedactMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// token should be in Authorization header now
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			t.Errorf("expected Bearer mytoken, got %q", r.Header.Get("Authorization"))
		}
		// query param should be stripped
		if r.URL.Query().Get("token") != "" {
			t.Error("token query param should be removed")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := tokenRedactMiddleware(nil, inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media?token=mytoken", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTokenRedactMiddleware_ExistingAuthHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer existing" {
			t.Errorf("existing header should not be overwritten, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := tokenRedactMiddleware(nil, inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media?token=mytoken", nil)
	req.Header.Set("Authorization", "Bearer existing")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
}

func TestSecurityHeadersMiddleware_Coverage(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeadersMiddleware(false, inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff header")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected DENY frame options")
	}
}

// =====================================
// media_handler.go — subtitleHandler auth tests
// =====================================

func TestSubtitleHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(subtitleHandler(
		verifier,
		&fakeMediaStreamsGetter{},
		"http://localhost",
		allowAll(),
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
		verifier,
		&fakeMediaStreamsGetter{},
		"http://localhost",
		allowAll(),
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "restricted", File: &domainlibrary.MediaFile{Path: "/a"}}},
		"http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleHandler_QueryToken(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		"http://localhost",
		allowAll(),
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
// server.go coverage — RegisterRoutes not feasible without many deps
// but we can test the exported middleware wrappers
// =====================================

func TestTokenRedactMiddleware_Exported(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := TokenRedactMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSecurityHeadersMiddleware_Exported(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := SecurityHeadersMiddleware(false, inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff")
	}
}

// =====================================
// media_handler.go — subtitleRoutes auth coverage
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
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================================
// auth_handler.go — parental with PIN
// =====================================

func TestParentalUpdateSettings_SetPIN_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234","currentPassword":"secret"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Regression: first-time PIN set without the account password must fail —
// otherwise a stolen JWT could lock the real owner out of their profile.
func TestParentalUpdateSettings_SetPIN_NoPassword_Forbidden(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalUpdateSettings_SetPIN_WrongPassword_Forbidden(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234","currentPassword":"wrong"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestParentalUpdateSettings_ClearPIN(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	empty := ""
	body, _ := json.Marshal(map[string]*string{"pin": &empty})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

