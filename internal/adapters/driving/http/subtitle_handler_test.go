package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"os"
	"path/filepath"
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
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip"},
		}},
		"http://localhost", allowAll(), allowAllParental{},
		subtitleCache{},
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

// When GetMediaStreams fails we can't validate the requested track or codec, so
// the handler must fail closed with 500 instead of silently spawning ffmpeg on
// the unvalidated index (which would emit a broken/empty stream with no signal).
func TestSubtitleHandler_StreamsError_500(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
	}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, sErr: errTest},
		"http://localhost", allowAll(), allowAllParental{},
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("stream-fetch error must fail closed with 500, got %d", w.Code)
	}
}

// A bitmap subtitle (PGS/DVDSub/...) can't be muxed to text WebVTT - reject
// with 422 instead of spawning an ffmpeg that emits an empty/failed stream.
func TestSubtitleHandler_ImageSubtitle_422(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
	}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle"},
		}},
		"http://localhost", allowAll(), allowAllParental{},
		subtitleCache{},
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

func TestSubtitleHandler_ParentalBlocked_NotFound(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item},
		"http://localhost",
		allowAll(),
		alwaysBlockChecker{},
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from parental gate (existence hidden), got %d", w.Code)
	}
}

func TestSubtitleHandler_ParentalAdmin_NotBlocked(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File:           &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item},
		"http://localhost",
		allowAll(),
		alwaysBlockChecker{},
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// Admin bypasses parental - handler continues past the gate.
	// ffmpeg is not present in unit tests so we get some non-403 status.
	if w.Code == http.StatusForbidden {
		t.Errorf("admin must not be blocked by parental, got 403")
	}
}

// --- isMkvgoSubtitleFastPath ---

// isMkvgoSubtitleFastPath is a pure function; test it directly so the container
// eligibility ("EBML + ISO-BMFF via mkvgo, everything else through ffmpeg")
// property is verifiable without spawning ffmpeg or touching the filesystem.
func TestSubtitleHandler_IsMKVFastPath(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".mkv", true},
		{".webm", true},
		{".mp4", true}, // tx3g/wvtt via the mkvgo mp4 package
		{".m4v", true}, // ISO-BMFF sibling of .mp4
		{".mov", true}, // QuickTime via the mkvgo mp4 package (mkvgo >= v0.12.0)
		{".ts", false}, // MPEG-TS is out of mkvgo's scope
		{".avi", false},
		{".MKV", false}, // caller lower-cases before calling; upper-case must be false
		{".MP4", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isMkvgoSubtitleFastPath(tc.ext); got != tc.want {
			t.Errorf("isMkvgoSubtitleFastPath(%q) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

// TestSubtitleHandler_MKV_TextSub_MkVgoFallback verifies that when the mkvgo
// fast path is triggered for a .mkv file but the file cannot be opened (no
// fixture on disk), the handler falls back to ffmpeg rather than returning a
// 4xx/5xx error from our own logic. Full happy-path validation (mkvgo extracts
// WebVTT from a real embedded subtitle) requires a real MKV fixture, which is
// not present in the test corpus; that validation must be performed against
// actual media files in integration / E2E tests.
func TestSubtitleHandler_MKV_TextSub_MkVgoFallback(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		// Non-existent path: mkvgo.OpenMeta will fail → handler falls back to ffmpeg.
		File: &domainlibrary.MediaFile{Path: "/tmp/nonexistent-subtitle-test.mkv"},
	}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip"},
		}},
		"http://localhost", allowAll(), allowAllParental{},
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// mkvgo.OpenMeta fails (non-existent file) → the ffmpeg fallback is
	// attempted, and it fails too in the unit-test environment.
	//
	// What matters is WHICH answer that produces. Our own guard responses - 404
	// "no such track", 422 "this one is a picture" - must not fire: neither is
	// true here, and a client that believes them stops asking.
	if w.Code == http.StatusNotFound || w.Code == http.StatusUnprocessableEntity {
		t.Errorf("mkvgo fallback must not produce %d: that is a guard answer, and neither guard applies", w.Code)
	}
	// And an extraction that produced nothing must SAY so. This used to answer
	// 200 with an empty body, because the handler wrote its headers before
	// spawning ffmpeg and streamed the process straight into the response: a
	// player then rendered a subtitle track that silently had no lines, with
	// nothing anywhere to say the extraction had failed.
	if w.Code == http.StatusOK && w.Body.Len() == 0 {
		t.Error("a failed extraction answered 200 with an empty body - the player cannot tell that from a subtitle with no dialogue")
	}
}

