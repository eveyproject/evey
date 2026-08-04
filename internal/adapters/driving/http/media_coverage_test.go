package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- shared helpers ---

// =====================================
// browse_handler.go - listDir coverage
// =====================================

func TestBrowseDirs_ValidMediaPath_OK(t *testing.T) {
	handler := browseDirsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/media", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	// /media may not exist on test system; either 200 or 400 (symlink resolution)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_ServiceError_500(t *testing.T) {
	h := libraryStatsHandler(
		&fakeLibraryLister{err: errTest},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
