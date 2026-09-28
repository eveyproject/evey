package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crypto/rand"
	"crypto/rsa"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/photo"
	photofakes "github.com/evey-project/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// A row pointing outside every library root is refused before its file is
// read or produced: the confinement keeps a bad path (a hand-edited row, a
// scan bug) from turning a media route into an arbitrary-file read. The file
// stream had this test; these four routes did not, and neutralising their
// check left the package green (sabotage sweep, 2026-09-28).
func rootsElsewhere(t *testing.T) *fakeMediaItemGetter {
	t.Helper()
	return &fakeMediaItemGetter{libraries: []*domainlibrary.Library{{ID: "lib1", Paths: []string{t.TempDir()}}}}
}

func TestRemux_PathOutsideRoots_NeverProduced(t *testing.T) {
	rmx := &fakeRemuxer{canRemux: true, payload: []byte("DATA")}
	h := newTestRemuxServe(t, rmx, allowAll())
	h.libLister = rootsElsewhere(t)
	w := doRemuxRequest(h, false, "")
	if w.Code == http.StatusOK || rmx.calls != 0 {
		t.Errorf("an out-of-roots file must not be remuxed, got %d with %d remux calls", w.Code, rmx.calls)
	}
}

func TestCompatDownload_PathOutsideRoots_NotFound(t *testing.T) {
	tr := &fakeCompatTranscoder{payload: []byte("x")}
	h := newTestCompatServe(t, tr, allowAll())
	h.libLister = rootsElsewhere(t)
	w := doCompatRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/compat-download", false)
	if w.Code != http.StatusNotFound {
		t.Errorf("an out-of-roots file must not be prepared, got %d", w.Code)
	}
}

func TestReducedDownload_PathOutsideRoots_NotFound(t *testing.T) {
	h := newTestReducedServe(t, newFakeReducedTranscoder(3), allowAll(), nil)
	h.libLister = rootsElsewhere(t)
	w := doReducedRequest(h.handlePrepare, http.MethodPost, "/api/v1/media/m1/reduced-download")
	if w.Code != http.StatusNotFound {
		t.Errorf("an out-of-roots file must not be prepared, got %d", w.Code)
	}
}

func TestServePhoto_PathOutsideRoots_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.jpg")
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatal(err)
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1"})
	repo := photofakes.NewFakeRepository()
	if err := repo.Upsert(context.Background(), photo.Photo{ID: "ph-1", LibraryID: "lib1", FilePath: path, MimeType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerPhotoRoutes(mux, appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, nil), tokenGen, allowAll(), rootsElsewhere(t), context.Background(), nil, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph-1/serve", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("an out-of-roots photo must not be served, got %d", w.Code)
	}
}
