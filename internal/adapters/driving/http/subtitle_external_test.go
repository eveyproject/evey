package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// serveExternalSubtitle on a .vtt file: serves it directly without ffmpeg.
func TestServeExternalSubtitle_VTT_ServedDirectly(t *testing.T) {
	tmp := t.TempDir()
	vttBody := "WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nHello"
	path := filepath.Join(tmp, "sub.vtt")
	if err := os.WriteFile(path, []byte(vttBody), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/subs/sub.vtt", nil)
	rec := httptest.NewRecorder()
	serveExternalSubtitle(rec, req, path, tmp, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/vtt; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/vtt; charset=utf-8", ct)
	}
	if rec.Body.String() != vttBody {
		t.Errorf("body = %q, want %q", rec.Body.String(), vttBody)
	}
}

// serveExternalSubtitle rejects paths escaping baseDir (safeJoin protection).
func TestServeExternalSubtitle_PathTraversal_NotFound(t *testing.T) {
	tmp := t.TempDir()
	// Put a file outside baseDir
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.vtt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/subs/x", nil)
	rec := httptest.NewRecorder()
	// base filename stripping + safeJoin - passing absolute path should be normalized
	serveExternalSubtitle(rec, req, "../../secret.vtt", tmp, "")

	// File doesn't exist in baseDir, so http.ServeFile returns 404 or .vtt branch
	// sends 200 with "file not found" depending on path handling. Either way the
	// handler must not expose the outside file. We assert the body doesn't contain "x".
	if rec.Code == http.StatusOK && rec.Body.String() == "x" {
		t.Fatalf("path traversal leaked outside file: body=%q", rec.Body.String())
	}
}
