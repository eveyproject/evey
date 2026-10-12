package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	shared "github.com/eveyproject/evey/internal/domain/shared"
)

// --- shared helpers ---

// --- downloadHandler extra tests ---

func TestDownloadHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(downloadHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		allowAll(), nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDownloadHandler_StreamTokenContext(t *testing.T) {
	h := downloadHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent"}}},
		"http://localhost",
		allowAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		Token: "tok", UserID: "u1", MediaItemID: "m1",
	})
	req = req.WithContext(ctx)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusForbidden {
		t.Errorf("matching stream token in context should be accepted, got %d", w.Code)
	}
}

// A stream token minted for a different media item must never grant access
// to this one: the handler compares st.MediaItemID against the path and
// rejects with 403 before ever touching the library.
func TestDownloadHandler_StreamTokenMediaMismatch_403(t *testing.T) {
	h := downloadHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent"}}},
		"http://localhost",
		allowAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		Token: "tok", UserID: "u1", MediaItemID: "m2",
	})
	req = req.WithContext(ctx)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("mismatched stream token media id should be rejected with 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// A syncplay guest token may stream but must never download the shared
// media - the handler special-cases shared.ScopeSyncplayGuest before the
// generic media-id check.
func TestDownloadHandler_SyncplayGuestScope_403(t *testing.T) {
	h := downloadHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent"}}},
		"http://localhost",
		allowAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		Token: "tok", UserID: "u1", MediaItemID: "m1", Scope: shared.ScopeSyncplayGuest,
	})
	req = req.WithContext(ctx)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("syncplay guest token should be rejected from download with 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDownloadHandler_EmptyFilePath_404(t *testing.T) {
	h := downloadHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		"http://localhost",
		allowAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
