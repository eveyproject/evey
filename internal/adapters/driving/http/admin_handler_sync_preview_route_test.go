package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

type stubSyncCatalog struct{ path string }

func (c stubSyncCatalog) ListMediaFiles(context.Context, string) ([]appmaintenance.MediaRef, error) {
	return nil, nil
}

func (c stubSyncCatalog) GetMediaFile(_ context.Context, id string) (appmaintenance.MediaRef, error) {
	if c.path == "" {
		return appmaintenance.MediaRef{}, errors.New("not found")
	}
	return appmaintenance.MediaRef{MediaID: id, Path: c.path}, nil
}

type stubSyncRenderer struct {
	clip []byte
	err  error
	got  appmaintenance.SyncPreviewRequest
}

func (r *stubSyncRenderer) RenderSyncPreview(_ context.Context, req appmaintenance.SyncPreviewRequest) ([]byte, error) {
	r.got = req
	return r.clip, r.err
}

func newSyncPreviewMux(t *testing.T, catalog stubSyncCatalog, renderer *stubSyncRenderer) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	svc := appmaintenance.NewSeekIndexService(nil, catalog, nil)
	if renderer != nil {
		svc.WithSyncPreviewRenderer(renderer)
	}
	mux := http.NewServeMux()
	registerSyncPreviewRoute(mux, svc, gen, ratelimit.NewWithBurst(100, 100), nil)
	return mux, adminTok, userTok
}

func syncPreviewGet(mux *http.ServeMux, token, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/m1/sync-preview"+query, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// The route forks an ffmpeg per call: it must be admin-only, and the clip it
// returns is a fresh judgement every time (never cacheable).
func TestSyncPreviewRoute_AdminGate(t *testing.T) {
	mux, _, userTok := newSyncPreviewMux(t, stubSyncCatalog{path: "/media/film.mkv"}, &stubSyncRenderer{clip: []byte("mp4")})
	if w := syncPreviewGet(mux, "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", w.Code)
	}
	if w := syncPreviewGet(mux, userTok, ""); w.Code != http.StatusForbidden {
		t.Errorf("non-admin: got %d, want 403", w.Code)
	}
}

func TestSyncPreviewRoute_ServesClipWithQueryParams(t *testing.T) {
	r := &stubSyncRenderer{clip: bytes.Repeat([]byte("x"), 1024)}
	mux, adminTok, _ := newSyncPreviewMux(t, stubSyncCatalog{path: "/media/film.mkv"}, r)
	w := syncPreviewGet(mux, adminTok, "?at=120&len=15&track=1&shift=-0.5")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("content-type = %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control = %q", cc)
	}
	if w.Body.Len() != 1024 {
		t.Errorf("body = %d bytes", w.Body.Len())
	}
	if r.got.Path != "/media/film.mkv" || r.got.AtSec != 120 || r.got.LenSec != 15 || r.got.Track != 1 || r.got.ShiftSec != -0.5 {
		t.Errorf("renderer request = %+v", r.got)
	}
}

func TestSyncPreviewRoute_RenderFailureIs422(t *testing.T) {
	mux, adminTok, _ := newSyncPreviewMux(t, stubSyncCatalog{path: "/media/film.mkv"}, &stubSyncRenderer{err: errors.New("ffmpeg exited 1")})
	w := syncPreviewGet(mux, adminTok, "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ffmpeg exited 1") {
		t.Errorf("body should carry the renderer error: %s", w.Body.String())
	}
}

func TestSyncPreviewRoute_UnknownMediaIs422(t *testing.T) {
	mux, adminTok, _ := newSyncPreviewMux(t, stubSyncCatalog{}, &stubSyncRenderer{clip: []byte("mp4")})
	if w := syncPreviewGet(mux, adminTok, ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", w.Code)
	}
}

func TestSyncPreviewRoute_NoRendererIs422(t *testing.T) {
	mux, adminTok, _ := newSyncPreviewMux(t, stubSyncCatalog{path: "/media/film.mkv"}, nil)
	w := syncPreviewGet(mux, adminTok, "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not available") {
		t.Errorf("body = %s", w.Body.String())
	}
}

// A clip past the cap means the renderer misbehaved; the browser is told, not
// handed 64 MB of it.
func TestSyncPreviewRoute_OversizedClipIs507(t *testing.T) {
	mux, adminTok, _ := newSyncPreviewMux(t, stubSyncCatalog{path: "/media/film.mkv"}, &stubSyncRenderer{clip: make([]byte, syncPreviewMaxBytes+1)})
	if w := syncPreviewGet(mux, adminTok, ""); w.Code != http.StatusInsufficientStorage {
		t.Fatalf("got %d, want 507", w.Code)
	}
}
