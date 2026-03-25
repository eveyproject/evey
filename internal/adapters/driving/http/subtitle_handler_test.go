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

	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/subtitle"
	subfakes "github.com/gravity-zero/evey/internal/domain/subtitle/fakes"
)

// --- fake subtitle media getter (satisfies subtitle.MediaItemGetter) ---

type fakeSubtitleMediaGetter struct {
	item *subtitle.MediaItemInfo
	err  error
}

func (f *fakeSubtitleMediaGetter) GetMediaItem(_ context.Context, _ string) (*subtitle.MediaItemInfo, error) {
	return f.item, f.err
}

func newSubtitleTestMux(t *testing.T, repo *subfakes.FakeRepository, provider *subfakes.FakeProvider) *http.ServeMux {
	t.Helper()
	subMediaGetter := &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1", FilePath: "/tmp/movie.mkv", Title: "Movie"}}
	svc := appsubtitle.NewService(repo, provider, subMediaGetter)
	tv := &fakeTokenVerifier{claims: testClaims(false)}
	mg := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/tmp/movie.mkv"}}
	lac := allowAll()
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, tv, mg, lac)
	return mux
}

// --- search ---

func TestSubtitleSearch_NoAuth_401(t *testing.T) {
	svc := appsubtitle.NewService(subfakes.NewFakeRepository(), subfakes.NewFakeProvider(), &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1"}})
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, &fakeTokenVerifier{err: errTest}, &fakeMediaItemGetter{item: testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=en", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleSearch_IDOR_403(t *testing.T) {
	svc := appsubtitle.NewService(subfakes.NewFakeRepository(), subfakes.NewFakeProvider(), &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1"}})
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, &fakeTokenVerifier{claims: testClaims(false)}, &fakeMediaItemGetter{item: testMediaItem("lib-restricted")}, denyAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=en", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubtitleSearch_Success_200(t *testing.T) {
	provider := subfakes.NewFakeProvider()
	provider.Results = []subtitle.SearchResult{
		{FileID: 1, FileName: "movie.en.srt", Language: "en", Format: "srt", Rating: 8.5},
	}
	mux := newSubtitleTestMux(t, subfakes.NewFakeRepository(), provider)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=en", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Results []struct {
			FileID int    `json:"fileId"`
			Format string `json:"format"`
		} `json:"results"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestSubtitleSearch_ProviderError_500(t *testing.T) {
	provider := subfakes.NewFakeProvider()
	provider.Err = errors.New("provider down")
	mux := newSubtitleTestMux(t, subfakes.NewFakeRepository(), provider)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles/search?language=en", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- download ---

func TestSubtitleDownload_NoAuth_401(t *testing.T) {
	svc := appsubtitle.NewService(subfakes.NewFakeRepository(), subfakes.NewFakeProvider(), &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1"}})
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, &fakeTokenVerifier{err: errTest}, &fakeMediaItemGetter{item: testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`{"fileId":1,"language":"en"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleDownload_InvalidBody_400(t *testing.T) {
	mux := newSubtitleTestMux(t, subfakes.NewFakeRepository(), subfakes.NewFakeProvider())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`not json`))
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDownload_MissingFileID_400(t *testing.T) {
	mux := newSubtitleTestMux(t, subfakes.NewFakeRepository(), subfakes.NewFakeProvider())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`{"language":"en"}`))
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubtitleDownload_Success_201(t *testing.T) {
	mux := newSubtitleTestMux(t, subfakes.NewFakeRepository(), subfakes.NewFakeProvider())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/subtitles/download", strings.NewReader(`{"fileId":42,"language":"en","format":"srt"}`))
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["language"] != "en" {
		t.Errorf("expected language 'en', got %v", resp["language"])
	}
}

// --- list ---

func TestSubtitleList_NoAuth_401(t *testing.T) {
	svc := appsubtitle.NewService(subfakes.NewFakeRepository(), subfakes.NewFakeProvider(), &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1"}})
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, &fakeTokenVerifier{err: errTest}, &fakeMediaItemGetter{item: testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleList_Success_200(t *testing.T) {
	repo := subfakes.NewFakeRepository()
	repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub1", MediaItemID: "m1", Language: "en", Format: "srt",
		FilePath: "/tmp/sub.srt", Source: "opensubtitles", DownloadedAt: time.Now(),
	})
	mux := newSubtitleTestMux(t, repo, subfakes.NewFakeProvider())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Subtitles []struct {
			ID       string `json:"id"`
			Language string `json:"language"`
		} `json:"subtitles"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Subtitles) != 1 {
		t.Errorf("expected 1 subtitle, got %d", len(resp.Subtitles))
	}
}

// --- delete ---

func TestSubtitleDelete_NoAuth_401(t *testing.T) {
	svc := appsubtitle.NewService(subfakes.NewFakeRepository(), subfakes.NewFakeProvider(), &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1"}})
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, &fakeTokenVerifier{err: errTest}, &fakeMediaItemGetter{item: testMediaItem("lib1")}, allowAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubtitleDelete_NotFound_404(t *testing.T) {
	mux := newSubtitleTestMux(t, subfakes.NewFakeRepository(), subfakes.NewFakeProvider())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub-nonexistent", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSubtitleDelete_Success_204(t *testing.T) {
	repo := subfakes.NewFakeRepository()
	repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub1", MediaItemID: "m1", Language: "en", Format: "srt",
		FilePath: "/tmp/sub.srt", Source: "opensubtitles", DownloadedAt: time.Now(),
	})
	mux := newSubtitleTestMux(t, repo, subfakes.NewFakeProvider())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestSubtitleDelete_IDOR_403(t *testing.T) {
	repo := subfakes.NewFakeRepository()
	repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub1", MediaItemID: "m1", Language: "en", Format: "srt",
		FilePath: "/tmp/sub.srt", Source: "opensubtitles", DownloadedAt: time.Now(),
	})
	svc := appsubtitle.NewService(repo, subfakes.NewFakeProvider(), &fakeSubtitleMediaGetter{item: &subtitle.MediaItemInfo{ID: "m1"}})
	mux := http.NewServeMux()
	registerSubtitleRoutes(mux, svc, &fakeTokenVerifier{claims: testClaims(false)}, &fakeMediaItemGetter{item: testMediaItem("lib-restricted")}, denyAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