// TestSubtitleHandler_MP4_TextSub_MkvgoFallback is the ISO-BMFF counterpart of
// the .mkv fallback test: a .mp4 with an embedded text subtitle triggers the
// mkvgo mp4 fast path (mp4.ExtractSubtitleWebVTT, tx3g/wvtt), and when the file
// cannot be opened (no fixture on disk) the handler must fall back to ffmpeg
// rather than emit one of our own guard responses. Happy-path extraction from a
// real tx3g track is covered in integration / E2E tests against actual media.
func TestSubtitleHandler_MP4_TextSub_MkvgoFallback(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		// Non-existent path: mp4.OpenMeta fails → handler falls back to ffmpeg.
		File: &domainlibrary.MediaFile{Path: "/tmp/nonexistent-subtitle-test.mp4"},
	}
	h := subtitleHandler(
		&fakeMediaStreamsGetter{item: item, streams: []*domainlibrary.MediaStream{
			// tx3g scanned via the mkvgo mp4 fast path stores "subrip" (srt→FFprobeCodecName).
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip"},
		}},
		"http://localhost", allowAll(), allowAllParental{},
		subtitleCache{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("track", "0")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound || w.Code == http.StatusUnprocessableEntity {
		t.Errorf("mkvgo mp4 fallback must not produce %d: that is a guard answer, and neither guard applies", w.Code)
	}
	if w.Code == http.StatusOK && w.Body.Len() == 0 {
		t.Error("a failed extraction answered 200 with an empty body - see the .mkv counterpart")
	}
}

// An .ass sidecar converts to WebVTT in-process via mkvgo (no ffmpeg fork): the
// response is WebVTT with cue text extracted from Dialogue lines.
func TestServeExternalSubtitle_ASS_InProcessWebVTT(t *testing.T) {
	dir := t.TempDir()
	ass := "[Script Info]\nTitle: Test\nScriptType: v4.00+\n\n[Events]\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hello\n" +
		"Dialogue: 0,0:00:03.00,0:00:04.00,Default,,0,0,0,,World\n"
	path := filepath.Join(dir, "movie.ass")
	if err := os.WriteFile(path, []byte(ass), 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	serveExternalSubtitle(w, req, path, dir, "")

	if ct := w.Result().Header.Get("Content-Type"); ct != "text/vtt; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/vtt; charset=utf-8", ct)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "WEBVTT") {
		t.Errorf("body must start with WEBVTT, got: %q", body)
	}
	for _, want := range []string{"Hello", "World"} {
		if !strings.Contains(body, want) {
			t.Errorf("WebVTT body missing %q; got:\n%s", want, body)
		}
	}
}

// An .srt sidecar converts to WebVTT in-process via mkvgo (no ffmpeg fork): the
// response is WebVTT (header + '.'-separated timestamps + cue text).
func TestServeExternalSubtitle_SRT_InProcessWebVTT(t *testing.T) {
	dir := t.TempDir()
	srt := "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n2\n00:00:03,000 --> 00:00:04,000\nWorld\n"
	path := filepath.Join(dir, "movie.srt")
	if err := os.WriteFile(path, []byte(srt), 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	serveExternalSubtitle(w, req, path, dir, "")

	if ct := w.Result().Header.Get("Content-Type"); ct != "text/vtt; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/vtt; charset=utf-8", ct)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "WEBVTT") {
		t.Errorf("body must start with WEBVTT, got: %q", body)
	}
	for _, want := range []string{"Hello", "World", "00:00:01.000", "00:00:02.000"} {
		if !strings.Contains(body, want) {
			t.Errorf("WebVTT body missing %q; got:\n%s", want, body)
		}
	}
}

// The subtitle provider is reached on the operator's API key and its download
// quota is capped per day, so search/download must be rate-limited while the
// purely local list route stays unthrottled.
func TestRegisterSubtitleRoutes_ProviderRoutesAreRateLimited(t *testing.T) {
	rl := ratelimit.NewWithBurst(1, 1.0)
	defer rl.Stop()
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, nil, &fakeTokenVerifier{}, &fakeMediaItemGetter{}, nil, rl, nil)

	status := func(method, path string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Code
	}

	// Burst of 1: the first search is admitted (whatever it then answers), the
	// second is rejected by the limiter before any provider call happens.
	_ = status(http.MethodGet, "/api/v1/media/mi-1/subtitles/search")
	if got := status(http.MethodGet, "/api/v1/media/mi-1/subtitles/search"); got != http.StatusTooManyRequests {
		t.Errorf("second search = %d, want %d", got, http.StatusTooManyRequests)
	}
	if got := status(http.MethodPost, "/api/v1/media/mi-1/subtitles/download"); got != http.StatusTooManyRequests {
		t.Errorf("download shares the provider budget, = %d, want %d", got, http.StatusTooManyRequests)
	}
	// The local list route spends no provider quota and must stay open.
	if got := status(http.MethodGet, "/api/v1/media/mi-1/subtitles"); got == http.StatusTooManyRequests {
		t.Error("list is a local read and must not be rate-limited")
	}
}
