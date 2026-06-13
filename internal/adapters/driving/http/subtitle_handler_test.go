package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
)

type fakeSubtitleService struct {
	searchResults []domainsubtitle.SearchResult
	searchErr     error
	downloadSub   *domainsubtitle.SubtitleDownload
	downloadErr   error
	listSubs      []*domainsubtitle.SubtitleDownload
	listErr       error
	deleteErr     error

	lastSearchLang string
	lastDownload   struct {
		fileID   int
		language string
		format   string
	}
	lastDeleteID string
}

func (f *fakeSubtitleService) Search(_ context.Context, _ string, language string) ([]domainsubtitle.SearchResult, error) {
	f.lastSearchLang = language
	return f.searchResults, f.searchErr
}

func (f *fakeSubtitleService) Download(_ context.Context, _ string, fileID int, language, format string) (*domainsubtitle.SubtitleDownload, error) {
	f.lastDownload.fileID = fileID
	f.lastDownload.language = language
	f.lastDownload.format = format
	return f.downloadSub, f.downloadErr
}

func (f *fakeSubtitleService) List(_ context.Context, _ string) ([]*domainsubtitle.SubtitleDownload, error) {
	return f.listSubs, f.listErr
}

func (f *fakeSubtitleService) Delete(_ context.Context, subtitleID string) error {
	f.lastDeleteID = subtitleID
	return f.deleteErr
}

func newSubtitleRoutes(svc subtitleService, tokens TokenVerifier, mg MediaItemGetter, la LibraryAccessChecker) *subtitleRoutes {
	return &subtitleRoutes{subtitleSvc: svc, tokens: tokens, mediaGetter: mg, libraryAccess: la}
}

func subtitleMediaGetter(libraryID string) *fakeMediaGetter {
	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: shared.LibraryID(libraryID)}
	return mg
}

// --- search ---

func TestSubtitleSearch_Success(t *testing.T) {
	svc := &fakeSubtitleService{searchResults: []domainsubtitle.SearchResult{
		{FileID: 1, FileName: "a.srt", Language: "en", Format: "srt", Rating: 9.5},
	}}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=fr", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if svc.lastSearchLang != "fr" {
		t.Errorf("expected lang fr, got %s", svc.lastSearchLang)
	}
	var resp struct {
		Results []struct {
			FileID int `json:"fileId"`
		} `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].FileID != 1 {
		t.Errorf("unexpected results: %+v", resp.Results)
	}
}

func TestSubtitleSearch_NoLanguage_DefaultsEmpty(t *testing.T) {
	svc := &fakeSubtitleService{}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if svc.lastSearchLang != "" {
		t.Errorf("expected empty lang, got %q", svc.lastSearchLang)
	}
}

func TestSubtitleSearch_Unauthenticated(t *testing.T) {
	h := newSubtitleRoutes(&fakeSubtitleService{}, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleSearch_NonAdmin_NoLibraryAccess_Forbidden(t *testing.T) {
	h := newSubtitleRoutes(&fakeSubtitleService{}, validUserValidator(false), subtitleMediaGetter("lib1"), denyAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleSearch_ServiceError_500(t *testing.T) {
	svc := &fakeSubtitleService{searchErr: errors.New("boom")}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// --- download ---

func TestSubtitleDownload_Success(t *testing.T) {
	svc := &fakeSubtitleService{downloadSub: &domainsubtitle.SubtitleDownload{
		ID: "s1", MediaItemID: "m1", Language: "en", Format: "srt", FilePath: "/p", Source: "opensubtitles", DownloadedAt: time.Unix(0, 0),
	}}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)

	body := strings.NewReader(`{"fileId":42,"language":"en","format":"srt"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", body)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if svc.lastDownload.fileID != 42 {
		t.Errorf("expected fileID 42, got %d", svc.lastDownload.fileID)
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "s1" {
		t.Errorf("unexpected id: %v", out["id"])
	}
}

func TestSubtitleDownload_BadJSON_400(t *testing.T) {
	h := newSubtitleRoutes(&fakeSubtitleService{}, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDownload_MissingFileID_400(t *testing.T) {
	h := newSubtitleRoutes(&fakeSubtitleService{}, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`{"language":"en"}`))
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDownload_ServiceError_500(t *testing.T) {
	svc := &fakeSubtitleService{downloadErr: errors.New("boom")}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`{"fileId":1}`))
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// filePath is a server-side filesystem path and must only be exposed to admins.
func TestSubtitleDownload_FilePath_AdminOnly(t *testing.T) {
	newDownloadResp := func(isAdmin bool) map[string]any {
		svc := &fakeSubtitleService{downloadSub: &domainsubtitle.SubtitleDownload{
			ID: "s1", MediaItemID: "m1", Language: "en", Format: "srt", FilePath: "/srv/media/sub.srt", Source: "opensubtitles", DownloadedAt: time.Unix(0, 0),
		}}
		h := newSubtitleRoutes(svc, validUserValidator(isAdmin), subtitleMediaGetter("lib1"), allowAll())
		mux := http.NewServeMux()
		mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`{"fileId":1}`))
		req.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", w.Code)
		}
		var out map[string]any
		if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	if got := newDownloadResp(false); got["filePath"] != nil {
		t.Errorf("non-admin download leaked filePath: %v", got["filePath"])
	}
	if got := newDownloadResp(true); got["filePath"] != "/srv/media/sub.srt" {
		t.Errorf("admin download missing filePath: %v", got["filePath"])
	}
}

