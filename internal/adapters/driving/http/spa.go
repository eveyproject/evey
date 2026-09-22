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

		// Try to serve the file directly using the cleaned path
		cleaned := filepath.Clean(r.URL.Path)
		fullPath := filepath.Join(dir, cleaned)
		if _, err := os.Stat(fullPath); err == nil {
			setStaticCachePolicy(w, cleaned)
			r.URL.Path = cleaned
			fileServer.ServeHTTP(w, r)
			return
		}

		// A missing /assets/ file is never a client-side route: Vite
		// content-hashes every asset name, so this request is a STALE TAB
		// asking for a chunk a redeploy replaced. Answering index.html made
		// the dynamic import fail as a module error, which aborts the
		// navigation with no visible feedback - on 2026-08-25 every home
		// title looked unclickable in any tab opened before the day's
		// redeploys. A 404 is what the client's preload-error handler needs
		// to detect staleness and reload the app.
		if isHashedAsset(cleaned) {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: the index.html of the client that owns this path. A
		// second client lives in a sub-directory (the browser TV under /tv),
		// and handing it the web app's index loaded the wrong application.
		if base := clientIndex(dir, cleaned); base != "" {
			setStaticCachePolicy(w, base+"/index.html")
			r.URL.Path = base + "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		setStaticCachePolicy(w, "/index.html")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// setStaticCachePolicy sets Cache-Control per static-file class:
//
//   - /assets/*: Vite content-hashes the filename, so a changed file IS a new
//     URL - cache forever.
//   - everything else (/wasm/*, index.html, manifest, icons): mutable content
//     at a FIXED url - no-cache forces revalidation on every use (unchanged
//     files still ride Last-Modified 304s, so the cost is one conditional
//     request). Without ANY header here browsers fell back to HEURISTIC
//     freshness, and /wasm/ paid for it in prod (2026-07-26): a redeploy that
//     swapped the ac3go build generation (go -> tinygo, incompatible
//     wasm_exec.js shims) left one half cached and one half fresh - instant
//     LinkError in the worker, silent fallback to the AAC transcode. The two
//     halves are only compatible within one build, and only revalidation
//     keeps them moving in lockstep.
func setStaticCachePolicy(w http.ResponseWriter, cleanedPath string) {
	if isHashedAsset(cleanedPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// A Vite-hashed asset, of the web app or of a client served from a
// sub-directory (/tv/assets/...): same immutability, same staleness 404.
func isHashedAsset(cleanedPath string) bool {
	return strings.Contains(cleanedPath, "/assets/")
}

// The sub-directory client a path belongs to ("/tv" for /tv/anything), or ""
// when the path belongs to the web app at the root.
func clientIndex(dir, cleanedPath string) string {
	first := cleanedPath
	if i := strings.Index(strings.TrimPrefix(cleanedPath, "/"), "/"); i >= 0 {
		first = "/" + strings.TrimPrefix(cleanedPath, "/")[:i]
	}
	if first == "" || first == "/" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.Clean(first), "index.html")); err != nil {
		return ""
	}
	return first
}
