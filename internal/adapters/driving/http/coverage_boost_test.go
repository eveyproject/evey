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

	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmusic "github.com/gravity-zero/evey/internal/app/music"
	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	apprating "github.com/gravity-zero/evey/internal/app/rating"
	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/config"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	metafakes "github.com/gravity-zero/evey/internal/domain/metadata/fakes"
	musicfakes "github.com/gravity-zero/evey/internal/domain/music/fakes"
	notifakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	domainphoto "github.com/gravity-zero/evey/internal/domain/photo"
	playlistfakes "github.com/gravity-zero/evey/internal/domain/playlist/fakes"
	ratingfakes "github.com/gravity-zero/evey/internal/domain/rating/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	transcodingfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
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
	h := mediaHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil, "http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMediaStream_InvalidToken_403(t *testing.T) {
	h := mediaHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil, "http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestMediaStream_TokenFromQueryParam(t *testing.T) {
	h := mediaHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/nonexistent"}},
		nil, "http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream?token=valid-token", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// file doesn't exist but auth passed
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

func TestMediaArtwork_NoAuth_401(t *testing.T) {
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- chaptersHandler ---

func TestChaptersHandler_NoAuth_401(t *testing.T) {
	h := chaptersHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestChaptersHandler_InvalidToken_403(t *testing.T) {
	h := chaptersHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- playbackInfoHandler ---

func TestPlaybackInfo_NoAuth_401(t *testing.T) {
	h := playbackInfoHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaStreamsGetter{},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfo_InvalidToken_403(t *testing.T) {
	h := playbackInfoHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaStreamsGetter{},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPlaybackInfo_NotFound_404(t *testing.T) {
	h := playbackInfoHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfo_IDOR_403(t *testing.T) {
	h := playbackInfoHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", FilePath: "/a"}},
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
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
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/a.mkv", ContainerFormat: "matroska"},
			streams: []*domainlibrary.MediaStream{
				{StreamType: "video", Codec: "h264", Width: &w1, Height: &h1},
				{StreamType: "audio", Codec: "aac", IsDefault: true},
			},
		},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
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
			item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/a.mkv"},
			sErr: errTest,
		},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- selectArtworkHandler ---

func TestSelectArtwork_NoAuth_401(t *testing.T) {
	h := selectArtworkHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	)
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
	h := selectArtworkHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeArtworkRepo{},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		allowAll(),
	)
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
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
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// --- downloadHandler extra tests ---

func TestDownloadHandler_InvalidToken_403(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDownloadHandler_QueryToken(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/nonexistent"}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download?token=valid", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Error("query token should be accepted")
	}
}

func TestDownloadHandler_EmptyFilePath_404(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: ""}},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =====================================
// admin_handler.go — requireAdminWithResponse, server config, transcode cache, user admin, media, refresh-all
// =====================================

func newServerConfigMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	mux := http.NewServeMux()
	registerServerConfigRoutes(mux, cfgRepo, gen)
	return mux, adminTok, userTok
}

func TestServerConfig_List_NoAuth_401(t *testing.T) {
	mux, _, _ := newServerConfigMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestServerConfig_List_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newServerConfigMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-config", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestServerConfig_List_Admin_200(t *testing.T) {
	mux, adminTok, _ := newServerConfigMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-config", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServerConfig_Set_Admin_204(t *testing.T) {
	mux, adminTok, _ := newServerConfigMux(t)
	body := `{"value":"test-val"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-config/test-key", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestServerConfig_Set_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newServerConfigMux(t)
	body := `{"value":"test-val"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-config/test-key", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestServerConfig_Set_BadBody_400(t *testing.T) {
	mux, adminTok, _ := newServerConfigMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-config/k", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestServerConfig_Delete_Admin_204(t *testing.T) {
	mux, adminTok, _ := newServerConfigMux(t)
	// Set first
	setReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/server-config/del-key", strings.NewReader(`{"value":"v"}`))
	setReq.Header.Set("Authorization", "Bearer "+adminTok)
	setW := httptest.NewRecorder()
	mux.ServeHTTP(setW, setReq)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/server-config/del-key", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestServerConfig_Delete_NonExistent_204or404(t *testing.T) {
	mux, adminTok, _ := newServerConfigMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/server-config/missing", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// fake repo may return error (404) or succeed (204)
	if w.Code != http.StatusNotFound && w.Code != http.StatusNoContent {
		t.Errorf("expected 404 or 204, got %d", w.Code)
	}
}

// --- transcode cache ---

func newTranscodeCacheMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	profileRepo := transcodingfakes.NewFakeTranscodeProfileRepository()
	jobRepo := transcodingfakes.NewFakeTranscodeJobRepository()
	transcoder := transcodingfakes.NewFakeTranscoder()
	hw := transcodingfakes.NewFakeHWDetector()
	bus := sharedfakes.NewFakeEventBus()
	cache := transcodingfakes.NewFakeCacheStore()
	svc := apptranscoding.NewService(transcoder, hw, jobRepo, profileRepo, bus, 0)
	svc.WithCacheStore(cache)
	mux := http.NewServeMux()
	registerTranscodeCacheRoutes(mux, svc, gen, &fakeMediaItemGetter{})
	return mux, adminTok, userTok
}

func TestTranscodeCache_Get_NoAuth_403(t *testing.T) {
	mux, _, _ := newTranscodeCacheMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTranscodeCache_Get_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newTranscodeCacheMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTranscodeCache_Get_Admin_200(t *testing.T) {
	mux, adminTok, _ := newTranscodeCacheMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTranscodeCache_Clear_Admin_204(t *testing.T) {
	mux, adminTok, _ := newTranscodeCacheMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestTranscodeCache_DeleteFile_NotFound(t *testing.T) {
	mux, adminTok, _ := newTranscodeCacheMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/transcode-cache/missing.mp4", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =====================================
// collection_handler.go — getByID, delete, routeByID
// =====================================

func newCollectionFullTestMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cmdRepo := libfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	queryRepo := libfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)
	mux := http.NewServeMux()
	registerCollectionRoutes(mux, svc, gen)
	return mux, adminTok, userTok
}

func TestCollectionGetByID_NoAuth_401(t *testing.T) {
	mux, _, _ := newCollectionFullTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/col-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCollectionGetByID_NotFound_404(t *testing.T) {
	mux, _, userTok := newCollectionFullTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCollectionDelete_NoAuth_401(t *testing.T) {
	mux, _, _ := newCollectionFullTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/collections/col-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCollectionDelete_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newCollectionFullTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/collections/col-1", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCollectionDelete_Admin_NotFound_404(t *testing.T) {
	mux, adminTok, _ := newCollectionFullTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/collections/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCollectionRouteByID_MethodNotAllowed(t *testing.T) {
	mux, _, userTok := newCollectionFullTestMux(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/collections/col-1", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// =====================================
// playlist_handler.go — routeByID, getByID, update, delete, addItem, removeItem, reorder
// =====================================

func newPlaylistFullTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	gen, _, _ := genTokens(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	querier := playlistfakes.NewFakeSmartPlaylistQuerier()
	svc := appplaylist.NewService(plRepo, itemRepo, querier)
	mux := http.NewServeMux()
	registerPlaylistRoutes(mux, svc, gen)
	return mux, token
}

func TestPlaylistCreate_BadBody_400(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlaylistCreate_EmptyName_400(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlaylistGetByID_NotFound_404(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaylistUpdate_NotFound_404(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "Updated"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/nonexistent", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaylistDelete_NotFound_404(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/playlists/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaylistAddItem_BadBody_400(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists/pl-1/items", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlaylistRemoveItem_NotFound_404(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/playlists/pl-1/items/m1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaylistReorder_BadBody_400(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/pl-1/reorder", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlaylistRouteByID_MethodNotAllowed(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/playlists/pl-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestPlaylistCRUD_Flow(t *testing.T) {
	mux, token := newPlaylistFullTestMux(t)

	// Create
	body, _ := json.Marshal(map[string]string{"name": "Test PL", "type": "manual"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", createW.Code)
	}
	var createResp map[string]any
	_ = json.NewDecoder(createW.Body).Decode(&createResp)
	plID, _ := createResp["id"].(string)

	// Get
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/playlists/"+plID, nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getW := httptest.NewRecorder()
	mux.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Errorf("get: expected 200, got %d body=%s", getW.Code, getW.Body.String())
	}

	// Update
	updateBody, _ := json.Marshal(map[string]string{"name": "Updated PL"})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/playlists/"+plID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateW := httptest.NewRecorder()
	mux.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusNoContent {
		t.Errorf("update: expected 204, got %d", updateW.Code)
	}

	// Delete
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/playlists/"+plID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delW := httptest.NewRecorder()
	mux.ServeHTTP(delW, delReq)
	if delW.Code != http.StatusNoContent {
		t.Errorf("delete: expected 204, got %d", delW.Code)
	}
}

// =====================================
// rating_handler.go — putRate happy path, getMyRating happy path
// =====================================

func TestRateMedia_OK(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	body := `{"value":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/media-1/rate", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRateMedia_InvalidValue_400(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	body := `{"value":5}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/media-1/rate", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRateMedia_BadJSON_400(t *testing.T) {
	mux, token := newRatingTestMux(t, allowAll(), nil)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/media-1/rate", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetMyRating_OK(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/my-rating", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "value") {
		t.Error("expected value in response")
	}
}

func TestGetMyRating_IDOR_403(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-restricted"},
	}
	mux, token := newRatingTestMux(t, denyAll(), items)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/my-rating", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- star rating extra tests ---

func TestPostStarRating_OK(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", strings.NewReader(`{"value":8}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetStarRating_OK(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDeleteStarRating_OK(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/media-1/rating", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestGetStarRatingAggregate_OK(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-ok"},
	}
	mux, token := newRatingTestMux(t, allowAll(), items)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating/aggregate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "average") {
		t.Error("expected average in response")
	}
}

func TestPostStarRating_IDOR_403(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-x"},
	}
	mux, token := newRatingTestMux(t, denyAll(), items)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", strings.NewReader(`{"value":8}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================================
// syncplay_handler.go — leaveRoom, joinRoom
// =====================================

func TestSyncPlayLeaveRoom_NoAuth_401(t *testing.T) {
	mux, _, _ := newSyncPlayTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/syncplay/rooms/room-1/leave", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayLeaveRoom_OK(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())

	hostClaims := domainuser.Claims{UserID: "host-1", Username: "Host", IsAdmin: false}
	hostToken, _ := tokenGen.GenerateAccessToken(hostClaims)

	// Create room
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(`{"mediaId":"media-1"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+hostToken)
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	var createResp map[string]any
	_ = json.NewDecoder(createW.Body).Decode(&createResp)
	roomID := createResp["id"].(string)

	// Leave room
	leaveReq := httptest.NewRequest(http.MethodDelete, "/api/v1/syncplay/rooms/"+roomID+"/leave", nil)
	leaveReq.Header.Set("Authorization", "Bearer "+hostToken)
	leaveW := httptest.NewRecorder()
	mux.ServeHTTP(leaveW, leaveReq)
	if leaveW.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", leaveW.Code)
	}
}

func TestSyncPlayJoinRoom_NoAuth_401(t *testing.T) {
	mux, _, _ := newSyncPlayTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms/room-1/join", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayJoinRoom_NotFound_404(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, newFakeMediaGetter(), allowAll())

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "U", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms/nonexistent/join", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSyncPlayCreateRoom_BadBody_400(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, newFakeMediaGetter(), allowAll())

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "U", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(`{"mediaId":""}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncPlayCreateRoom_MediaNotFound_404(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, newFakeMediaGetter(), allowAll())

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "U", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(`{"mediaId":"not-found"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSyncPlayCreateRoom_IDOR_403(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "restricted"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, denyAll())

	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "U", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(`{"mediaId":"m1"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================================
// cookie_middleware.go
// =====================================

func TestAuthCookieMiddleware_StripsRefreshToken(t *testing.T) {
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
	if strings.Contains(body, "refreshToken") {
		t.Error("refreshToken should be stripped from body")
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

// =====================================
// access_handler.go — PUT library access, DELETE library access, remote access, playback settings
// =====================================

func TestSetLibraryAccess_Admin_204(t *testing.T) {
	mux, token := newAccessTestMux(t, true)
	body := `{"libraryIds":["lib-1","lib-2"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/u1/library-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestSetLibraryAccess_NonAdmin_403(t *testing.T) {
	mux, token := newAccessTestMux(t, false)
	body := `{"libraryIds":["lib-1"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/u1/library-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSetLibraryAccess_BadBody_400(t *testing.T) {
	mux, token := newAccessTestMux(t, true)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/u1/library-access", strings.NewReader("bad"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDeleteLibraryAccess_Admin_204(t *testing.T) {
	mux, token := newAccessTestMux(t, true)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/u1/library-access", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestDeleteLibraryAccess_NonAdmin_403(t *testing.T) {
	mux, token := newAccessTestMux(t, false)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/u1/library-access", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- remote access routes ---

func newRemoteAccessMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{}
	store := &fakeRemoteAccessStore{data: make(map[string]string)}
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store)
	return mux, adminTok, userTok
}

func TestRemoteAccess_Get_NoAuth_403(t *testing.T) {
	mux, _, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
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
	registerPlaybackSettingsRoutes(mux, cfg, gen)
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
	mux := http.NewServeMux()
	registerParentalRoutes(mux, profileRepo, hasher, gen)
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

// --- push unregister ---

func TestPushUnregister_NoAuth_401(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/push/unregister", strings.NewReader(`{"endpoint":"https://push.example.com/1"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPushListSubscriptions_OK(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/push/subscriptions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.20.30.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPushDeleteSubscription_OK(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/push/subscriptions", strings.NewReader(`{"endpoint":"https://push.example.com/1"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.20.30.2:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPushDeleteSubscription_MissingEndpoint_400(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/push/subscriptions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.20.30.3:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
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
// filter_handler.go — extra coverage
// =====================================

func TestFilterHandler_InvalidToken_401(t *testing.T) {
	mux := newFilterTestServer(&fakeMediaItemLister{}, &fakeTokenVerifier{err: errTest}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFilterHandler_InternalError_500(t *testing.T) {
	lister := &fakeMediaItemLister{err: errTest}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(true)}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFilterHandler_GenreFilter(t *testing.T) {
	lister := &fakeMediaItemLister{items: []*domainlibrary.MediaItem{
		{ID: "m1", Title: "Action Movie"},
	}}
	mux := newFilterTestServer(lister, &fakeTokenVerifier{claims: testClaims(true)}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/filter?genre=action&sort_field=title&sort_order=asc&type=movie&limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
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
// photo_handler.go — search IDOR, albums IDOR, getPhoto/servePhoto missing params
// =====================================

func TestPhotoAlbums_IDOR_403(t *testing.T) {
	mux, token := newPhotoTestMux(t, denyAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/albums?library_id=lib-restricted", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPhotoAlbums_MissingLibraryID_400(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/albums", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPhotoSearch_IDOR_403(t *testing.T) {
	mux, token := newPhotoTestMux(t, denyAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/search?library_id=lib-x&q=test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPhotoSearch_MissingParams_400(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/search", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListPhotos_MissingLibraryID_400(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPhotoScan_NoAuth_403(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader(`{"library_id":"lib-1","dir":"/tmp"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPhotoScan_NonAdmin_403(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader(`{"library_id":"lib-1","dir":"/tmp"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================================
// music_handler.go — search, delete not found
// =====================================

func TestSearchTracks_NoAuth_401(t *testing.T) {
	mux, _ := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/search?library_id=lib-1&q=test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSearchTracks_Auth_OK(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/search?library_id=lib-1&q=test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSearchTracks_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-x", IsAdmin: false})

	mux := http.NewServeMux()
	repo := musicfakes.NewFakeMusicTrackRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, nil)
	registerMusicRoutes(mux, svc, tokenGen, denyAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/search?library_id=lib-x&q=test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDeleteTrack_NotFound_404(t *testing.T) {
	mux, token := newMusicTestMuxWithToken(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/music/tracks/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =====================================
// libraryStatsHandler — extra coverage
// =====================================

func TestLibraryStatsHandler_InvalidToken_403(t *testing.T) {
	h := libraryStatsHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeLibraryLister{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_ServiceError_500(t *testing.T) {
	h := libraryStatsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeLibraryLister{err: errTest},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	req.Header.Set("Authorization", "Bearer tok")
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
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats?token=valid", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// =====================================
// admin_handler.go — user admin routes (bitrate), media routes, refresh-all
// =====================================

func newUserAdminMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	refreshRepo := userfakes.NewFakeRefreshTokenRepository()
	hasher := userfakes.NewFakePasswordHasher()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()

	userSvc := appuser.NewUserService(userRepo, profileRepo, refreshRepo, hasher, nil)
	userQuery := appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)

	mux := http.NewServeMux()
	registerUserAdminRoutes(mux, userSvc, userQuery, gen)
	return mux, adminTok, userTok
}

func TestGetBitrateLimit_NoAuth_403(t *testing.T) {
	mux, _, _ := newUserAdminMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/bitrate-limit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetBitrateLimit_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newUserAdminMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/bitrate-limit", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSetBitrateLimit_NoAuth_403(t *testing.T) {
	mux, _, _ := newUserAdminMux(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", strings.NewReader(`{"max_bitrate_kbps":5000}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSetBitrateLimit_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newUserAdminMux(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", strings.NewReader(`{"max_bitrate_kbps":5000}`))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSetBitrateLimit_BadBody_400(t *testing.T) {
	mux, adminTok, _ := newUserAdminMux(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", strings.NewReader("bad"))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSetBitrateLimit_NegativeValue_400(t *testing.T) {
	mux, adminTok, _ := newUserAdminMux(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", strings.NewReader(`{"max_bitrate_kbps":-1}`))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
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
	handler := securityHeadersMiddleware(inner)
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
	h := subtitleHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaStreamsGetter{},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleHandler_InvalidToken_403(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{err: errTest},
		&fakeMediaStreamsGetter{},
		"http://localhost",
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
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
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleHandler_IDOR_403(t *testing.T) {
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "restricted", FilePath: "/a"}},
		"http://localhost",
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req.Header.Set("Authorization", "Bearer tok")
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
	handler := SecurityHeadersMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff")
	}
}

// =====================================
// photo_handler.go — toPhotoResponse
// =====================================

func TestToPhotoResponse(t *testing.T) {
	now := time.Now()
	p := domainphoto.Photo{
		ID:          "p1",
		LibraryID:   "lib-1",
		Title:       "Sunset",
		FilePath:    "/photos/sunset.jpg",
		FileSize:    1024,
		MimeType:    "image/jpeg",
		Width:       1920,
		Height:      1080,
		TakenAt:     &now,
		CameraModel: "Canon",
		Album:       "Vacation",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	resp := toPhotoResponse(p)
	if resp.ID != "p1" || resp.Title != "Sunset" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// =====================================
// admin_handler.go — admin media routes, search-metadata, refresh-all
// =====================================

func newAdminMediaMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	metaOverrides := metafakes.NewFakeMetadataOverrideRepository()
	metaArtwork := metafakes.NewFakeArtworkRepository()
	metaDownloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("test")
	metaCmd := appmetadata.NewCommandService(metaOverrides, metaArtwork, metaDownloader, bus, []domainmeta.MetadataProvider{provider})
	metaQuery := appmetadata.NewQueryService(metaOverrides, metaArtwork, []domainmeta.MetadataProvider{provider})
	rl := ratelimit.New(100)
	mux := http.NewServeMux()
	registerAdminRoutes(mux, metaCmd, metaQuery, gen, rl, context.Background())
	return mux, adminTok, userTok
}

func TestAdminMedia_NoAuth_401(t *testing.T) {
	mux, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/refresh-metadata", strings.NewReader(`{"tmdb_id":"123","media_type":"movie"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminMedia_NonAdmin_401(t *testing.T) {
	mux, _, userTok := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/refresh-metadata", strings.NewReader(`{"tmdb_id":"123","media_type":"movie"}`))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminMedia_NotFound_404(t *testing.T) {
	mux, adminTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminSearchMetadata_MissingParams_400(t *testing.T) {
	mux, adminTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/m1/search-metadata", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminSearchMetadata_OK(t *testing.T) {
	mux, adminTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/m1/search-metadata?query=test&type=movie", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminRefreshAll_NoAuth_403(t *testing.T) {
	mux, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/refresh-all", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminRefreshAll_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/refresh-all", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminRefreshAll_Admin_202(t *testing.T) {
	mux, adminTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/refresh-all", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestAdminRefreshMetadata_BadBody_400(t *testing.T) {
	mux, adminTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/refresh-metadata", strings.NewReader("bad"))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.RemoteAddr = "10.0.0.2:9999"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminMedia_UnknownAction_404(t *testing.T) {
	mux, adminTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/unknown-action", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
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
// auth_handler.go — push unregister happy path
// =====================================

func TestPushUnregister_OK(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "u1", Username: "user", Enabled: true})
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userRepo, tokenGen, "test-vapid-key")
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})

	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/push/register",
		strings.NewReader(`{"endpoint":"https://push.example.com/1","keys":{"p256dh":"abc","auth":"def"},"consent":true}`))
	regReq.Header.Set("Authorization", "Bearer "+token)
	regReq.RemoteAddr = "10.50.50.1:9999"
	regW := httptest.NewRecorder()
	mux.ServeHTTP(regW, regReq)

	unregReq := httptest.NewRequest(http.MethodDelete, "/api/v1/push/unregister",
		strings.NewReader(`{"endpoint":"https://push.example.com/1"}`))
	unregReq.Header.Set("Authorization", "Bearer "+token)
	unregW := httptest.NewRecorder()
	mux.ServeHTTP(unregW, unregReq)
	if unregW.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", unregW.Code, unregW.Body.String())
	}
}

func TestPushUnregister_InvalidBody_400(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	repo := notifakes.NewFakePushSubscriptionRepository()
	mux := http.NewServeMux()
	registerPushRoutes(mux, repo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeUserRepository(), tokenGen, "test-vapid-key")
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", Username: "user"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/push/unregister", strings.NewReader("bad"))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// =====================================
// auth_handler.go — parental with PIN
// =====================================

func TestParentalUpdateSettings_SetPIN_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
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

// ensure imports used
var (
	_ = apprating.NewService
	_ = ratingfakes.NewFakeRepository
	_ = libfakes.NewFakeCollectionCommandRepo
	_ = playlistfakes.NewFakePlaylistRepository
)
