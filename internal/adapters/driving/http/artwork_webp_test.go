package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	shared "github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

type fakeImageConverter struct {
	called       bool
	resizeCalled int
	failErr      error
}

func (f *fakeImageConverter) ConvertToWebP(_ context.Context, src, dst string, quality int) error {
	f.called = true
	if f.failErr != nil {
		return f.failErr
	}
	return os.WriteFile(dst, []byte("RIFF-webp-fake"), 0644)
}

func (f *fakeImageConverter) ResizeAndConvertToWebP(_ context.Context, src, dst string, width, quality int) error {
	f.resizeCalled++
	if f.failErr != nil {
		return f.failErr
	}
	// Write a smaller fake to distinguish from full-res.
	return os.WriteFile(dst, []byte("RIFF-webp-resized"), 0644)
}

type fakeArtworkWriter struct {
	lastID   string
	lastPath string
}

func (f *fakeArtworkWriter) UpdateWebPPath(_ context.Context, id, webpPath string) error {
	f.lastID = id
	f.lastPath = webpPath
	return nil
}

func TestArtwork_WebP_AcceptHeader_ServesWebP(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	localPath := imgPath
	conv := &fakeImageConverter{}
	writer := &fakeArtworkWriter{}

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		allowAll(),
		withImageConverter(conv, writer),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/webp,image/png,*/*")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !conv.called {
		t.Error("expected converter to be called")
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", ct)
	}
	if v := w.Header().Get("Vary"); v != "Accept" {
		t.Errorf("expected Vary: Accept, got %s", v)
	}
	if writer.lastID != "a1" {
		t.Errorf("expected artwork writer to save id a1, got %s", writer.lastID)
	}
}

func TestArtwork_WebP_ExistingWebPPath(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	webpPath := filepath.Join(tmp, "poster.webp")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webpPath, []byte("RIFF-webp-cached"), 0644); err != nil {
		t.Fatal(err)
	}

	localPath := imgPath
	wp := webpPath
	conv := &fakeImageConverter{}

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath, WebPPath: &wp}},
		}},
		"http://localhost",
		allowAll(),
		withImageConverter(conv, &fakeArtworkWriter{}),
	)

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
	if conv.called {
		t.Error("converter should NOT be called when webp already exists")
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", ct)
	}
}

func TestArtwork_WebP_ZeroByteVariant_Regenerated(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
	// A leftover 0-byte variant from a prior failed/timed-out resize must be
	// treated as "not cached" and regenerated - never served as an empty 200
	// (which `immutable` caching would then freeze in clients).
	variant := imgPath[:len(imgPath)-len(filepath.Ext(imgPath))] + "_w500.webp"
	if err := os.WriteFile(variant, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	localPath := imgPath
	conv := &fakeImageConverter{}

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		allowAll(),
		withImageConverter(conv, &fakeArtworkWriter{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster?w=500", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/webp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if conv.resizeCalled == 0 {
		t.Error("expected resize to regenerate the 0-byte variant")
	}
	if w.Body.Len() == 0 {
		t.Error("must never serve an empty variant body")
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", ct)
	}
}

func TestArtwork_NoAcceptWebP_ServesOriginal(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	localPath := imgPath
	conv := &fakeImageConverter{}

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		allowAll(),
		withImageConverter(conv, &fakeArtworkWriter{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req = withHTTPClaims(req, testClaims(false))
	req.Header.Set("Accept", "image/jpeg")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if conv.called {
		t.Error("converter should NOT be called without webp accept")
	}
	if v := w.Header().Get("Vary"); v != "Accept" {
		t.Errorf("expected Vary: Accept, got %s", v)
	}
}

func TestArtwork_WebP_StreamToken(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	localPath := imgPath
	conv := &fakeImageConverter{}

	h := mediaHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}},
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {{ID: "a1", MediaItemID: "m1", Type: "poster", LocalPath: &localPath}},
		}},
		"http://localhost",
		allowAll(),
		withImageConverter(conv, &fakeArtworkWriter{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/artwork/poster", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artType", "poster")
	req.Header.Set("Accept", "image/webp")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "u1",
		MediaItemID: "m1",
	})
	ctx = context.WithValue(ctx, httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !conv.called {
		t.Error("expected converter to be called for webp")
	}
}
