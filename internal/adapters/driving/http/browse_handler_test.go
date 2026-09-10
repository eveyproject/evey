package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseDirs_NoAuth_401(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/media", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBrowseDirs_NonAdmin_403(t *testing.T) {
	v := validUserValidator(false)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestBrowseDirs_InvalidToken_403(t *testing.T) {
	v := invalidValidator()
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer badtoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBrowseDirs_PathTraversal_400(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=../../etc", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for relative path, got %d", w.Code)
	}
}

func TestBrowseDirs_AbsoluteNonMediaPath_400(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse?path=/etc/passwd", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-/media path, got %d", w.Code)
	}
}

// isUnderMediaRoot accepts exactly the trees listRoots offers, and nothing
// else. Two rules at once:
//   - a mounted root that is not "/media" (this deployment runs on /media2) is
//     browsable, because the picker lists it. It used to be listed and then
//     refused, which left the admin unable to configure a library at all.
//   - the match is still anchored on a path separator, so a directory that
//     merely shares a string prefix with a root is a different tree. Only a
//     root the discovery actually returned counts.
func TestIsUnderMediaRoot(t *testing.T) {
	h := &browseHandler{roots: func() []string { return []string{"/media", "/media2"} }}
	cases := []struct {
		path string
		want bool
	}{
		{"/media", true},
		{"/media/movies", true},
		{"/media2", true},
		{"/media2/Films", true},
		{"/media2/Films/Nested", true},
		// Not a discovered root, only a lookalike prefix.
		{"/media-evil", false},
		{"/media-evil/secret", false},
		{"/media22", false},
		{"/media22/foo", false},
		{"/mediaX/foo", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		if got := h.isUnderMediaRoot(c.path); got != c.want {
			t.Errorf("isUnderMediaRoot(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// Nothing is browsable when no media volume is mounted: an empty root set must
// refuse everything rather than degrade into "allow all".
func TestIsUnderMediaRoot_NoRootsRefusesEverything(t *testing.T) {
	h := &browseHandler{roots: func() []string { return nil }}
	for _, p := range []string{"/media", "/media/movies", "/media2", "/", "/etc"} {
		if h.isUnderMediaRoot(p) {
			t.Errorf("isUnderMediaRoot(%q) = true with no mounted root", p)
		}
	}
}

// The regression this closes end to end: the picker listed a directory it then
// refused to open. What listRoots offers, validatePath must accept.
func TestBrowseDirs_ListedRootIsAlsoBrowsable(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Films"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &browseHandler{
		skip:  map[string]bool{},
		roots: func() []string { return []string{root} },
	}

	// What the picker shows at the top level.
	listed := h.listRoots()
	var child string
	for _, e := range listed {
		if filepath.Base(e.Path) == "Films" {
			child = e.Path
		}
	}
	if child == "" {
		t.Fatalf("listRoots did not offer the child directory: %+v", listed)
	}

	// Clicking it must not be a 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/fs/browse?path="+child, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("browsing a listed root child = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
}

func TestBrowseDirs_NoPath_200(t *testing.T) {
	v := validUserValidator(true)
	handler := httpAdminMiddleware(v)(browseDirsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/browse", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
