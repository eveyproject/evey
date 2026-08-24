package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
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
	// children maps a ParentID to its child media items, used by the archive
	// (season/series) download tests via ListMediaItems.
	children map[string][]*domainlibrary.MediaItem
	// libraries backs ListLibraries; nil means "no roots known" so the archive
	// path-confinement falls open (most tests don't care about it).
	libraries []*domainlibrary.Library
}

func (f *fakeMediaItemGetter) GetMediaItem(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return f.item, f.err
}

func (f *fakeMediaItemGetter) GetMediaItemsByIDs(_ context.Context, ids []string) ([]*domainlibrary.MediaItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.item == nil {
		return nil, nil
	}
	if slices.Contains(ids, string(f.item.ID)) {
		return []*domainlibrary.MediaItem{f.item}, nil
	}
	return nil, nil
}

func (f *fakeMediaItemGetter) ListMediaItems(_ context.Context, filter domainlibrary.ListMediaItemsFilter) ([]*domainlibrary.MediaItem, error) {
	if filter.ParentID == nil {
		return nil, nil
	}
	return f.children[*filter.ParentID], nil
}

func (f *fakeMediaItemGetter) ListLibraries(_ context.Context) ([]*domainlibrary.Library, error) {
	return f.libraries, nil
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
	return &domainlibrary.MediaItem{ID: "m1", LibraryID: shared.LibraryID(libraryID), File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"}}
}

// --- tests: downloadHandler ---

func TestDownloadHandler_Unauthenticated(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	inner := downloadHandler(
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true}, nil)

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
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: false}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDownloadHandler_Allowed_FileNotFound(t *testing.T) {
	h := downloadHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent_file_xyz.mkv"}}},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true}, nil)

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
	inner := libraryStatsHandler(&fakeLibraryLister{}, nil)
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
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
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

// --- tests: serveDownload path confinement ---

func TestServeDownload_PathOutsideRoots_404(t *testing.T) {
	dir := t.TempDir()
	// Create a real file that sits outside the declared root.
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: dir + "/movie.mkv"},
	}
	if err := os.WriteFile(item.File.Path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// roots does NOT include dir, so path is outside
	roots := []string{"/some/other/root"}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	serveDownload(w, r, item, "http://localhost", roots)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when path is outside roots, got %d", w.Code)
	}
}

func TestServeDownload_EmptyRoots_FailOpen(t *testing.T) {
	dir := t.TempDir()
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: dir + "/movie.mkv"},
	}
	if err := os.WriteFile(item.File.Path, []byte("videodata"), 0o644); err != nil {
		t.Fatal(err)
	}
	// empty roots → fail open, file should be served
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	serveDownload(w, r, item, "http://localhost", nil)

	if w.Code == http.StatusNotFound && w.Body.String() == `{"message":"not found"}`+"\n" {
		t.Error("empty roots should fail open, not block the download")
	}
	// 200 or 206 (range) expected - anything but a confinement 404
	if w.Code != http.StatusOK && w.Code != http.StatusPartialContent {
		// if the stat failed for another reason that's fine - just not our confinement 404
		t.Logf("got %d (acceptable if not confinement block)", w.Code)
	}
}

func TestServeDownload_PathInsideRoots_Serves(t *testing.T) {
	dir := t.TempDir()
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: dir + "/movie.mkv"},
	}
	if err := os.WriteFile(item.File.Path, []byte("videodata"), 0o644); err != nil {
		t.Fatal(err)
	}
	// roots includes dir - path is inside
	roots := []string{dir}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	serveDownload(w, r, item, "http://localhost", roots)

	// Should not be blocked by confinement; expect 200 (or 206 for range)
	if w.Code == http.StatusNotFound {
		body := w.Body.String()
		if body == `{"message":"not found"}`+"\n" {
			t.Errorf("path inside roots should not be blocked; got 404 with body %q", body)
		}
	}
}

// withHTTPClaims injects claims into request context (simulates middleware).
func withHTTPClaims(r *http.Request, claims *domainuser.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), httpClaimsCtxKey{}, claims))
}

var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// --- tests: handleStream path confinement ---

// newStreamConfinementHandler builds the stream handler with a real on-disk
// media file and the given library roots wired through withLibraryLister.
func newStreamConfinementHandler(t *testing.T, mediaPath string, roots []string) http.HandlerFunc {
	t.Helper()
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: mediaPath},
	}
	var libs []*domainlibrary.Library
	if roots != nil {
		libs = []*domainlibrary.Library{{ID: "lib1", Paths: roots}}
	}
	return mediaHandler(
		&fakeMediaItemGetter{item: item},
		&fakeArtworkRepo{},
		"http://localhost",
		allowAll(),
		withLibraryLister(&fakeLibraryLister{libs: libs}),
	)
}

func streamRequest(h http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.SetPathValue("id", "m1")
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, testClaims(false)))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// Direct play is a bare ServeFile with no ffmpeg session behind it, so without
// this wiring a direct-play viewer reads as an idle server and every background
// worker keeps competing with them for the same disk.
func TestStream_DirectPlayMarksPlaybackActive(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(media, []byte("mkv-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	act := apptranscoding.NewPlaybackActivity(time.Minute)
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: media},
	}
	h := mediaHandler(
		&fakeMediaItemGetter{item: item}, &fakeArtworkRepo{}, "http://localhost", allowAll(),
		withLibraryLister(&fakeLibraryLister{libs: []*domainlibrary.Library{{ID: "lib1", Paths: []string{root}}}}),
		withPlaybackActivity(act),
	)

	if got := act.ActiveSessionCount(); got != 0 {
		t.Fatalf("nothing streamed yet, got %d", got)
	}
	if w := streamRequest(h); w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	// The request has returned, so the in-flight count is back to zero and the
	// TTL window is what keeps it counted - exactly the gap-covering behaviour a
	// client making successive range requests depends on.
	if got := act.ActiveSessionCount(); got != 1 {
		t.Fatalf("a direct-play read must register as active playback, got %d", got)
	}
}

