package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

type browseHandler struct {
	skip map[string]bool
}

type browseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func browseDirsHandler() http.HandlerFunc {
	h := &browseHandler{
		skip: map[string]bool{"cdrom": true, "floppy": true, "usb": true, "sr0": true, "proc": true, "sys": true, "dev": true},
	}
	return h.ServeHTTP
}

func (h *browseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.StartSpan(r.Context(), "http.browse.ListDirs")
	defer span.End()
	dir, ok := h.validatePath(w, r)
	if !ok {
		return
	}

	var result []browseEntry
	if dir == "" {
		result = h.listRoots()
	} else {
		result = h.listDir(w, dir)
		if result == nil {
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	jsonEncodeResponse(w, result)
}

// mediaBrowseRoot is the only tree an admin may browse to pick a library path.
const mediaBrowseRoot = "/media"

// isUnderMediaRoot reports whether p is mediaBrowseRoot itself or strictly
// inside it. A bare strings.HasPrefix(p, mediaBrowseRoot) is NOT anchored on
// a path separator, so a sibling directory whose name merely starts with the
// same characters - "/media-evil", "/media2" - would pass: it shares the
// string prefix but is a completely different tree the admin was never meant
// to browse.
func isUnderMediaRoot(p string) bool {
	return p == mediaBrowseRoot || strings.HasPrefix(p, mediaBrowseRoot+string(filepath.Separator))
}

func (h *browseHandler) validatePath(w http.ResponseWriter, r *http.Request) (string, bool) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		return "", true
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) || !isUnderMediaRoot(dir) {
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !isUnderMediaRoot(resolved) {
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return "", false
	}
	return resolved, true
}

func (h *browseHandler) listRoots() []browseEntry {
	result := []browseEntry{}
	roots, _ := filepath.Glob("/media*") //nolint:errcheck // best-effort directory discovery
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		name := filepath.Base(root)
		result = append(result, browseEntry{Name: name, Path: root})
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name()[0] == '.' || h.skip[e.Name()] {
				continue
			}
			result = append(result, browseEntry{Name: name + "/" + e.Name(), Path: root + "/" + e.Name()})
		}
	}
	return result
}

func (h *browseHandler) listDir(w http.ResponseWriter, dir string) []browseEntry {
	base := dir
	if len(base) > 1 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return nil
	}
	result := []browseEntry{}
	for _, e := range dirEntries {
		if !e.IsDir() || e.Name()[0] == '.' || h.skip[e.Name()] {
			continue
		}
		result = append(result, browseEntry{Name: e.Name(), Path: base + "/" + e.Name()})
	}
	return result
}
