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
		&fakeSegmentsGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		func() bool { return false },
		nil,
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
		&fakeSegmentsGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{err: errTest},
		func() bool { return false },
		nil,
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
		&fakeSegmentsGetter{},
		"http://localhost",
		denyAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib-restricted")},
		func() bool { return false },
		nil,
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

func TestSegmentsHandler_StreamTokenMismatch_403(t *testing.T) {
	h := segmentsHandler(
		&fakeSegmentsGetter{},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		func() bool { return false },
		nil,
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
		&fakeSegmentsGetter{segments: segs},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		func() bool { return false },
		nil,
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
		&fakeSegmentsGetter{segments: segs},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		func() bool { return false },
		nil,
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

// preview is admin-gated: withheld when the toggle is off (even if the
// client explicitly asks for it), served when on.
func TestSegmentsHandler_PreviewGating(t *testing.T) {
	segs := []*domainlibrary.MediaSegment{
		{ID: "s1", MediaItemID: "m1", Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
		{ID: "s2", MediaItemID: "m1", Type: "preview", StartMs: 90000, EndMs: 120000, Source: "tidb"},
	}
	run := func(previewEnabled bool) string {
		h := segmentsHandler(
			&fakeSegmentsGetter{segments: segs},
			"http://localhost",
			allowAll(),
			&fakeMediaItemGetter{item: testMediaItem("lib1")},
			func() bool { return previewEnabled },
			nil,
		)
		// client explicitly asks for preview
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments?types=intro,preview", nil)
		req.SetPathValue("id", "m1")
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		return w.Body.String()
	}

	off := run(false)
	if strings.Contains(off, `"preview"`) {
		t.Errorf("preview must be withheld when disabled: %s", off)
	}
	if !strings.Contains(off, `"intro"`) {
		t.Errorf("intro must still be present when preview disabled: %s", off)
	}

	on := run(true)
	if !strings.Contains(on, `"preview"`) {
		t.Errorf("preview must be served when enabled: %s", on)
	}
}

func TestSegmentsHandler_ParentalBlocked_NotFound(t *testing.T) {
	segs := []*domainlibrary.MediaSegment{
		{ID: "s1", MediaItemID: "m1", Type: "intro", StartMs: 0, EndMs: 60000, Source: "phash"},
	}
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := segmentsHandler(
		&fakeSegmentsGetter{segments: segs},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: item},
		func() bool { return false },
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from parental gate (existence hidden), got %d", w.Code)
	}
}

func TestSegmentsHandler_ParentalAdmin_OK(t *testing.T) {
	segs := []*domainlibrary.MediaSegment{
		{ID: "s1", MediaItemID: "m1", Type: "intro", StartMs: 0, EndMs: 60000, Source: "phash"},
	}
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: "R"},
	}
	h := segmentsHandler(
		&fakeSegmentsGetter{segments: segs},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: item},
		func() bool { return false },
		alwaysBlockChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/segments", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("admin must bypass parental, got %d", w.Code)
	}
}

func TestSegmentsHandler_GetError_500(t *testing.T) {
	h := segmentsHandler(
		&fakeSegmentsGetter{err: errTest},
		"http://localhost",
		allowAll(),
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		func() bool { return false },
		nil,
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
