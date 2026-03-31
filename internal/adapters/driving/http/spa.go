package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler serves static files from dir with SPA fallback to index.html.
// API requests (paths starting with /api/) are not handled.
func spaHandler(dir string) http.Handler {
	fs := http.Dir(dir)
	fileServer := http.FileServer(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve SPA for API or internal routes
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/metrics") ||
			r.URL.Path == "/healthz" {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for all other paths
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
