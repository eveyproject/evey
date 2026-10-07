package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

var unsafeFilenameChars = regexp.MustCompile(`[^\w\s.\-()]`)

func jsonEncodeResponse(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"message":"encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\"", "")
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	if name == "" {
		name = "download"
	}
	return name
}

// isWithinDir reports whether path is strictly inside dir (both cleaned).
func isWithinDir(dir, path string) bool {
	cleanDir := filepath.Clean(dir) + string(filepath.Separator)
	return strings.HasPrefix(filepath.Clean(path), cleanDir)
}

// safeJoin joins base and rel, then verifies the result is strictly inside base.
func safeJoin(base, rel string) (string, error) {
	joined := filepath.Join(base, rel)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	if abs != baseAbs && !strings.HasPrefix(abs, baseAbs+string(filepath.Separator)) {
		return "", errors.New("path traversal detected")
	}
	return abs, nil
}

// extractBearerToken reads the Bearer token from Authorization header or ?token query param.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return h[7:]
	}
	return r.URL.Query().Get("token")
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request, baseURL string) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	allowed := strings.TrimRight(baseURL, "/")
	// Token-authenticated requests (e.g. Chromecast fetching HLS) are allowed
	// from any origin - the token itself is the authorization proof.
	rawTok := RawTokenFromContext(r.Context())
	originMatches := strings.TrimRight(origin, "/") == allowed
	if rawTok != "" || originMatches {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		// Expose the shed contract headers so a cross-origin player (e.g.
		// Chromecast token-auth) can read the load-shed reason / Retry-After off
		// a 503, exactly as a same-origin player already can.
		w.Header().Set("Access-Control-Expose-Headers", "X-Evey-Error, Retry-After")
		// Credentials (cookies) only when the origin is our own configured
		// frontend - token-auth requests carry their own bearer and must NOT
		// piggy-back on cookie-based session auth across arbitrary origins.
		if originMatches {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
	}
}

