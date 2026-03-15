package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowseDirs_NoAuth_401(t *testing.T) {
	handler := browseDirsHandler(validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/media", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBrowseDirs_NonAdmin_403(t *testing.T) {
	handler := browseDirsHandler(validUserValidator(false))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestBrowseDirs_InvalidToken_403(t *testing.T) {
	handler := browseDirsHandler(invalidValidator())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer badtoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestBrowseDirs_PathTraversal_400(t *testing.T) {
	handler := browseDirsHandler(validUserValidator(true))
	// relative path traversal attempt
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=../../etc", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for relative path, got %d", w.Code)
	}
}

func TestBrowseDirs_AbsoluteNonMediaPath_400(t *testing.T) {
	handler := browseDirsHandler(validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/etc/passwd", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-/media path, got %d", w.Code)
	}
}

func TestBrowseDirs_NoPath_200(t *testing.T) {
	handler := browseDirsHandler(validUserValidator(true))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
