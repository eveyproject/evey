package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
)

// buildSubtitleRoutesWithParental constructs subtitleRoutes directly (bypassing
// registerSubtitleRoutes) so the parental gate can be swapped per test, mirroring
// the alwaysBlockChecker/allowAllParental pattern already used in parental_test.go
// for the stream/download handlers.
func buildSubtitleRoutesWithParental(svc subtitleService, mg *fakeMediaGetter, pc ParentalChecker) subtitleRoutesUnderTest {
	h := &subtitleRoutes{
		subtitleSvc:   svc,
		mediaGetter:   mg,
		libraryAccess: allowAll(),
		parental:      pc,
	}
	auth := httpAuthMiddleware(validUserValidator(false))
	return subtitleRoutesUnderTest{search: auth(h.search), download: auth(h.download), list: auth(h.list), delete: auth(h.delete)}
}

// --- Bug 1: subtitle routes skipped checkParental (list + delete) ---

func TestSubtitleList_ParentalBlocked_NotFound(t *testing.T) {
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitles", nil)
		req.SetPathValue("id", "m1")
		req.Header.Set("Authorization", "Bearer tok")
		return req
	}
	build := func(pc ParentalChecker) http.Handler {
		h := buildSubtitleRoutesWithParental(&fakeSubtitleService{}, subtitleMediaGetter("lib1"), pc)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)
		return mux
	}

	// Control: gate open, unrestricted profile still gets 200 (non-regression).
	wOK := httptest.NewRecorder()
	build(allowAllParental{}).ServeHTTP(wOK, newReq())
	if wOK.Code != http.StatusOK {
		t.Fatalf("control: open parental gate should list subtitles, got %d: %s", wOK.Code, wOK.Body.String())
	}

	// A blocked profile must not see the list - existence hidden via 404.
	wBlocked := httptest.NewRecorder()
	build(alwaysBlockChecker{}).ServeHTTP(wBlocked, newReq())
	if wBlocked.Code != http.StatusNotFound {
		t.Errorf("parental-blocked subtitle list must return 404, got %d", wBlocked.Code)
	}
}

func TestSubtitleDelete_ParentalBlocked_NotFound_AndNoDelete(t *testing.T) {
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/subtitles/sub1", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("subtitleId", "sub1")
		req.Header.Set("Authorization", "Bearer tok")
		return req
	}
	build := func(svc *fakeSubtitleService, pc ParentalChecker) http.Handler {
		h := buildSubtitleRoutesWithParental(svc, subtitleMediaGetter("lib1"), pc)
		mux := http.NewServeMux()
		mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)
		return mux
	}

	// Control: gate open, unrestricted profile can delete (non-regression).
	okSvc := &fakeSubtitleService{listSubs: []*domainsubtitle.SubtitleDownload{{ID: "sub1", MediaItemID: "m1"}}}
	wOK := httptest.NewRecorder()
	build(okSvc, allowAllParental{}).ServeHTTP(wOK, newReq())
	if wOK.Code != http.StatusNoContent {
		t.Fatalf("control: open parental gate should delete, got %d: %s", wOK.Code, wOK.Body.String())
	}
	if okSvc.lastDeleteID != "sub1" {
		t.Fatalf("control: expected delete to reach the service, lastDeleteID=%q", okSvc.lastDeleteID)
	}

	// A blocked profile must get 404 AND the delete must never reach the service.
	blockedSvc := &fakeSubtitleService{listSubs: []*domainsubtitle.SubtitleDownload{{ID: "sub1", MediaItemID: "m1"}}}
	wBlocked := httptest.NewRecorder()
	build(blockedSvc, alwaysBlockChecker{}).ServeHTTP(wBlocked, newReq())
	if wBlocked.Code != http.StatusNotFound {
		t.Errorf("parental-blocked subtitle delete must return 404, got %d", wBlocked.Code)
	}
	if blockedSvc.lastDeleteID != "" {
		t.Errorf("parental-blocked subtitle delete must NOT reach the service, but Delete was called with %q", blockedSvc.lastDeleteID)
	}
}

// --- Bug 2: artwork handler never consulted the parental gate ---

func artworkParentalHandler(t *testing.T, pc ParentalChecker) (http.Handler, string) {
	t.Helper()
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-image-data"), 0644); err != nil {
		t.Fatal(err)
	}
	localPath := imgPath
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		allowAll(),
		withParental(pc),
	)
	return h, imgPath
}

func TestArtwork_ParentalBlocked_NotFound(t *testing.T) {
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("artType", "poster")
		return withHTTPClaims(req, testClaims(false))
	}

	// Control: gate open → the poster is served.
	hOK, _ := artworkParentalHandler(t, allowAllParental{})
	wOK := httptest.NewRecorder()
	hOK.ServeHTTP(wOK, newReq())
	if wOK.Code != http.StatusOK {
		t.Fatalf("control: open parental gate should serve artwork, got %d: %s", wOK.Code, wOK.Body.String())
	}

	// A blocked profile must not receive the artwork - existence hidden via 404.
	hBlocked, _ := artworkParentalHandler(t, alwaysBlockChecker{})
	wBlocked := httptest.NewRecorder()
	hBlocked.ServeHTTP(wBlocked, newReq())
	if wBlocked.Code != http.StatusNotFound {
		t.Errorf("parental-blocked artwork must return 404, got %d", wBlocked.Code)
	}
}

// An unauthenticated caller (no claims at all) must still receive the artwork
// even with a parental checker that would block everything - artwork stays
// public for anonymous requests, mirroring checkParental's existing no-op when
// claims == nil (see TestArtwork_Public_NoAuth_200 for the library-access side
// of this same contract).
func TestArtwork_Unauthenticated_BypassesParental_200(t *testing.T) {
	h, _ := artworkParentalHandler(t, alwaysBlockChecker{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	// NOTE: no claims, no stream token - simulating a public request.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unauthenticated artwork despite an always-block parental checker, got %d: %s", w.Code, w.Body.String())
	}
}