// serveFileWithCache serves an immutable file with a year-long cache. shared
// says whether a proxy or CDN may keep a copy: true for a public route
// (artwork), false for one that authenticated through ?token= (trickplay
// sprites), whose response belongs to that viewer's browser cache only.
func serveFileWithCache(w http.ResponseWriter, r *http.Request, filePath string, shared bool) {
	fi, err := os.Stat(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	etag := fmt.Sprintf(`"%x-%x"`, fi.ModTime().Unix(), fi.Size())
	w.Header().Set("ETag", etag)
	scope := "private"
	if shared {
		scope = "public"
	}
	w.Header().Set("Cache-Control", scope+", max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeFile(w, r, filePath)
}

func checkLibraryAccess(ctx context.Context, w http.ResponseWriter, claims *domainuser.Claims, libraryID shared.LibraryID, checker LibraryAccessChecker) bool {
	// Fail closed: a nil claims (shouldn't happen behind auth middleware) is
	// denied rather than panicking on claims.IsAdmin.
	if claims == nil {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return false
	}
	if claims.IsAdmin {
		return true
	}
	if checker == nil {
		slog.WarnContext(ctx, "library access checker is nil, denying access", slog.String("user_id", claims.UserID), slog.String("library_id", string(libraryID)))
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return false
	}
	ok, err := checker.HasAccess(ctx, shared.UserID(claims.UserID), libraryID)
	if err != nil || !ok {
		// 404, not 403: returning "forbidden" for a resource the caller can't
		// access but "not found" for one that doesn't exist is an existence
		// oracle - a profile could enumerate which media IDs live in libraries
		// it has no grant to. Mirror the not-found response so the two are
		// indistinguishable. (err is logged but still denied, fail-closed.)
		if err != nil {
			slog.WarnContext(ctx, "library access check failed", slog.String("user_id", claims.UserID), slog.String("library_id", string(libraryID)), slog.Any("err", err))
		} else {
			// ok == false: a caller probing a library it holds no grant to. Log
			// the deny (not just infra errors) so repeated cross-library
			// enumeration by one profile is visible/alertable to operators.
			slog.WarnContext(ctx, "library access denied", slog.String("user_id", claims.UserID), slog.String("library_id", string(libraryID)))
		}
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return false
	}
	return true
}

// hlsCookieName is the httpOnly cookie that carries a media's opaque stream
// token for the web player. It lets a native <video> HLS element (and hls.js)
// authenticate the master playlist and every segment without the token ever
// appearing in the URL: it can't then leak via history/logs/referrer, and a
// native player that cannot rewrite segment URIs keeps working across the
// token's sliding TTL (which no query-baked token allows).
const hlsCookieName = "evey_hls"

// TokenRedactMiddleware moves ?token= from URL query to Authorization header.
// For opaque stream tokens (no "." = not a JWT), it resolves the token via the store
// and injects synthetic claims into the context so handlers work unchanged.
func TokenRedactMiddleware(next http.Handler) http.Handler {
	return tokenRedactMiddleware(nil, false, next)
}

// TokenRedactMiddlewareWithStore creates a token middleware that also resolves
// opaque stream tokens and, for HLS requests, mints/slides the evey_hls cookie.
// httpsOnly sets the cookie's Secure flag (true behind a TLS-terminating proxy).
func TokenRedactMiddlewareWithStore(store streamTokenValidator, httpsOnly bool, next http.Handler) http.Handler {
	return tokenRedactMiddleware(store, httpsOnly, next)
}

// TokenRedactMiddlewareLive reads the HTTPS flag on every request.
func TokenRedactMiddlewareLive(store streamTokenValidator, httpsOnly func() bool, next http.Handler) http.Handler {
	return tokenRedactMiddlewareFn(store, httpsOnly, next)
}

func tokenRedactMiddleware(store streamTokenValidator, httpsOnly bool, next http.Handler) http.Handler {
	return tokenRedactMiddlewareFn(store, fixedBool(httpsOnly), next)
}

// fixedBool adapts a boot-time flag to the per-request form.
func fixedBool(b bool) func() bool { return func() bool { return b } }

type streamTokenValidator interface {
	ValidateAny(token string) (*shared.StreamToken, bool)
}

func isMediaPath(p string) bool { return strings.HasPrefix(p, "/api/v1/media/") }
func isHLSPath(p string) bool   { return strings.Contains(p, "/hls/") }

// setHLSCookie (re)issues evey_hls scoped to the media's own /hls path, so it
// is sent for that item's master playlist and every segment but nothing else.
// Called on each validated HLS request, its Max-Age tracks the token's sliding
// TTL: the master fetch mints it, each segment slides it.
func setHLSCookie(w http.ResponseWriter, reqPath, token string, ttl time.Duration, httpsOnly bool) {
	before, _, found := strings.Cut(reqPath, "/hls/")
	if !found {
		return
	}
	maxAge := int(ttl.Seconds())
	if maxAge <= 0 {
		maxAge = int((5 * time.Minute).Seconds())
	}
	//nolint:gosec // G124: Secure follows the reverse-proxy's forwarded-proto
	// (same as the evey_refresh cookie) so it works on a plain-http dev origin;
	// HttpOnly + SameSite=Lax are always set and it is path-scoped to the media.
	http.SetCookie(w, &http.Cookie{
		Name:     hlsCookieName,
		Value:    token,
		Path:     before + "/hls",
		HttpOnly: true,
		Secure:   httpsOnly,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func tokenRedactMiddlewareFn(store streamTokenValidator, httpsOnly func() bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The player hits the media routes as raw URLs, so a stream token can
		// arrive three ways: ?token= (legacy web + cast), the evey_hls httpOnly
		// cookie (native <video>/hls.js - auto-sent same-origin, and the only
		// channel a native HLS element has), or an opaque Bearer token (mobile
		// expo-video passes it as a request header). Resolve whichever is present
		// into the Authorization + context shape the downstream stream-auth wants.
		rawTok := r.URL.Query().Get("token")
		fromQuery := rawTok != ""
		if fromQuery {
			// Keep the token out of the (logged / referrer'd) URL.
			r = r.WithContext(context.WithValue(r.Context(), rawTokenCtxKey, rawTok))
			// Remember the token arrived by query so cookie-less second devices
			// (Chromecast) get the token baked back into on-demand manifest URIs;
			// same-origin cookie/Bearer clients keep bare URIs (token stays out
			// of their URLs).
			r = r.WithContext(context.WithValue(r.Context(), tokenFromQueryCtxKey, true))
			q := r.URL.Query()
			q.Del("token")
			r.URL.RawQuery = q.Encode()
		} else if isMediaPath(r.URL.Path) {
			// No query token: accept the cookie, else an opaque Bearer. A real
			// JWT Bearer is left untouched for the downstream verifier.
			if c, err := r.Cookie(hlsCookieName); err == nil && c.Value != "" {
				rawTok = c.Value
			} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				if b := strings.TrimPrefix(h, "Bearer "); !strings.Contains(b, ".") {
					rawTok = b
				}
			}
		}

		if rawTok == "" {
			next.ServeHTTP(w, r)
			return
		}

		isOpaque := store != nil && !strings.Contains(rawTok, ".")
		if isOpaque {
			if st, ok := store.ValidateAny(rawTok); ok {
				if r.Header.Get("Authorization") == "" {
					r.Header.Set("Authorization", "Bearer "+rawTok)
				}
				if !fromQuery {
					r = r.WithContext(context.WithValue(r.Context(), rawTokenCtxKey, rawTok))
				}
				r = r.WithContext(context.WithValue(r.Context(), streamTokenCtxKey, st))
				// Mint/slide the httpOnly cookie so a native player re-authenticates
				// on every segment without the token ever touching the URL.
				if isHLSPath(r.URL.Path) {
					setHLSCookie(w, r.URL.Path, rawTok, st.TTL, httpsOnly())
				}
			} else if fromQuery {
				// opaque token invalid/expired: don't set as Bearer, handler returns 401.
				// Logged so a Chromecast disconnect caused by a vanished token (server
				// restart wiped the in-memory store; token actually expired) is visible
				// instead of being silently spanned away.
				slog.WarnContext(r.Context(), "stream token: opaque token validation failed",
					slog.String("path", r.URL.Path),
					slog.String("user_agent", r.UserAgent()),
					slog.Int("token_len", len(rawTok)))
			}
		} else if fromQuery && r.Header.Get("Authorization") == "" {
			// JWT delivered via ?token= (legacy images, music, photos): promoted
			// to a Bearer header for READS only, and never on an admin route - a
			// session token that leaks from a proxy log or a browser history
			// must not be able to write anything, or reach the admin surface.
			// The token has already left the URL either way.
			if jwtQueryAllowed(r) {
				r.Header.Set("Authorization", "Bearer "+rawTok)
			} else {
				metrics.AuthzDeniedTotal.WithLabelValues("http", "jwt_in_query_refused").Inc()
				slog.WarnContext(r.Context(), "account token in the URL refused on this route",
					slog.String("method", r.Method), slog.String("path", r.URL.Path))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// jwtQueryAllowed is where an account JWT may still arrive as ?token=: reads
// outside /api/v1/admin/.
func jwtQueryAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return !strings.HasPrefix(r.URL.Path, "/api/v1/admin/")
}

type ctxKeyStreamToken struct{}
type ctxKeyRawToken struct{}
type ctxKeyTokenFromQuery struct{}

var streamTokenCtxKey = ctxKeyStreamToken{}
var rawTokenCtxKey = ctxKeyRawToken{}
var tokenFromQueryCtxKey = ctxKeyTokenFromQuery{}

// TokenFromQuery reports whether the request authenticated via a ?token= query
// param (web-legacy / cast) rather than the evey_hls cookie or a Bearer header.
// On-demand manifest handlers use it to decide whether to bake the token back
// into sub-resource URIs so a cookie-less Chromecast can authenticate them.
func TokenFromQuery(ctx context.Context) bool {
	v, _ := ctx.Value(tokenFromQueryCtxKey).(bool)
	return v
}

// StreamTokenFromContext returns the resolved stream token if the request was authenticated via opaque token.
func StreamTokenFromContext(ctx context.Context) (*shared.StreamToken, bool) {
	st, ok := ctx.Value(streamTokenCtxKey).(*shared.StreamToken)
	return st, ok
}

// RawTokenFromContext returns the original query-param token before the middleware stripped it.
func RawTokenFromContext(ctx context.Context) string {
	s, _ := ctx.Value(rawTokenCtxKey).(string)
	return s
}

// SecurityHeadersMiddleware sets security-related response headers.
// httpsOnly enables HSTS when the server is behind an HTTPS reverse proxy.
func SecurityHeadersMiddleware(httpsOnly bool, next http.Handler) http.Handler {
	return securityHeadersMiddleware(httpsOnly, next)
}

// SecurityHeadersMiddlewareLive reads the HTTPS flag (HSTS) on every request.
func SecurityHeadersMiddlewareLive(httpsOnly func() bool, next http.Handler) http.Handler {
	return securityHeadersMiddlewareFn(httpsOnly, next)
}

func securityHeadersMiddleware(httpsOnly bool, next http.Handler) http.Handler {
	return securityHeadersMiddlewareFn(fixedBool(httpsOnly), next)
}

func securityHeadersMiddlewareFn(httpsOnly func() bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// 'wasm-unsafe-eval' lets the player compile the ac3go WebAssembly decoder
		// (the browser AC-3/E-AC-3 path) - the narrow token that permits Wasm
		// compilation ONLY, without opening general eval() the way 'unsafe-eval'
		// would (Chrome 97+/Firefox 102+).
		// https://www.gstatic.com is the Google Cast sender SDK (cast_sender.js +
		// cast_framework.js): the ONLY way a browser can discover Chromecasts on
		// the USER's own LAN (JS can't do mDNS). Dropping it silently kills the
		// cast button for every user.
		// No 'unsafe-inline' in script-src: an injected inline script does not
		// run. Plugin UIs and theme scripts used to need it (a srcdoc iframe
		// inherits this policy); they now run in /plugin-frame.html, a page
		// with its own policy (plugin_frame_handler.go), framed from 'self'.
		// media-src and worker-src must both allow blob:. hls.js attaches the
		// MediaSource through a blob: URL and demuxes in a blob: worker, and
		// both directives fall back to default-src 'self' when absent - which
		// does not match blob:, so playback dies. This went unnoticed because
		// the shipped Caddy strips this header and curates a complete policy of
		// its own; deployments behind Nginx Proxy Manager or Traefik pass the
		// app header straight through, exactly as docs/reverse-proxy.md tells
		// them to, and got a silently broken player. Keep this policy correct
		// standalone - the proxy is not there to repair it.
		// frame-src is the Cast SDK's own iframe, script-src alone is not enough.
		//
		// /cast-receiver.html is the page LOADED BY THE CHROMECAST (custom Web
		// Receiver declared in the Google Cast Console): the CAF player injects
		// styles, fonts and images from gstatic on top of the SDK - the app's
		// policy would silently block them on the TV (broken backdrop, zero
		// log). This variant widens ONLY this page; the app keeps the strict
		// policy.
		if r.URL.Path == "/cast-receiver.html" {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline'; img-src 'self' data: https: blob:; media-src 'self' blob:; worker-src 'self' blob:; font-src 'self' data:; connect-src 'self' https: wss:; frame-src 'self' https://www.gstatic.com; base-uri 'self'; object-src 'none'")
		}
		if httpsOnly() || r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func apiCacheMiddleware(next http.Handler) http.Handler {
	type rule struct {
		path     string
		ccHeader string
	}
	rules := []rule{
		{"/api/v1/admin/system", "private, max-age=60"},
		{"/api/v1/admin/dashboard", "private, max-age=10"},
		{"/api/v1/libraries", "private, max-age=300"},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method == http.MethodGet {
			for _, rule := range rules {
				if r.URL.Path == rule.path {
					w.Header().Set("Cache-Control", rule.ccHeader)
					break
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// discardResponseWriter swallows body writes; used to run a handler for side-effects only.
type discardResponseWriter struct {
	header http.Header
	status int
}

func (d *discardResponseWriter) Header() http.Header         { return d.header }
func (d *discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponseWriter) WriteHeader(code int)        { d.status = code }
