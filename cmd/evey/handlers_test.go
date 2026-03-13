package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/config"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	domainplayback "github.com/gravity-zero/evey/internal/domain/playback"
	domainsubtitle "github.com/gravity-zero/evey/internal/domain/subtitle"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

// --- Fake token validator ---

type fakeTokenValidator struct {
	claims *domainuser.Claims
	err    error
}

func (f *fakeTokenValidator) ValidateAccessToken(_ string) (*domainuser.Claims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func validTokenValidator(isAdmin bool) *fakeTokenValidator {
	return &fakeTokenValidator{
		claims: &domainuser.Claims{
			UserID:    "user-1",
			Username:  "test",
			IsAdmin:   isAdmin,
			ProfileID: "prof-1",
		},
	}
}

func invalidTokenValidator() *fakeTokenValidator {
	return &fakeTokenValidator{err: errors.New("invalid token")}
}

// --- Fake library service ---

type fakeLibService struct {
	items    map[string]*domainlibrary.MediaItem
	streams  map[string][]*domainlibrary.MediaStream
	chapters map[string][]*domainlibrary.Chapter
}

func newFakeLibService() *fakeLibService {
	return &fakeLibService{
		items:    make(map[string]*domainlibrary.MediaItem),
		streams:  make(map[string][]*domainlibrary.MediaStream),
		chapters: make(map[string][]*domainlibrary.Chapter),
	}
}

func (f *fakeLibService) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeLibService) GetMediaStreams(_ context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error) {
	if s, ok := f.streams[mediaItemID]; ok {
		return s, nil
	}
	return nil, nil
}

func (f *fakeLibService) GetChapters(_ context.Context, mediaItemID string) ([]*domainlibrary.Chapter, error) {
	if c, ok := f.chapters[mediaItemID]; ok {
		return c, nil
	}
	return nil, nil
}

func (f *fakeLibService) ListLibraries(_ context.Context) ([]*domainlibrary.Library, error) {
	return nil, nil
}

// --- Fake artwork repo ---

type fakeArtworkRepo struct {
	records map[string][]*domainmeta.ArtworkRecord
}

func newFakeArtworkRepo() *fakeArtworkRepo {
	return &fakeArtworkRepo{records: make(map[string][]*domainmeta.ArtworkRecord)}
}

func (f *fakeArtworkRepo) GetByMediaItem(_ context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error) {
	if r, ok := f.records[mediaItemID]; ok {
		return r, nil
	}
	return nil, nil
}

func (f *fakeArtworkRepo) UpdateSortOrder(_ context.Context, _ string, _ int) error {
	return nil
}

// --- Fake playback service ---

type fakePlaybackService struct {
	sessions []*domainplayback.ActiveSessionInfo
	stopErr  error
}

func (f *fakePlaybackService) GetActiveSessions(_ context.Context) ([]*domainplayback.ActiveSessionInfo, error) {
	return f.sessions, nil
}

func (f *fakePlaybackService) StopStream(_ context.Context, _ string) error {
	return f.stopErr
}

// --- Fake subtitle deps ---

type fakeSubtitleRepo struct {
	subs map[string][]*domainsubtitle.SubtitleDownload
}

func newFakeSubtitleRepo() *fakeSubtitleRepo {
	return &fakeSubtitleRepo{subs: make(map[string][]*domainsubtitle.SubtitleDownload)}
}

func (f *fakeSubtitleRepo) Save(_ context.Context, sub *domainsubtitle.SubtitleDownload) error {
	f.subs[sub.MediaItemID] = append(f.subs[sub.MediaItemID], sub)
	return nil
}

func (f *fakeSubtitleRepo) GetByMediaItem(_ context.Context, mediaItemID string) ([]*domainsubtitle.SubtitleDownload, error) {
	return f.subs[mediaItemID], nil
}

func (f *fakeSubtitleRepo) Delete(_ context.Context, id string) error {
	for mediaID, subs := range f.subs {
		for i, s := range subs {
			if s.ID == id {
				f.subs[mediaID] = append(subs[:i], subs[i+1:]...)
				return nil
			}
		}
	}
	return domainsubtitle.ErrNotFound
}

