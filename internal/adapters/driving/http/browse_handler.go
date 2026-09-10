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
	// roots lists the trees an admin may browse. Injectable so the tests can
	// pin the rule without depending on what happens to be mounted under / on
	// the machine running them.
	roots func() []string
}

type browseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func browseDirsHandler() http.HandlerFunc {
	h := &browseHandler{
		skip:  map[string]bool{"cdrom": true, "floppy": true, "usb": true, "sr0": true, "proc": true, "sys": true, "dev": true},
		roots: discoverMediaRoots,
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

// mediaBrowseGlob is the pattern the operator's media volumes are mounted
// under. It is the SAME pattern listRoots offers, and that is the whole point:
// the two used to disagree - listRoots advertised every "/media*" directory
// while the validator only ever accepted "/media" - so on a deployment mounted
// at /media2 the picker listed the tree and then answered 400 on the first
// click into it. An admin could not configure a library at all.
const mediaBrowseGlob = "/media*"

// discoverMediaRoots returns the mounted media trees, in glob order. Resolving
// this per request rather than at boot is deliberate: a volume mounted after
// startup shows up without a restart, and the cost is one glob plus a stat per
// root on an admin-only route.
func discoverMediaRoots() []string {
	matches, _ := filepath.Glob(mediaBrowseGlob) //nolint:errcheck // best-effort directory discovery; a bad pattern is a compile-time constant here
	roots := make([]string, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}
		roots = append(roots, m)
	}
	return roots
}

// isUnderMediaRoot reports whether p is one of the browsable roots or strictly
// inside one. Each comparison is anchored on a path separator: a bare
// strings.HasPrefix would let "/media-evil" through on the strength of a shared
// string prefix with "/media", which is a different tree entirely. Being a real
// mounted "/media*" directory is what earns a root its place here - not looking
// like one.
func (h *browseHandler) isUnderMediaRoot(p string) bool {
	for _, root := range h.mediaRoots() {
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (h *browseHandler) mediaRoots() []string {
	if h.roots == nil {
		return discoverMediaRoots()
	}
	return h.roots()
}

func (h *browseHandler) validatePath(w http.ResponseWriter, r *http.Request) (string, bool) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		return "", true
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) || !h.isUnderMediaRoot(dir) {
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !h.isUnderMediaRoot(resolved) {
		http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
		return "", false
	}
	return resolved, true
}

func (h *browseHandler) listRoots() []browseEntry {
	result := []browseEntry{}
	for _, root := range h.mediaRoots() {
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
