package httpapi

import (
	"net/http"
	"strings"
)

// CORSMiddleware lets a client that does not live on this server's origin
// talk to the API: the packaged television app (webOS, Tizen) runs from a
// file:// or app:// origin - the browser reports it as "null" - and the
// installable /pwa client opened from another host. Two regimes, on purpose:
//
//   - The configured frontend origin (baseURL) is the web app itself, which
//     authenticates with cookies: it gets Allow-Credentials, as the
//     per-handler setCORSHeaders already granted it.
//   - Every other origin gets the response WITHOUT credentials. A cross-site
//     page can then read an API answer only for a request it authenticated
//     itself, with a Bearer it holds; a request riding the victim's cookie
//     still runs (the CSRF middleware guards the writes) but its answer is
//     never handed to the page. This is what makes "any origin" safe here.
//
// Preflights (OPTIONS with Access-Control-Request-Method) are answered
// directly: nothing downstream knows OPTIONS, and a preflight that reaches
// the gateway is a 404 the browser reads as "refused". Only API paths and
// /healthz (the server probe of every client) take part; static files
// belong to the web app and need no CORS.
func CORSMiddleware(baseURL string, next http.Handler) http.Handler {
	return CORSMiddlewareLive(func() string { return baseURL }, next)
}

// CORSMiddlewareLive reads the public address on every request, so an External
// URL saved in Admin -> Remote Access applies without a restart.
func CORSMiddlewareLive(baseURL func() string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed := strings.TrimRight(baseURL(), "/")
		origin := r.Header.Get("Origin")
		if origin == "" || !corsPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Add("Vary", "Origin")
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Expose-Headers", "X-Evey-Error, Retry-After, ETag, X-Evey-Remote-Access")
		if strings.TrimRight(origin, "/") == allowed {
			h.Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			// The headers the client announced are the ones it needs
			// (Authorization, Content-Type, X-Persistent-Session, If-None-Match,
			// ...); echoing them is safe because no credential rides on a
			// header the page did not set itself.
			if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
				h.Set("Access-Control-Allow-Headers", req)
			}
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func corsPath(p string) bool {
	return strings.HasPrefix(p, "/api/") || p == "/healthz"
}