type fakeSubtitleProvider struct {
	results []domainsubtitle.SearchResult
}

func (f *fakeSubtitleProvider) Search(_ context.Context, _, _, _ string) ([]domainsubtitle.SearchResult, error) {
	return f.results, nil
}

func (f *fakeSubtitleProvider) Download(_ context.Context, _ int, destPath string) (string, error) {
	return destPath + "/sub.srt", nil
}

type fakeMediaItemGetter struct {
	items map[string]*appsubtitle.MediaItemInfo
}

func (f *fakeMediaItemGetter) GetMediaItem(_ context.Context, id string) (*appsubtitle.MediaItemInfo, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}

// --- Fake trickplay service ---

type fakeTrickplayService struct {
	manifests map[string]*domainlibrary.TrickplayManifest
}

func newFakeTrickplayService() *fakeTrickplayService {
	return &fakeTrickplayService{manifests: make(map[string]*domainlibrary.TrickplayManifest)}
}

func (f *fakeTrickplayService) GetTrickplayManifest(_ context.Context, mediaItemID string) (*domainlibrary.TrickplayManifest, error) {
	if m, ok := f.manifests[mediaItemID]; ok {
		return m, nil
	}
	return nil, errors.New("not found")
}

// --- Fake generate trickplay service ---

type fakeGenerateTrickplayService struct {
	err error
}

func (f *fakeGenerateTrickplayService) GenerateTrickplay(_ context.Context, _ string) error {
	return f.err
}

// --- Fake library access checker ---

type fakeLibraryAccessChecker struct {
	allowed bool
	err     error
}

func (f *fakeLibraryAccessChecker) HasAccess(_ context.Context, _, _ string) (bool, error) {
	return f.allowed, f.err
}

func allowAllLibraries() *fakeLibraryAccessChecker  { return &fakeLibraryAccessChecker{allowed: true} }
func denyAllLibraries() *fakeLibraryAccessChecker   { return &fakeLibraryAccessChecker{allowed: false} }

// --- Helper ---

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// --- Tests ---

