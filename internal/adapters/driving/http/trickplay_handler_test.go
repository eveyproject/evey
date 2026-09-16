package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: false},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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
		&fakeTrickplayGetter{manifest: manifest},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
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
		&fakeTrickplayGetter{},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
		nil,
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
		&fakeTrickplayGetter{err: errTest},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true},
		&fakeMediaItemGetter{item: item},
		alwaysBlockChecker{},
		nil,
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
	inner := trickplayGenerateAllHandler(&fakeTrickplayAllGenerator{count: 5})
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

// The pre-existing *_Unauthenticated tests wrap the handler in httpAuthMiddleware
// THEMSELVES - but server.go mounted these two routes with no middleware at all,
// so those tests asserted a configuration production never ran. These reproduce
// the real anonymous request: no middleware, no claims. Trickplay sprites are
// frames of the film, so serving them unauthenticated leaks the content itself,
// and the handler's "skip library access check when no auth claims" branch also
// dropped the parental gate.
func TestTrickplayManifestHandler_AnonymousMustNotBeServed(t *testing.T) {
	h := trickplayManifestHandler(
		&fakeTrickplayGetter{err: errTest},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: false}, // caller has no right to this library
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req) // no claims in context: an anonymous caller

	// 401, not 404: the gate short-circuits on absent claims before it even
	// reaches the repository, which is the correct answer for an anonymous
	// caller. Before the fix this returned 200 with a full storyboard.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for an anonymous caller", w.Code)
	}
}

func TestTrickplayImageHandler_AnonymousMustNotBeServed(t *testing.T) {
	access := &fakeLibraryAccessChecker{allowed: false}
	h := trickplayImageHandler(
		&fakeTrickplayGetter{err: errTest},
		"http://localhost",
		access,
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay/0.webp", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("image", "0.webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// 401 discriminates properly: before the fix this handler skipped the gate
	// and fell through to a 404 (missing manifest), so asserting "not 200" alone
	// would have passed on the vulnerable code.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for an anonymous caller", w.Code)
	}
	if access.called {
		t.Error("the repository was queried for a caller with no identity")
	}
}

// The sprite URLs in the manifest must carry a credential: <img> tags cannot
// send an Authorization header, which is precisely why these routes used to be
// left open. This is the guard that keeps the fix from silently regressing into
// "secure the route, break every thumbnail".
func TestSpriteCredential_PropagatesAGuestTokenRatherThanMintingANewOne(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay", nil)
	req = req.WithContext(context.WithValue(req.Context(), rawTokenCtxKey, "guest-token-abc"))

	// A watch-together guest holds a token scoped to the ONE media it was
	// invited to. Re-using it keeps that scope; minting a fresh one off its
	// synthesized claims would widen what the guest can reach.
	got := spriteCredential(req, nil, nil, "m1")
	if got != "?token=guest-token-abc" {
		t.Errorf("got %q, want the caller's own token propagated", got)
	}
}

func TestSpriteCredential_NoCredentialWithoutAnIdentity(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay", nil)
	if got := spriteCredential(req, nil, nil, "m1"); got != "" {
		t.Errorf("got %q, want empty: nothing may be handed to an anonymous caller", got)
	}
}

// The sidecar carries the REAL per-frame timestamps the VTT is built from:
// a missing sprite dir, a missing file or a corrupt one are errors (the
// caller falls back to the nominal interval), a valid one yields the frames
// in order.
func TestReadSidecarTimestamps(t *testing.T) {
	if _, err := readSidecarTimestamps(""); err == nil {
		t.Error("empty image path must be refused")
	}
	dir := t.TempDir()
	if _, err := readSidecarTimestamps(dir); err == nil {
		t.Error("no sidecar yet: an error, not an empty list")
	}
	if err := os.WriteFile(filepath.Join(dir, "trickplay.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSidecarTimestamps(dir); err == nil {
		t.Error("a corrupt sidecar must be reported")
	}
	if err := os.WriteFile(filepath.Join(dir, "trickplay.json"), []byte(`{"frame_ms":[0,10040,20080,30120]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readSidecarTimestamps(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[1] != 10040 || got[3] != 30120 {
		t.Errorf("frames = %v", got)
	}
}
