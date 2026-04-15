package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// --- fakes ---

type fakeTokenVerifier struct {
	claims *domainuser.Claims
	err    error
}

func (f *fakeTokenVerifier) ValidateAccessToken(_ string) (*domainuser.Claims, error) {
	return f.claims, f.err
}

type fakeMediaItemGetter struct {
	item *domainlibrary.MediaItem
	err  error
}

func (f *fakeMediaItemGetter) GetMediaItem(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return f.item, f.err
}

type fakeLibraryLister struct {
	libs []*domainlibrary.Library
	err  error
}

func (f *fakeLibraryLister) ListLibraries(_ context.Context) ([]*domainlibrary.Library, error) {
	return f.libs, f.err
}

// --- helpers ---

func testClaims(admin bool) *domainuser.Claims {
	return &domainuser.Claims{UserID: "u1", ProfileID: "p1", IsAdmin: admin}
}

func testMediaItem(libraryID string) *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{ID: "m1", LibraryID: shared.LibraryID(libraryID), FilePath: "/tmp/fake.mkv"}
}

// --- tests: downloadHandler ---

func TestDownloadHandler_Unauthenticated(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	inner := downloadHandler(
		verifier,
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
	)
	h := httpAuthMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	// no Authorization header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDownloadHandler_IDOR_Forbidden(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: false},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDownloadHandler_Allowed_FileNotFound(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", FilePath: "/nonexistent_file_xyz.mkv"}},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// file doesn't exist → 404, but NOT 403
	if w.Code == http.StatusForbidden {
		t.Error("should not be forbidden when access is allowed")
	}
}

// --- tests: libraryStatsHandler ---

func TestLibraryStatsHandler_Unauthenticated(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := libraryStatsHandler(verifier, &fakeLibraryLister{}, nil)
	h := httpAdminMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_OK(t *testing.T) {
	now := time.Now()
	h := libraryStatsHandler(
		&fakeTokenVerifier{claims: testClaims(true)},
		&fakeLibraryLister{libs: []*domainlibrary.Library{
			{ID: "lib1", LastScannedAt: &now},
		}},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	req = withHTTPClaims(req, testClaims(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "lib1") {
		t.Error("expected lib1 in response")
	}
}

// --- tests: filterMasterPlaylist ---

func TestFilterMasterPlaylist_NoLimit(t *testing.T) {
	input := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nhigh.m3u8\n"
	if got := filterMasterPlaylist(input, 0); got != input {
		t.Error("with maxBps=0, content should be unchanged")
	}
}

func TestFilterMasterPlaylist_FiltersHigh(t *testing.T) {
	input := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000,CODECS=\"avc1\"\nhigh.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2000000,CODECS=\"avc1\"\nmid.m3u8\n"
	result := filterMasterPlaylist(input, 2500000)
	if strings.Contains(result, "high.m3u8") {
		t.Error("high bitrate should be filtered")
	}
	if !strings.Contains(result, "mid.m3u8") {
		t.Error("mid bitrate should remain")
	}
}

func TestFilterMasterPlaylist_KeepsLowestWhenAllExceed(t *testing.T) {
	input := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000,CODECS=\"avc1\"\nhigh.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=3000000,CODECS=\"avc1\"\nmid.m3u8\n"
	result := filterMasterPlaylist(input, 1000000)
	if !strings.Contains(result, "mid.m3u8") {
		t.Error("lowest variant should remain when all exceed limit")
	}
	if strings.Contains(result, "high.m3u8") {
		t.Error("high bitrate should be removed")
	}
}

// --- tests: checkLibraryAccess ---

func TestCheckLibraryAccess_Admin_AlwaysAllowed(t *testing.T) {
	claims := testClaims(true)
	checker := &fakeLibraryAccessChecker{allowed: false}
	w := httptest.NewRecorder()
	ok := checkLibraryAccess(context.Background(), w, claims, shared.LibraryID("lib1"), checker)
	if !ok {
		t.Error("admin should always have access")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCheckLibraryAccess_User_Denied(t *testing.T) {
	claims := testClaims(false)
	checker := &fakeLibraryAccessChecker{allowed: false}
	w := httptest.NewRecorder()
	ok := checkLibraryAccess(context.Background(), w, claims, shared.LibraryID("lib1"), checker)
	if ok {
		t.Error("should be denied")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCheckLibraryAccess_User_Allowed(t *testing.T) {
	claims := testClaims(false)
	checker := &fakeLibraryAccessChecker{allowed: true}
	w := httptest.NewRecorder()
	ok := checkLibraryAccess(context.Background(), w, claims, shared.LibraryID("lib1"), checker)
	if !ok {
		t.Error("should be allowed")
	}
}

// withHTTPClaims injects claims into request context (simulates middleware).
func withHTTPClaims(r *http.Request, claims *domainuser.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), httpClaimsCtxKey{}, claims))
}

var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
