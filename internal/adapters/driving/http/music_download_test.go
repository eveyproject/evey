package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	domainmusic "github.com/eveyproject/evey/internal/domain/music"
	musicfakes "github.com/eveyproject/evey/internal/domain/music/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// downloadTrack: missing Authorization → 401.
func TestDownloadTrack_NoAuth_401(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, _, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// downloadTrack: track not found → 404.
func TestDownloadTrack_TrackNotFound_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/missing/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// downloadTrack: access denied → 403.
func TestDownloadTrack_AccessDenied_403(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	_, _ = repo.Upsert(context.Background(), domainmusic.Track{
		ID: "t1", LibraryID: "lib-A", Title: "T", FilePath: "/tmp/x.mp3",
	})
	mux, token, _ := newMusicMux(t, false, denyAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// downloadTrack: file missing from disk → 404.
func TestDownloadTrack_FileMissing_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	_, _ = repo.Upsert(context.Background(), domainmusic.Track{
		ID: "t1", LibraryID: "lib-A", Title: "T", FilePath: "/tmp/nonexistent-track.mp3",
	})
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// downloadTrack: success sets Content-Disposition attachment.
func TestDownloadTrack_Success_SetsAttachmentHeader(t *testing.T) {
	dir := t.TempDir()
	p := writeTempFile(t, dir, "track.mp3", []byte("fake audio bytes"))
	repo := musicfakes.NewFakeMusicTrackRepository()
	_, _ = repo.Upsert(context.Background(), domainmusic.Track{
		ID: "t1", LibraryID: shared.LibraryID("lib-A"), Title: "My Song", FilePath: p,
	})
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/tracks/t1/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%q)", w.Code, w.Body.String())
	}
	disp := w.Header().Get("Content-Disposition")
	if !strings.Contains(disp, "attachment") || !strings.Contains(disp, filepath.Ext(p)) {
		t.Fatalf("unexpected Content-Disposition: %q", disp)
	}
}

// downloadAlbum: missing params → 400.
func TestDownloadAlbum_MissingParams_400(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// downloadAlbum: no matching tracks → 404.
func TestDownloadAlbum_Empty_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums/download?libraryId=lib-A&album=Unknown", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// downloadAlbum: access denied → 403.
func TestDownloadAlbum_AccessDenied_403(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	mux, token, _ := newMusicMux(t, false, denyAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums/download?libraryId=lib-A&album=Any", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// downloadAlbum: success → 200 + zip headers.
func TestDownloadAlbum_Success_ZipResponse(t *testing.T) {
	dir := t.TempDir()
	p1 := writeTempFile(t, dir, "1.mp3", []byte("aaa"))
	p2 := writeTempFile(t, dir, "2.mp3", []byte("bbb"))
	repo := musicfakes.NewFakeMusicTrackRepository()
	_, _ = repo.Upsert(context.Background(), domainmusic.Track{
		ID: "t1", LibraryID: "lib-A", Album: "MyAlbum", Title: "One", TrackNumber: 1, FilePath: p1,
	})
	_, _ = repo.Upsert(context.Background(), domainmusic.Track{
		ID: "t2", LibraryID: "lib-A", Album: "MyAlbum", Title: "Two", TrackNumber: 2, FilePath: p2,
	})
	mux, token, _ := newMusicMux(t, false, allowAll(), repo, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/music/albums/download?libraryId=lib-A&album=MyAlbum", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("want application/zip, got %q", ct)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty zip body")
	}
}
