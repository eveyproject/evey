package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// --- shared helpers ---

type fakeChaptersGetter struct {
	chapters []*domainlibrary.Chapter
	err      error
}

func (f *fakeChaptersGetter) GetChapters(_ context.Context, _ string) ([]*domainlibrary.Chapter, error) {
	return f.chapters, f.err
}

// --- chaptersHandler ---
// (parental-gate coverage lives in transcode_serve_test.go:TestChaptersHandler_ParentalBlocked_NotFound)

func TestChaptersHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(chaptersHandler(
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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
		&fakeChaptersGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{err: errTest},
		nil,
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
		&fakeChaptersGetter{},
		"http://localhost",
		denyAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		nil,
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

func TestChaptersHandler_OK(t *testing.T) {
	h := chaptersHandler(
		&fakeChaptersGetter{chapters: []*domainlibrary.Chapter{
			{Title: "Ch1", StartTimeMs: 0, EndTimeMs: 60000},
		}},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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

// A block-ordered file plays through the packager: its chapters are served.
func TestChaptersHandler_BlockOrderedFileKeepsItsChapters(t *testing.T) {
	item := testMediaItem("lib1")
	item.File = &domainlibrary.MediaFile{Path: "/m/a.mkv", InterleaveBackstepMs: 366_000}
	h := chaptersHandler(
		&fakeChaptersGetter{chapters: []*domainlibrary.Chapter{{Title: "Ch1", StartTimeMs: 0, EndTimeMs: 60000}}},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: item},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/chapters", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Ch1") {
		t.Errorf("expected the chapters, got %d %s", w.Code, w.Body.String())
	}
}

func TestChaptersHandler_ServiceError_500(t *testing.T) {
	h := chaptersHandler(
		&fakeChaptersGetter{err: errTest},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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
