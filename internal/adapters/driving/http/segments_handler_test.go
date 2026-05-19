package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// --- fakes ---

type fakeSegmentsGetter struct {
	segments []*domainlibrary.MediaSegment
	err      error
}

func (f *fakeSegmentsGetter) GetSegments(_ context.Context, _ string) ([]*domainlibrary.MediaSegment, error) {
	return f.segments, f.err
}

// --- tests ---

func TestSegmentsHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(segmentsHandler(
		verifier,
		&fakeSegmentsGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSegmentsHandler_NotFound_404(t *testing.T) {
	h := segmentsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeSegmentsGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{err: errTest},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSegmentsHandler_IDOR_403(t *testing.T) {
	h := segmentsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeSegmentsGetter{},
		"http://localhost",
		denyAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSegmentsHandler_StreamTokenMismatch_403(t *testing.T) {
	h := segmentsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeSegmentsGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	// inject stream token for a DIFFERENT media id
	st := &shared.StreamToken{MediaItemID: shared.MediaItemID("m2"), UserID: shared.UserID("u1")}
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, st))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for stream token media mismatch, got %d", w.Code)
	}
}

func TestSegmentsHandler_OK(t *testing.T) {
	segs := []*domainlibrary.MediaSegment{
		{ID: "s1", MediaItemID: "m1", Type: "intro", StartMs: 0, EndMs: 60000, Confidence: 0.95, Source: "phash"},
		{ID: "s2", MediaItemID: "m1", Type: "outro", StartMs: 1800000, EndMs: 1860000, Confidence: 0.88, Source: "phash"},
	}
	h := segmentsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeSegmentsGetter{segments: segs},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Segments []struct {
			Type    string `json:"type"`
			StartMs int64  `json:"startMs"`
			EndMs   int64  `json:"endMs"`
			Source  string `json:"source"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(body.Segments))
	}
}

func TestSegmentsHandler_TypeFilter(t *testing.T) {
	segs := []*domainlibrary.MediaSegment{
		{ID: "s1", MediaItemID: "m1", Type: "intro", StartMs: 0, EndMs: 60000, Source: "phash"},
		{ID: "s2", MediaItemID: "m1", Type: "outro", StartMs: 1800000, EndMs: 1860000, Source: "phash"},
		{ID: "s3", MediaItemID: "m1", Type: "recap", StartMs: 0, EndMs: 30000, Source: "manual"},
	}
	h := segmentsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeSegmentsGetter{segments: segs},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments?types=intro,outro", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"intro"`) || !strings.Contains(body, `"outro"`) {
		t.Errorf("expected intro+outro segments, got %s", body)
	}
	if strings.Contains(body, `"recap"`) {
		t.Errorf("recap should be filtered out: %s", body)
	}
}

func TestSegmentsHandler_GetError_500(t *testing.T) {
	h := segmentsHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeSegmentsGetter{err: errTest},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
