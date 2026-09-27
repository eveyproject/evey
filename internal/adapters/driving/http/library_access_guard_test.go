package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/photo"
	photofakes "github.com/evey-project/evey/internal/domain/photo/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// Library-access gates that no test exercised: neutralising each one left the
// whole package green (sabotage sweep, 2026-09-28). Every case pairs the
// refusal with an open-gate control on a real file, so the 404 can only come
// from the gate - a missing fixture 404s just the same and hid gates before.

func TestMediaStream_LibraryAccessDenied_NotFound(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("fake-media-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: shared.LibraryID("lib1"), File: &domainlibrary.MediaFile{Path: mediaPath}}
	serve := func(allowed bool) *httptest.ResponseRecorder {
		h := mediaHandler(&fakeMediaItemGetter{item: item}, nil, "http://localhost", &fakeLibraryAccessChecker{allowed: allowed})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
		req.SetPathValue("id", "m1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, withHTTPClaims(req, testClaims(false)))
		return w
	}
	if w := serve(true); w.Code != http.StatusOK {
		t.Fatalf("control: an open gate must stream the file, got %d", w.Code)
	}
	if w := serve(false); w.Code != http.StatusNotFound || w.Body.String() == "fake-media-bytes" {
		t.Errorf("a library the viewer cannot access must 404 without its bytes, got %d", w.Code)
	}
}

// Selecting a poster is open to any signed-in viewer (the route is auth, not
// admin), so the library gate is the only thing between a viewer and the
// artwork of a library they were never granted.
func TestSelectArtwork_NonAdminLibraryAccessDenied_NotFound(t *testing.T) {
	checker := denyAll()
	h := selectArtworkHandler(
		&fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{
			"m1": {
				{ID: "art1", MediaItemID: "m1", Type: "poster", SortOrder: 1},
				{ID: "art2", MediaItemID: "m1", Type: "poster", SortOrder: 0},
			},
		}},
		&fakeMediaItemGetter{item: testMediaItem("lib1")},
		checker,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/artwork/art1/select", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("artworkId", "art1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, withHTTPClaims(req, testClaims(false)))
	if !checker.called.Load() || w.Code != http.StatusNotFound {
		t.Errorf("a viewer outside the library must get 404 from the gate, got %d (gate asked: %v)", w.Code, checker.called.Load())
	}
}

func TestServePhoto_LibraryAccessDenied_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.jpg")
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatal(err)
	}
	serve := func(checker LibraryAccessChecker) *httptest.ResponseRecorder {
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		tokenGen := auth.NewJWTGenerator(key, time.Minute)
		token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
		repo := photofakes.NewFakeRepository()
		if err := repo.Upsert(context.Background(), photo.Photo{ID: "ph-1", LibraryID: "lib1", FilePath: path, MimeType: "image/jpeg"}); err != nil {
			t.Fatal(err)
		}
		mux := http.NewServeMux()
		registerPhotoRoutes(mux, appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, nil), tokenGen, checker, nil, context.Background(), nil, "", nil)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph-1/serve", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	if w := serve(allowAll()); w.Code != http.StatusOK {
		t.Fatalf("control: an open gate must serve the photo, got %d", w.Code)
	}
	if w := serve(denyAll()); w.Code != http.StatusNotFound {
		t.Errorf("a library the viewer cannot access must 404, got %d", w.Code)
	}
}

// The stream lookup fails: past the gate that is a 500, so only the gate can
// answer this 404.
func TestSubtitleStream_LibraryAccessDenied_NotFound(t *testing.T) {
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"}}
	serve := func(checker LibraryAccessChecker) int {
		h := subtitleHandler(&fakeMediaStreamsGetter{item: item, sErr: errTest}, "http://localhost", checker, nil, subtitleCache{})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
		req.SetPathValue("id", "m1")
		req.SetPathValue("track", "0")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, withHTTPClaims(req, testClaims(false)))
		return w.Code
	}
	if code := serve(allowAll()); code != http.StatusInternalServerError {
		t.Fatalf("control: an open gate must reach the stream lookup (500), got %d", code)
	}
	if code := serve(denyAll()); code != http.StatusNotFound {
		t.Errorf("a library the viewer cannot access must 404, got %d", code)
	}
}
