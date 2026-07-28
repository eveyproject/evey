package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowseDirs_NoAuth_401(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/media", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBrowseDirs_NonAdmin_403(t *testing.T) {
	v := validUserValidator(false)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestBrowseDirs_InvalidToken_403(t *testing.T) {
	v := invalidValidator()
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer badtoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBrowseDirs_PathTraversal_400(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=../../etc", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for relative path, got %d", w.Code)
	}
}

func TestBrowseDirs_AbsoluteNonMediaPath_400(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/etc/passwd", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-/media path, got %d", w.Code)
	}
}

// isUnderMediaRoot must anchor on a path separator: a sibling directory
// whose name merely starts with "/media" ("/media-evil", "/media2") is a
// different tree entirely and must never be treated as inside it.
func TestIsUnderMediaRoot(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/media", true},
		{"/media/movies", true},
		{"/media-evil", false},
		{"/media-evil/secret", false},
		{"/media2", false},
		{"/mediaX/foo", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		if got := isUnderMediaRoot(c.path); got != c.want {
			t.Errorf("isUnderMediaRoot(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestBrowseDirs_NoPath_200(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