func TestSubtitleList_FilePath_AdminOnly(t *testing.T) {
	newListResp := func(isAdmin bool) []map[string]any {
		svc := &fakeSubtitleService{listSubs: []*domainsubtitle.SubtitleDownload{
			{ID: "s1", MediaItemID: "m1", Language: "en", Format: "srt", FilePath: "/srv/media/sub.srt", DownloadedAt: time.Unix(0, 0)},
		}}
		h := newSubtitleRoutes(svc, validUserValidator(isAdmin), subtitleMediaGetter("lib1"), allowAll())
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
		req.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Subtitles []map[string]any `json:"subtitles"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.Subtitles
	}

	nonAdmin := newListResp(false)
	if len(nonAdmin) != 1 || nonAdmin[0]["filePath"] != nil {
		t.Errorf("non-admin list leaked filePath: %v", nonAdmin)
	}
	admin := newListResp(true)
	if len(admin) != 1 || admin[0]["filePath"] != "/srv/media/sub.srt" {
		t.Errorf("admin list missing filePath: %v", admin)
	}
}

// --- list ---

func TestSubtitleList_Success(t *testing.T) {
	svc := &fakeSubtitleService{listSubs: []*domainsubtitle.SubtitleDownload{
		{ID: "s1", MediaItemID: "m1", Language: "en", Format: "srt", DownloadedAt: time.Unix(0, 0)},
		{ID: "s2", MediaItemID: "m1", Language: "fr", Format: "srt", DownloadedAt: time.Unix(0, 0)},
	}}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Subtitles []struct {
			ID string `json:"id"`
		} `json:"subtitles"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Subtitles) != 2 {
		t.Errorf("expected 2 subs, got %d", len(resp.Subtitles))
	}
}

func TestSubtitleList_ServiceError_500(t *testing.T) {
	svc := &fakeSubtitleService{listErr: errors.New("boom")}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// --- delete ---

func TestSubtitleDelete_Success(t *testing.T) {
	svc := &fakeSubtitleService{listSubs: []*domainsubtitle.SubtitleDownload{{ID: "s1", MediaItemID: "m1"}}}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/s1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if svc.lastDeleteID != "s1" {
		t.Errorf("expected delete id s1, got %q", svc.lastDeleteID)
	}
}

func TestSubtitleDelete_NotInList_404(t *testing.T) {
	svc := &fakeSubtitleService{listSubs: []*domainsubtitle.SubtitleDownload{{ID: "other"}}}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/s1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if svc.lastDeleteID != "" {
		t.Errorf("Delete should not have been called")
	}
}

func TestSubtitleDelete_ServiceError_404(t *testing.T) {
	svc := &fakeSubtitleService{
		listSubs:  []*domainsubtitle.SubtitleDownload{{ID: "s1"}},
		deleteErr: errors.New("boom"),
	}
	h := newSubtitleRoutes(svc, validUserValidator(true), subtitleMediaGetter("lib1"), allowAll())
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/s1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- subtitleHandler embedded-track validation ---

// A ?track= past the real subtitle-stream count must 404, not become
// `-map 0:s:99` and fail the ffmpeg spawn (forged deep index).
func TestSubtitleHandler_TrackOutOfRange_404(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
	}
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip"},
		}},
		"http://localhost", allowAll(), allowAllParental{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/5", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "5")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("out-of-range subtitle track must 404, got %d", w.Code)
	}
}

// A bitmap subtitle (PGS/DVDSub/...) can't be muxed to text WebVTT — reject
// with 422 instead of spawning an ffmpeg that emits an empty/failed stream.
func TestSubtitleHandler_ImageSubtitle_422(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
	}
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle"},
		}},
		"http://localhost", allowAll(), allowAllParental{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("image subtitle must 422, got %d", w.Code)
	}
}

// --- subtitleHandler parental gate ---

func TestSubtitleHandler_ParentalBlocked_403(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: item},
		"http://localhost",
		allowAll(),
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 from parental gate, got %d", w.Code)
	}
}

func TestSubtitleHandler_ParentalAdmin_NotBlocked(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := subtitleHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeMediaStreamsGetter{item: item},
		"http://localhost",
		allowAll(),
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// Admin bypasses parental — handler continues past the gate.
	// ffmpeg is not present in unit tests so we get some non-403 status.
	if w.Code == http.StatusForbidden {
		t.Errorf("admin must not be blocked by parental, got 403")
	}
}