func TestHealthzHandler(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"test"}`))
	})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
	if resp["version"] != "test" {
		t.Errorf("version = %q, want test", resp["version"])
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
}

func TestStreamHandler_Unauthenticated(t *testing.T) {
	handler := mediaHandler(invalidTokenValidator(), newFakeLibService(), newFakeArtworkRepo(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamHandler_InvalidToken(t *testing.T) {
	handler := mediaHandler(invalidTokenValidator(), newFakeLibService(), newFakeArtworkRepo(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamHandler_ValidToken_NotFound(t *testing.T) {
	handler := mediaHandler(validTokenValidator(false), newFakeLibService(), newFakeArtworkRepo(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamHandler_ValidToken_ServesFile(t *testing.T) {
	libSvc := newFakeLibService()
	tmpFile := t.TempDir() + "/test.mp4"
	if err := writeTestFile(tmpFile, "fake video content"); err != nil {
		t.Fatal(err)
	}
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: tmpFile}
	handler := mediaHandler(validTokenValidator(false), libSvc, newFakeArtworkRepo(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != "fake video content" {
		t.Errorf("body = %q, want fake video content", body)
	}
}

func TestStreamHandler_TokenInQueryParam(t *testing.T) {
	libSvc := newFakeLibService()
	tmpFile := t.TempDir() + "/test.mp4"
	writeTestFile(tmpFile, "ok")
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: tmpFile}
	handler := mediaHandler(validTokenValidator(false), libSvc, newFakeArtworkRepo(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream?token=valid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestArtworkHandler_NotFound(t *testing.T) {
	handler := mediaHandler(validTokenValidator(false), newFakeLibService(), newFakeArtworkRepo(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/artwork/poster", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestArtworkHandler_Redirect(t *testing.T) {
	artRepo := newFakeArtworkRepo()
	url := "https://image.tmdb.org/poster.jpg"
	artRepo.records["m1"] = []*domainmeta.ArtworkRecord{
		{ID: "art1", MediaItemID: "m1", Type: "poster", SourceURL: &url, SortOrder: 0},
	}
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/movies/movie.mkv"}
	handler := mediaHandler(validTokenValidator(false), libSvc, artRepo, "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/artwork/poster", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != url {
		t.Errorf("Location = %q, want %q", loc, url)
	}
}

func TestArtworkHandler_NoSourceURL(t *testing.T) {
	artRepo := newFakeArtworkRepo()
	artRepo.records["m1"] = []*domainmeta.ArtworkRecord{
		{ID: "art1", MediaItemID: "m1", Type: "poster"},
	}
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/movies/movie.mkv"}
	handler := mediaHandler(validTokenValidator(false), libSvc, artRepo, "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/artwork/poster", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestArtworkHandler_Unauthenticated(t *testing.T) {
	handler := mediaHandler(validTokenValidator(false), newFakeLibService(), newFakeArtworkRepo(), "http://localhost", allowAllLibraries())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/artwork/poster", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestArtworkHandler_LibraryAccessDenied(t *testing.T) {
	artRepo := newFakeArtworkRepo()
	url := "https://image.tmdb.org/poster.jpg"
	artRepo.records["m1"] = []*domainmeta.ArtworkRecord{
		{ID: "art1", MediaItemID: "m1", Type: "poster", SourceURL: &url, SortOrder: 0},
	}
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/movies/movie.mkv", LibraryID: "lib-restricted"}
	handler := mediaHandler(validTokenValidator(false), libSvc, artRepo, "http://localhost", denyAllLibraries())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/artwork/poster", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDownloadHandler_Unauthenticated(t *testing.T) {
	handler := downloadHandler(invalidTokenValidator(), newFakeLibService(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/download", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/download", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDownloadHandler_InvalidToken(t *testing.T) {
	handler := downloadHandler(invalidTokenValidator(), newFakeLibService(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/download", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/download", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDownloadHandler_NotFound(t *testing.T) {
	handler := downloadHandler(validTokenValidator(false), newFakeLibService(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/download", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/download", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDownloadHandler_ServesFile(t *testing.T) {
	libSvc := newFakeLibService()
	tmpFile := t.TempDir() + "/movie.mkv"
	writeTestFile(tmpFile, "file data")
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: tmpFile}

	handler := downloadHandler(validTokenValidator(false), libSvc, "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/download", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/download", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("content-type = %q, want application/octet-stream", ct)
	}
	if disp := w.Header().Get("Content-Disposition"); !strings.Contains(disp, "movie.mkv") {
		t.Errorf("content-disposition = %q, should contain movie.mkv", disp)
	}
}

func TestTranscodeHandler_MissingToken(t *testing.T) {
	handler := transcodeHandler(invalidTokenValidator(), newFakeLibService(), "http://localhost", allowAllLibraries(), apptranscoding.NewTranscodeThrottle(), nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/transcode", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTranscodeHandler_InvalidToken(t *testing.T) {
	handler := transcodeHandler(invalidTokenValidator(), newFakeLibService(), "http://localhost", allowAllLibraries(), apptranscoding.NewTranscodeThrottle(), nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/transcode", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestTranscodeHandler_NotFound(t *testing.T) {
	handler := transcodeHandler(validTokenValidator(false), newFakeLibService(), "http://localhost", allowAllLibraries(), apptranscoding.NewTranscodeThrottle(), nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/transcode", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestChaptersHandler_Unauthenticated(t *testing.T) {
	handler := chaptersHandler(invalidTokenValidator(), newFakeLibService(), "", allowAllLibraries(), newFakeLibService())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/chapters", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestChaptersHandler_ReturnsJSON(t *testing.T) {
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-1"}
	libSvc.chapters["m1"] = []*domainlibrary.Chapter{
		{ID: "ch1", MediaItemID: "m1", Title: "Intro", StartTimeMs: 0, EndTimeMs: 30000},
		{ID: "ch2", MediaItemID: "m1", Title: "Act 1", StartTimeMs: 30000, EndTimeMs: 120000},
	}
	handler := chaptersHandler(validTokenValidator(false), libSvc, "", allowAllLibraries(), libSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/chapters", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var resp struct {
		Chapters []struct {
			Title       string `json:"title"`
			StartTimeMs int64  `json:"startTimeMs"`
			EndTimeMs   int64  `json:"endTimeMs"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("chapters count = %d, want 2", len(resp.Chapters))
	}
	if resp.Chapters[0].Title != "Intro" {
		t.Errorf("chapter[0].title = %q, want Intro", resp.Chapters[0].Title)
	}
	if resp.Chapters[1].StartTimeMs != 30000 {
		t.Errorf("chapter[1].startTimeMs = %d, want 30000", resp.Chapters[1].StartTimeMs)
	}
}

