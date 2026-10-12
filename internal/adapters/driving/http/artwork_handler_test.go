package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	shared "github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// --- helpers for resize tests ---

func artworkHandlerWithConverter(t *testing.T, conv ImageConverter) (http.Handler, string) {
	t.Helper()
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg-original"), 0644); err != nil {
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
		withImageConverter(conv, &fakeArtworkWriter{}),
	)
	return h, imgPath
}

func TestArtwork_LocalFile_200(t *testing.T) {
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
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty body")
	}
}

func TestArtwork_SourceURL_Redirect(t *testing.T) {
	srcURL := "https://image.tmdb.org/t/p/original/poster.jpg"
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", SourceURL: &srcURL}},
		}},
		"http://localhost",
		allowAll(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if loc != srcURL {
		t.Errorf("expected redirect to %s, got %s", srcURL, loc)
	}
}

// A LocalPath whose file has vanished from disk must NOT 404; the handler must
// fall through to the SourceURL redirect when one is available (homepage poster
// regression: stale local path swallowed the remote fallback).
func TestArtwork_StaleLocalPath_FallsBackToSourceURL(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.jpg") // never written
	srcURL := "https://image.tmdb.org/t/p/original/poster.jpg"
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &missing, SourceURL: &srcURL}},
		}},
		"http://localhost",
		allowAll(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 fallback when local file missing, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != srcURL {
		t.Errorf("expected redirect to %s, got %s", srcURL, loc)
	}
}

func TestArtwork_MediaNotFound_404(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{err: errTest},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}},
		"http://localhost",
		allowAll(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestArtwork_StreamToken_OK(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("image-bytes"), 0644); err != nil {
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
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "u1",
		MediaItemID: "m1",
	})
	ctx = context.WithValue(ctx, httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// Artwork is now public: unauthenticated requests must serve the image (200),
// bypassing library access checks entirely (no claims in context).
func TestArtwork_Public_NoAuth_200(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-image-data"), 0644); err != nil {
		t.Fatal(err)
	}
	localPath := imgPath
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		denyAll(), // would deny if checked; proves check is skipped for anonymous requests
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	// NOTE: no claims, no stream token - simulating a public request.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for public artwork, got %d: %s", w.Code, w.Body.String())
	}
}

func TestArtwork_IDOR_403(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-restricted"}},
		&fakeArtworkRepo{},
		"http://localhost",
		denyAll(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// A self-referential parent_id (poisoned/buggy row) must not spin the parent
// walk forever: the depth guard bounds it and the handler returns a no-artwork
// response instead of hanging. The test completing at all is the assertion.
func TestArtwork_ParentCycle_Terminates(t *testing.T) {
	h := mediaHandler(
		// ParentID points back at the item itself; GetMediaItem always returns
		// it, so an unbounded walk would loop forever.
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-1", ParentID: "m1"}},
		&fakeArtworkRepo{}, // no records → walk never short-circuits on a hit
		"http://localhost",
		allowAll(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 after bounded parent walk, got %d", w.Code)
	}
}

// An item that simply has no artwork records must NOT 404: a 404 makes the
// browser log a console/network error for every posterless card. The handler
// returns 204 No Content (empty body, mutable short cache) so the <img> load
// fails silently and the client renders its own placeholder.
func TestArtwork_NoRecords_204_NoConsoleError(t *testing.T) {
	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}}, // item exists, no artwork
		"http://localhost",
		allowAll(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for an item with no artwork, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
	// Must be cacheable but NOT immutable: lite feed URLs are unversioned, so an
	// immutable 204 would freeze "no poster" even after artwork is added.
	if cc := w.Header().Get("Cache-Control"); cc == "" || strings.Contains(cc, "immutable") {
		t.Errorf("expected a mutable short cache, got Cache-Control=%q", cc)
	}
}

// --- resize variant tests ---

func TestArtwork_Width_400_ServeResized(t *testing.T) {
	conv := &fakeImageConverter{}
	h, _ := artworkHandlerWithConverter(t, conv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster?w=400", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", ct)
	}
	if conv.resizeCalled == 0 {
		t.Error("expected ResizeAndConvertToWebP to be called")
	}
}

func TestArtwork_Width_999_SnapsToNearest(t *testing.T) {
	// 999 snaps to 780; 1280/1920 are exact matches for the hero's srcSet.
	conv := &fakeImageConverter{}
	h, _ := artworkHandlerWithConverter(t, conv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster?w=999", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if conv.resizeCalled == 0 {
		t.Error("expected ResizeAndConvertToWebP to be called for snapped width")
	}
}

func TestParseRequestedWidth_Snap(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"", 0},
		{"abc", 0},
		{"0", 0},
		{"-5", 0},
		{"185", 185},
		{"200", 185}, // nearest to 185 (diff=15) vs 342 (diff=142)
		{"300", 342}, // nearest to 342 (diff=42) vs 185 (diff=115)
		{"400", 342}, // 400-342=58 < 500-400=100
		{"999", 780},
		// Backdrop-scale entries. Before these existed, the detail hero's
		// srcSet (800/1280/1920) collapsed onto the single 780px variant, so a
		// wide viewport stretched one blurry file across the whole page.
		{"1280", 1280},
		{"1920", 1920},
		{"1500", 1280}, // 1500-1280=220 < 1920-1500=420
		{"3000", 1920}, // anything larger still snaps to the ceiling
	}
	for _, tc := range cases {
		url := "/test"
		if tc.query != "" {
			url += "?w=" + tc.query
		}
		r := httptest.NewRequest(http.MethodGet, url, nil)
		got := parseRequestedWidth(r)
		if got != tc.want {
			t.Errorf("parseRequestedWidth(w=%q) = %d, want %d", tc.query, got, tc.want)
		}
	}
}

func TestArtwork_Width_Empty_NoResize(t *testing.T) {
	conv := &fakeImageConverter{}
	h, _ := artworkHandlerWithConverter(t, conv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if conv.resizeCalled != 0 {
		t.Error("ResizeAndConvertToWebP should NOT be called when no ?w= param")
	}
}

func TestArtwork_Width_CacheHit_NoSecondResize(t *testing.T) {
	conv := &fakeImageConverter{}
	h, imgPath := artworkHandlerWithConverter(t, conv)

	// Pre-create the variant so it already exists on disk - simulates a cache hit.
	variantPath := buildVariantPath(imgPath, 342)
	if err := os.WriteFile(variantPath, []byte("RIFF-webp-cached"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster?w=342", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if conv.resizeCalled != 0 {
		t.Errorf("ResizeAndConvertToWebP should NOT be called on cache hit, called %d times", conv.resizeCalled)
	}
}
