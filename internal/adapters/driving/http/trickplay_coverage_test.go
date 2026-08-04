package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- shared helpers ---

// ============================================================
// Trickplay generate handler tests
// ============================================================

type fakeTrickplayGenerator struct {
	err error
}

func (f *fakeTrickplayGenerator) GenerateTrickplay(_ context.Context, _ string) error {
	return f.err
}

func TestTrickplayGenerate_OK(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestTrickplayGenerate_NonAdmin_403(t *testing.T) {
	v := &fakeTokenVerifier{claims: testClaims(false)}
	h := httpAdminMiddleware(v)(trickplayGenerateHandler(&fakeTrickplayGenerator{}, context.Background()))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTrickplayGenerate_NoAuth_403(t *testing.T) {
	v := &fakeTokenVerifier{err: errTest}
	h := httpAdminMiddleware(v)(trickplayGenerateHandler(&fakeTrickplayGenerator{}, context.Background()))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTrickplayGenerate_QueryToken(t *testing.T) {
	h := trickplayGenerateHandler(
		&fakeTrickplayGenerator{},
		context.Background(),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/trickplay/generate", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}