// A refused request is not playback: the touch sits after every access gate, so
// a path-confinement 404 must leave the counter alone. Otherwise a poisoned row
// could hold background work off the disk without serving anyone.
func TestStream_RefusedRequestDoesNotCountAsPlayback(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	act := apptranscoding.NewPlaybackActivity(time.Minute)
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: outside},
	}
	h := mediaHandler(
		&fakeMediaItemGetter{item: item}, &fakeArtworkRepo{}, "http://localhost", allowAll(),
		withLibraryLister(&fakeLibraryLister{libs: []*domainlibrary.Library{{ID: "lib1", Paths: []string{t.TempDir()}}}}),
		withPlaybackActivity(act),
	)

	if w := streamRequest(h); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	if got := act.ActiveSessionCount(); got != 0 {
		t.Fatalf("a refused request must not count as playback, got %d", got)
	}
}

// A DB path outside every configured library root must 404 instead of being
// served - otherwise a poisoned media_files row reads arbitrary host files.
func TestStream_PathOutsideRoots_404(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	libRoot := t.TempDir() // roots elsewhere
	w := streamRequest(newStreamConfinementHandler(t, outside, []string{libRoot}))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for out-of-roots path, got %d", w.Code)
	}
}

func TestStream_PathInsideRoots_200(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(media, []byte("mkv-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := streamRequest(newStreamConfinementHandler(t, media, []string{root}))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for in-roots path, got %d (%s)", w.Code, w.Body.String())
	}
}

// No roots known (empty library list) keeps the historical fail-open
// behaviour - confinement is defense-in-depth, not the access gate.
func TestStream_NoRootsKnown_FailsOpen(t *testing.T) {
	media := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(media, []byte("mkv-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := streamRequest(newStreamConfinementHandler(t, media, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 when roots unknown, got %d", w.Code)
	}
}

// libraryAccessFunc is handed the same checker as checkLibraryAccess, which
// denies (rather than panics) when the checker is unwired. The per-item
// variant used by the archive/ZIP paths must fail closed the same way: a nil
// checker there used to dereference straight into a panic mid-response.
func TestLibraryAccessFunc_NilCheckerDeniesInsteadOfPanicking(t *testing.T) {
	allow := libraryAccessFunc(context.Background(), &domainuser.Claims{UserID: "usr-1"}, nil)
	if allow("lib-1") {
		t.Error("a nil checker must deny, not grant")
	}
}

func TestLibraryAccessFunc_NilCheckerStillGrantsAdmin(t *testing.T) {
	// Admins short-circuit before the checker is consulted at all.
	allow := libraryAccessFunc(context.Background(), &domainuser.Claims{UserID: "usr-1", IsAdmin: true}, nil)
	if !allow("lib-1") {
		t.Error("admin must keep full access")
	}
}

// A checker that ERRORS must deny, and deny the same way a plain refusal
// does. The distinction matters twice over: fail-open on a transient DB error
// would hand out another library's media during an outage, and answering 500
// (or 403) instead of 404 re-opens the existence oracle the 404 exists to
// close - "this id errored" and "this id is not yours" would read differently
// to someone probing ids.
//
// The fake has carried an `err` field all along; only the refusal path
// (ok == false) was ever asserted here.
func TestCheckLibraryAccess_CheckerError_DeniesAsNotFound(t *testing.T) {
	claims := testClaims(false)
	// allowed:true on purpose - if the error were ignored, this would PASS.
	checker := &fakeLibraryAccessChecker{allowed: true, err: errors.New("connection reset by peer")}
	w := httptest.NewRecorder()

	ok := checkLibraryAccess(context.Background(), w, claims, shared.LibraryID("lib1"), checker)

	if ok {
		t.Fatal("a failed access check must DENY - fail-closed, not fail-open")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("an errored check must be indistinguishable from a refusal (404), got %d", w.Code)
	}
}

// A nil checker is a wiring mistake, not a permission: deny.
func TestCheckLibraryAccess_NilChecker_Denies(t *testing.T) {
	w := httptest.NewRecorder()
	if checkLibraryAccess(context.Background(), w, testClaims(false), shared.LibraryID("lib1"), nil) {
		t.Fatal("a missing checker must deny, never wave the caller through")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for an unwired gate, got %d", w.Code)
	}
}

// Nil claims behind the auth middleware "cannot happen" - which is exactly
// why it must be asserted: the guard exists so a future path that reaches
// here unauthenticated is refused instead of panicking on claims.IsAdmin.
func TestCheckLibraryAccess_NilClaims_Denies(t *testing.T) {
	w := httptest.NewRecorder()
	if checkLibraryAccess(context.Background(), w, nil, shared.LibraryID("lib1"), &fakeLibraryAccessChecker{allowed: true}) {
		t.Fatal("nil claims must deny")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for nil claims, got %d", w.Code)
	}
}
