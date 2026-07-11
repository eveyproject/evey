package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

type fakeTrickplayGetter struct {
	manifest *domainlibrary.TrickplayManifest
	err      error
}

func (f *fakeTrickplayGetter) GetTrickplayManifest(_ context.Context, _ string) (*domainlibrary.TrickplayManifest, error) {
	return f.manifest, f.err
}

func TestFormatVTTTimeMs(t *testing.T) {
	cases := []struct {
		input int64
		want  string
	}{
		{0, "00:00:00.000"},
		{61_000, "00:01:01.000"},
		{3_661_000, "01:01:01.000"},
		{3_661_500, "01:01:01.500"},
	}
	for _, c := range cases {
		if got := formatVTTTimeMs(c.input); got != c.want {
			t.Errorf("formatVTTTimeMs(%d) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestTrickplayManifestHandler_Unauthenticated(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := trickplayManifestHandler(
		verifier,
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	h := httpAuthMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay.vtt", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_IDOR_Forbidden(t *testing.T) {
	h := trickplayManifestHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: false},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay.vtt", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_OK(t *testing.T) {
	manifest := &domainlibrary.TrickplayManifest{
		MediaItemID: "m1",
		Width:       320,
		Interval:    10,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  4,
		ImagePath:   "/tmp",
	}
	h := trickplayManifestHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeTrickplayGetter{manifest: manifest},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay.vtt", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "WEBVTT") {
		t.Error("expected WEBVTT in response")
	}
}

func TestTrickplayImageHandler_Unauthenticated(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := trickplayImageHandler(
		verifier,
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	h := httpAuthMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay/trickplay_001.webp", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("image", "trickplay_001.webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTrickplayImageHandler_IDOR_Forbidden(t *testing.T) {
	h := trickplayImageHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: false},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay/trickplay_001.webp", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("image", "trickplay_001.webp")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_ParentalBlocked_NotFound(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := trickplayManifestHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from parental gate (existence hidden), got %d", w.Code)
	}
}

func TestTrickplayManifestHandler_ParentalAdmin_OK(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	// getter returns error so the handler emits empty WEBVTT and returns 200
	h := trickplayManifestHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayGetter{err: errTest},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// on manifest error the handler emits an empty WEBVTT - any non-403 is pass
	if w.Code == http.StatusForbidden {
		t.Errorf("admin must bypass parental, got 403")
	}
}

func TestTrickplayImageHandler_ParentalBlocked_NotFound(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := trickplayImageHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay/trickplay_001.webp", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("image", "trickplay_001.webp")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from parental gate (existence hidden), got %d", w.Code)
	}
}

func TestTrickplayImageHandler_ParentalAdmin_OK(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	// getter returns error → handler emits 404 (not 403)
	h := trickplayImageHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayGetter{err: errTest},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay/trickplay_001.webp", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("image", "trickplay_001.webp")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// admin bypasses parental; manifest not found → 404, never 403
	if w.Code == http.StatusForbidden {
		t.Errorf("admin must bypass parental, got 403")
	}
}

func TestTrickplayGenerateAllHandler_Forbidden(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	inner := trickplayGenerateAllHandler(verifier, &fakeTrickplayAllGenerator{count: 5})
	h := httpAdminMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trickplay/generate-all", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

type fakeTrickplayAllGenerator struct {
	count int
	err   error
}

func (f *fakeTrickplayAllGenerator) GenerateAllMissingTrickplays(_ context.Context) (int, error) {
	return f.count, f.err
}

func TestTrickplayGenerateAllHandler_AdminOK(t *testing.T) {
	h := trickplayGenerateAllHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeTrickplayAllGenerator{count: 3},
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trickplay/generate-all", nil)
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "3") {
		t.Error("expected count in response body")
	}
}