func TestChaptersHandler_Empty(t *testing.T) {
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-1"}
	handler := chaptersHandler(validTokenValidator(false), libSvc, "", allowAllLibraries(), libSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/chapters", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Chapters []any `json:"chapters"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Chapters) != 0 {
		t.Errorf("chapters count = %d, want 0", len(resp.Chapters))
	}
}

func TestSelectArtworkHandler_Unauthenticated(t *testing.T) {
	handler := selectArtworkHandler(invalidTokenValidator(), newFakeArtworkRepo())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", handler)

	req := httptest.NewRequest("POST", "/api/v1/media/m1/artwork/art1/select", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSelectArtworkHandler_NonAdmin(t *testing.T) {
	handler := selectArtworkHandler(validTokenValidator(false), newFakeArtworkRepo())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", handler)

	req := httptest.NewRequest("POST", "/api/v1/media/m1/artwork/art1/select", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSelectArtworkHandler_NotFound(t *testing.T) {
	handler := selectArtworkHandler(validTokenValidator(true), newFakeArtworkRepo())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", handler)

	req := httptest.NewRequest("POST", "/api/v1/media/m1/artwork/art1/select", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSelectArtworkHandler_Success(t *testing.T) {
	artRepo := newFakeArtworkRepo()
	url := "https://img.example.com/poster.jpg"
	artRepo.records["m1"] = []*domainmeta.ArtworkRecord{
		{ID: "art1", MediaItemID: "m1", Type: "poster", SourceURL: &url, SortOrder: 0},
		{ID: "art2", MediaItemID: "m1", Type: "poster", SourceURL: &url, SortOrder: 1},
	}
	handler := selectArtworkHandler(validTokenValidator(true), artRepo)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/artwork/{artworkId}/select", handler)

	req := httptest.NewRequest("POST", "/api/v1/media/m1/artwork/art2/select", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]bool
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp["ok"] {
		t.Error("expected ok = true")
	}
}

func TestBrowseDirsHandler_Unauthenticated(t *testing.T) {
	handler := browseDirsHandler(invalidTokenValidator())

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/fs/browse", handler)

	req := httptest.NewRequest("GET", "/api/v1/admin/fs/browse", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestBrowseDirsHandler_NonAdmin(t *testing.T) {
	handler := browseDirsHandler(validTokenValidator(false))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/fs/browse", handler)

	req := httptest.NewRequest("GET", "/api/v1/admin/fs/browse", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestBrowseDirsHandler_InvalidPath(t *testing.T) {
	handler := browseDirsHandler(validTokenValidator(true))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/fs/browse", handler)

	req := httptest.NewRequest("GET", "/api/v1/admin/fs/browse?path=/etc/passwd", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestActiveSessionsHandler_NonAdmin(t *testing.T) {
	handler := activeSessionsHandler(validTokenValidator(false), &fakePlaybackService{})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/sessions/active", handler)

	req := httptest.NewRequest("GET", "/api/v1/admin/sessions/active", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestActiveSessionsHandler_Admin(t *testing.T) {
	now := time.Now()
	pbSvc := &fakePlaybackService{
		sessions: []*domainplayback.ActiveSessionInfo{
			{
				SessionID:   "sess-1",
				ProfileName: "Test",
				MediaTitle:  "Movie A",
				MediaType:   "movie",
				MediaItemID: "m1",
				DurationMs:  7200000,
				PositionMs:  3600000,
				StartedAt:   now,
				PlayMethod:  "direct_play",
			},
		},
	}
	handler := activeSessionsHandler(validTokenValidator(true), pbSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/sessions/active", handler)

	req := httptest.NewRequest("GET", "/api/v1/admin/sessions/active", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Sessions []struct {
			SessionID  string  `json:"sessionId"`
			MediaTitle string  `json:"mediaTitle"`
			Progress   float64 `json:"progress"`
		} `json:"sessions"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(resp.Sessions))
	}
	if resp.Sessions[0].SessionID != "sess-1" {
		t.Errorf("sessionId = %q, want sess-1", resp.Sessions[0].SessionID)
	}
	if resp.Sessions[0].Progress != 50 {
		t.Errorf("progress = %f, want 50", resp.Sessions[0].Progress)
	}
}

func TestKillSessionHandler_NonAdmin(t *testing.T) {
	handler := killSessionHandler(validTokenValidator(false), &fakePlaybackService{})

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/sessions/sess-1", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestKillSessionHandler_Success(t *testing.T) {
	handler := killSessionHandler(validTokenValidator(true), &fakePlaybackService{})

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/sessions/sess-1", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKillSessionHandler_NotFound(t *testing.T) {
	handler := killSessionHandler(validTokenValidator(true), &fakePlaybackService{stopErr: errors.New("not found")})

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/sessions/sess-1", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_Forbidden(t *testing.T) {
	handler := libraryStatsHandler(invalidTokenValidator(), newFakeLibService())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/library-stats", handler)

	req := httptest.NewRequest("GET", "/api/v1/library-stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_Success(t *testing.T) {
	handler := libraryStatsHandler(validTokenValidator(false), newFakeLibService())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/library-stats", handler)

	req := httptest.NewRequest("GET", "/api/v1/library-stats", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Stats []any `json:"stats"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Stats == nil {
		t.Error("stats should not be nil")
	}
}

func TestSubtitleHandler_Unauthenticated(t *testing.T) {
	handler := subtitleHandler(invalidTokenValidator(), newFakeLibService(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/subtitle/0", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleHandler_NotFound(t *testing.T) {
	handler := subtitleHandler(validTokenValidator(false), newFakeLibService(), "http://localhost", allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/subtitle/0", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleRoutes_List(t *testing.T) {
	subRepo := newFakeSubtitleRepo()
	now := time.Now()
	subRepo.subs["m1"] = []*domainsubtitle.SubtitleDownload{
		{ID: "sub-1", MediaItemID: "m1", Language: "en", Format: "srt", FilePath: "/subs/en.srt", Source: "opensubtitles", DownloadedAt: now},
	}
	provider := &fakeSubtitleProvider{}
	mediaGetter := &fakeMediaItemGetter{
		items: map[string]*appsubtitle.MediaItemInfo{
			"m1": {ID: "m1", FilePath: "/movies/movie.mkv", Title: "Movie"},
		},
	}
	subtitleSvc := appsubtitle.NewService(subRepo, provider, mediaGetter)

	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)
	token := createTestToken(t, tokenGen, "prof-1", false)

	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/movies/movie.mkv"}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, subtitleSvc, tokenGen, libSvc, allowAllLibraries())

	req := httptest.NewRequest("GET", "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Subtitles []struct {
			ID       string `json:"id"`
			Language string `json:"language"`
			Format   string `json:"format"`
		} `json:"subtitles"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Subtitles) != 1 {
		t.Fatalf("subtitles count = %d, want 1", len(resp.Subtitles))
	}
	if resp.Subtitles[0].Language != "en" {
		t.Errorf("language = %q, want en", resp.Subtitles[0].Language)
	}
}

func TestSubtitleRoutes_Unauthenticated(t *testing.T) {
	subRepo := newFakeSubtitleRepo()
	provider := &fakeSubtitleProvider{}
	mediaGetter := &fakeMediaItemGetter{items: map[string]*appsubtitle.MediaItemInfo{}}
	subtitleSvc := appsubtitle.NewService(subRepo, provider, mediaGetter)

	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)

	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, subtitleSvc, tokenGen, newFakeLibService(), allowAllLibraries())

	req := httptest.NewRequest("GET", "/api/v1/media/m1/subtitles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleRoutes_Delete(t *testing.T) {
	subRepo := newFakeSubtitleRepo()
	now := time.Now()
	subRepo.subs["m1"] = []*domainsubtitle.SubtitleDownload{
		{ID: "sub-1", MediaItemID: "m1", Language: "en", Format: "srt", FilePath: "/subs/en.srt", Source: "opensubtitles", DownloadedAt: now},
	}
	provider := &fakeSubtitleProvider{}
	mediaGetter := &fakeMediaItemGetter{items: map[string]*appsubtitle.MediaItemInfo{}}
	subtitleSvc := appsubtitle.NewService(subRepo, provider, mediaGetter)

	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)
	token := createTestToken(t, tokenGen, "prof-1", false)

	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/movies/movie.mkv"}
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, subtitleSvc, tokenGen, libSvc, allowAllLibraries())

	req := httptest.NewRequest("DELETE", "/api/v1/media/m1/subtitles/sub-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPlaybackSettingsHandler_Get(t *testing.T) {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  10,
			SkipBackwardSeconds: 5,
		},
	}
	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)
	token := createTestToken(t, tokenGen, "prof-1", false)

	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, tokenGen)

	req := httptest.NewRequest("GET", "/api/v1/playback-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		SkipForwardSeconds  int `json:"skipForwardSeconds"`
		SkipBackwardSeconds int `json:"skipBackwardSeconds"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SkipForwardSeconds != 10 {
		t.Errorf("skipForwardSeconds = %d, want 10", resp.SkipForwardSeconds)
	}
	if resp.SkipBackwardSeconds != 5 {
		t.Errorf("skipBackwardSeconds = %d, want 5", resp.SkipBackwardSeconds)
	}
}

func TestPlaybackSettingsHandler_Get_Unauthenticated(t *testing.T) {
	cfg := &config.Config{}
	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)

	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, tokenGen)

	req := httptest.NewRequest("GET", "/api/v1/playback-settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackSettingsHandler_Put_Admin(t *testing.T) {
	cfg := &config.Config{
		Playback: config.PlaybackConfig{
			SkipForwardSeconds:  10,
			SkipBackwardSeconds: 10,
		},
	}
	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)
	token := createTestToken(t, tokenGen, "prof-1", true)

	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, tokenGen)

	body := strings.NewReader(`{"skipForwardSeconds":30,"skipBackwardSeconds":15}`)
	req := httptest.NewRequest("PUT", "/api/v1/admin/playback-settings", body)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if cfg.Playback.SkipForwardSeconds != 30 {
		t.Errorf("SkipForwardSeconds = %d, want 30", cfg.Playback.SkipForwardSeconds)
	}
	if cfg.Playback.SkipBackwardSeconds != 15 {
		t.Errorf("SkipBackwardSeconds = %d, want 15", cfg.Playback.SkipBackwardSeconds)
	}
}

func TestPlaybackSettingsHandler_Put_NonAdmin(t *testing.T) {
	cfg := &config.Config{}
	key, _ := generateTestKey()
	tokenGen := auth.NewJWTGenerator(key, time.Hour)
	token := createTestToken(t, tokenGen, "prof-1", false)

	mux := http.NewServeMux()
	registerPlaybackSettingsRoutes(mux, cfg, tokenGen)

	body := strings.NewReader(`{"skipForwardSeconds":30}`)
	req := httptest.NewRequest("PUT", "/api/v1/admin/playback-settings", body)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_Forbidden(t *testing.T) {
	handler := trickplayManifestHandler(invalidTokenValidator(), newFakeTrickplayService(), "", allowAllLibraries(), newFakeLibService())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/manifest", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/trickplay/manifest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_NotFound(t *testing.T) {
	handler := trickplayManifestHandler(validTokenValidator(false), newFakeTrickplayService(), "", allowAllLibraries(), newFakeLibService())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/manifest", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/trickplay/manifest", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_ReturnsVTT(t *testing.T) {
	svc := newFakeTrickplayService()
	svc.manifests["m1"] = &domainlibrary.TrickplayManifest{
		ID:          "tp1",
		MediaItemID: "m1",
		Interval:    10,
		Width:       320,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  1,
		ImagePath:   "/cache/trickplay/m1",
	}
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-1"}
	handler := trickplayManifestHandler(validTokenValidator(false), svc, "", allowAllLibraries(), libSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/manifest", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/trickplay/manifest", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/vtt" {
		t.Errorf("content-type = %q, want text/vtt", ct)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "WEBVTT") {
		t.Error("body should start with WEBVTT")
	}
	if !strings.Contains(body, "trickplay_001.jpg") {
		t.Error("body should reference trickplay image file")
	}
}

func TestTrickplayGenerateHandler_NonAdmin(t *testing.T) {
	handler := trickplayGenerateHandler(validTokenValidator(false), &fakeGenerateTrickplayService{})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/trickplay/generate", handler)

	req := httptest.NewRequest("POST", "/api/v1/media/m1/trickplay/generate", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestTrickplayGenerateHandler_Admin(t *testing.T) {
	handler := trickplayGenerateHandler(validTokenValidator(true), &fakeGenerateTrickplayService{})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/trickplay/generate", handler)

	req := httptest.NewRequest("POST", "/api/v1/media/m1/trickplay/generate", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "trickplay generation started" {
		t.Errorf("message = %q, want trickplay generation started", resp["message"])
	}
}

func TestFormatVTTTime(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "00:00:00.000"},
		{61, "00:01:01.000"},
		{3661, "01:01:01.000"},
		{7200, "02:00:00.000"},
	}
	for _, tt := range tests {
		got := formatVTTTime(tt.input)
		if got != tt.want {
			t.Errorf("formatVTTTime(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal.mp4", "normal.mp4"},
		{"with spaces.mkv", "with spaces.mkv"},
		{"path/slash.mp4", "path_slash.mp4"},
		{"", "download"},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPlaybackInfoHandler_Unauthenticated(t *testing.T) {
	handler := playbackInfoHandler(invalidTokenValidator(), newFakeLibService(), allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/playback-info", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfoHandler_NotFound(t *testing.T) {
	handler := playbackInfoHandler(validTokenValidator(false), newFakeLibService(), allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/playback-info", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfoHandler_ReturnsDecision(t *testing.T) {
	libSvc := newFakeLibService()
	w1, h1 := 1920, 1080
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/movies/movie.mkv", ContainerFormat: "mkv"}
	libSvc.streams["m1"] = []*domainlibrary.MediaStream{
		{ID: "v1", MediaItemID: "m1", StreamType: "video", Codec: "h264", Width: &w1, Height: &h1, IsDefault: true},
		{ID: "a1", MediaItemID: "m1", StreamType: "audio", Codec: "aac", IsDefault: true},
	}

	handler := playbackInfoHandler(validTokenValidator(false), libSvc, allowAllLibraries())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/playback-info", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/playback-info", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if _, ok := resp["play_method"]; !ok {
		t.Error("response should contain play_method")
	}
}

func TestIsWithinDir(t *testing.T) {
	cases := []struct {
		dir  string
		path string
		want bool
	}{
		{"/cache/trickplay/m1", "/cache/trickplay/m1/trickplay_001.jpg", true},
		{"/cache/trickplay/m1", "/cache/trickplay/m1/sub/deep.jpg", true},
		{"/cache/trickplay/m1", "/cache/trickplay/m2/evil.jpg", false},
		{"/cache/trickplay/m1", "/cache/trickplay/m1", false},
		{"/cache/hls/session1", "/cache/hls/session1/segment.ts", true},
		{"/cache/hls/session1", "/cache/hls/session1/../other/evil.ts", false},
		{"/cache/hls/session1", "/etc/passwd", false},
	}
	for _, tc := range cases {
		got := isWithinDir(tc.dir, tc.path)
		if got != tc.want {
			t.Errorf("isWithinDir(%q, %q) = %v, want %v", tc.dir, tc.path, got, tc.want)
		}
	}
}

func TestChaptersHandler_CORS_UsesOriginNotStar(t *testing.T) {
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-1"}
	handler := chaptersHandler(validTokenValidator(false), libSvc, "https://evey.test", allowAllLibraries(), libSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/chapters", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Origin", "https://evey.test")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader == "*" {
		t.Error("CORS header must not be wildcard *")
	}
	if corsHeader != "https://evey.test" {
		t.Errorf("CORS header = %q, want https://evey.test", corsHeader)
	}
}

func TestTrickplayImageHandler_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	svc := newFakeTrickplayService()
	svc.manifests["m1"] = &domainlibrary.TrickplayManifest{
		ID:          "tp1",
		MediaItemID: "m1",
		ImagePath:   dir,
	}
	handler := trickplayImageHandler(validTokenValidator(false), svc, "", nil, newFakeLibService())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/trickplay/..%2Fevil.jpg", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("path traversal attempt should not return 200")
	}
}

func TestRequireAdmin_NoToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAdmin(invalidTokenValidator(), inner)

	req := httptest.NewRequest("GET", "/admin/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAdmin_InvalidToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAdmin(invalidTokenValidator(), inner)

	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAdmin_NonAdmin(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAdmin(validTokenValidator(false), inner)

	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin_AdminAllowed(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAdmin(validTokenValidator(true), inner)

	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_TokenViaQueryParam(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAdmin(validTokenValidator(true), inner)

	req := httptest.NewRequest("GET", "/admin/test?token=valid-token", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Library access checks ---

func TestStreamHandler_LibraryAccessDenied(t *testing.T) {
	libSvc := newFakeLibService()
	tmpFile := t.TempDir() + "/test.mp4"
	writeTestFile(tmpFile, "fake video content")
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: tmpFile, LibraryID: "lib-restricted"}

	handler := mediaHandler(validTokenValidator(false), libSvc, newFakeArtworkRepo(), "http://localhost", denyAllLibraries())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamHandler_AdminBypassesLibraryAccess(t *testing.T) {
	libSvc := newFakeLibService()
	tmpFile := t.TempDir() + "/test.mp4"
	writeTestFile(tmpFile, "fake video content")
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: tmpFile, LibraryID: "lib-restricted"}

	handler := mediaHandler(validTokenValidator(true), libSvc, newFakeArtworkRepo(), "http://localhost", denyAllLibraries())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("admin should bypass library access, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDownloadHandler_LibraryAccessDenied(t *testing.T) {
	libSvc := newFakeLibService()
	tmpFile := t.TempDir() + "/movie.mkv"
	writeTestFile(tmpFile, "file data")
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: tmpFile, LibraryID: "lib-restricted"}

	handler := downloadHandler(validTokenValidator(false), libSvc, "http://localhost", denyAllLibraries())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/download", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/download", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTranscodeHandler_LibraryAccessDenied(t *testing.T) {
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/some/file.mkv", LibraryID: "lib-restricted"}

	handler := transcodeHandler(validTokenValidator(false), libSvc, "http://localhost", denyAllLibraries(), apptranscoding.NewTranscodeThrottle(), nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/transcode", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/transcode", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubtitleHandler_LibraryAccessDenied(t *testing.T) {
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", FilePath: "/some/file.mkv", LibraryID: "lib-restricted"}

	handler := subtitleHandler(validTokenValidator(false), libSvc, "http://localhost", denyAllLibraries())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/subtitle/0", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChaptersHandler_LibraryAccessDenied(t *testing.T) {
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}

	handler := chaptersHandler(validTokenValidator(false), libSvc, "", denyAllLibraries(), libSvc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/chapters", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/chapters", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrickplayManifestHandler_LibraryAccessDenied(t *testing.T) {
	svc := newFakeTrickplayService()
	svc.manifests["m1"] = &domainlibrary.TrickplayManifest{ID: "tp1", MediaItemID: "m1"}
	libSvc := newFakeLibService()
	libSvc.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}

	handler := trickplayManifestHandler(validTokenValidator(false), svc, "", denyAllLibraries(), libSvc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", handler)

	req := httptest.NewRequest("GET", "/api/v1/media/m1/trickplay", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}
