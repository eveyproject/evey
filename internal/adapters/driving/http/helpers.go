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

	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
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
		return "", fmt.Errorf("path traversal detected")
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
	if strings.TrimRight(origin, "/") == allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

func serveFileWithCache(w http.ResponseWriter, r *http.Request, filePath string) {
	fi, err := os.Stat(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	etag := fmt.Sprintf(`"%x-%x"`, fi.ModTime().Unix(), fi.Size())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeFile(w, r, filePath)
}

func checkLibraryAccess(ctx context.Context, w http.ResponseWriter, claims *domainuser.Claims, libraryID string, checker LibraryAccessChecker) bool {
	if claims.IsAdmin {
		return true
	}
	if checker == nil {
		slog.Warn("library access checker is nil, denying access", slog.String("user_id", claims.UserID), slog.String("library_id", libraryID))
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return false
	}
	ok, err := checker.HasAccess(ctx, claims.UserID, libraryID)
	if err != nil || !ok {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return false
	}
	return true
}

// TokenRedactMiddleware moves ?token= from URL query to Authorization header.
// For opaque stream tokens (no "." = not a JWT), it resolves the token via the store
// and injects synthetic claims into the context so handlers work unchanged.
func TokenRedactMiddleware(next http.Handler) http.Handler { return tokenRedactMiddleware(nil, next) }

// TokenRedactMiddlewareWithStore creates a token middleware that also resolves opaque stream tokens.
func TokenRedactMiddlewareWithStore(store streamTokenValidator, next http.Handler) http.Handler {
	return tokenRedactMiddleware(store, next)
}

type streamTokenValidator interface {
	ValidateAny(token string) (*shared.StreamToken, bool)
}

func tokenRedactMiddleware(store streamTokenValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("token")
		if t != "" {
			q := r.URL.Query()
			q.Del("token")
			r.URL.RawQuery = q.Encode()

			if r.Header.Get("Authorization") == "" {
				isOpaque := store != nil && !strings.Contains(t, ".")
				if isOpaque {
					if st, ok := store.ValidateAny(t); ok {
						r.Header.Set("Authorization", "Bearer "+t)
						ctx := context.WithValue(r.Context(), streamTokenCtxKey, st)
						r = r.WithContext(ctx)
					}
					// opaque token invalid/expired: don't set as Bearer, handler returns 401
				} else {
					// JWT or no store: set as Bearer, handlers validate
					r.Header.Set("Authorization", "Bearer "+t)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKeyStreamToken struct{}

var streamTokenCtxKey = ctxKeyStreamToken{}

// StreamTokenFromContext returns the resolved stream token if the request was authenticated via opaque token.
func StreamTokenFromContext(ctx context.Context) (*shared.StreamToken, bool) {
	st, ok := ctx.Value(streamTokenCtxKey).(*shared.StreamToken)
	return st, ok
}

// SecurityHeadersMiddleware sets security-related response headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler { return securityHeadersMiddleware(next) }

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self' wss:")
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

// extractClaims extracts a Bearer token and validates it, returning claims.
func extractClaims(r *http.Request, verifier TokenVerifier) (*domainuser.Claims, bool) {
	token := extractBearerToken(r)
	if token == "" {
		return nil, false
	}
	claims, err := verifier.ValidateAccessToken(token)
	if err != nil {
		return nil, false
	}
	return claims, true
}

var errUnauthenticated = fmt.Errorf("unauthenticated")
var errForbidden = fmt.Errorf("forbidden")

// extractAdminClaims extracts a Bearer token (header or query param) and validates admin access.
func extractAdminClaims(r *http.Request, verifier TokenVerifier) (*domainuser.Claims, error) {
	token := extractBearerToken(r)
	if token == "" {
		return nil, errUnauthenticated
	}
	claims, err := verifier.ValidateAccessToken(token)
	if err != nil {
		return nil, errUnauthenticated
	}
	if !claims.IsAdmin {
		return nil, errForbidden
	}
	return claims, nil
}

type adminCtxKeyType struct{}

var adminClaimsKey = adminCtxKeyType{}

func requireAdmin(tokenGen TokenVerifier, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := extractAdminClaims(r, tokenGen)
		if err != nil {
			if errors.Is(err, errForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			}
			return
		}
		ctx := context.WithValue(r.Context(), adminClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// discardResponseWriter swallows body writes; used to run a handler for side-effects only.
type discardResponseWriter struct {
	header http.Header
	status int
}

func (d *discardResponseWriter) Header() http.Header        { return d.header }
func (d *discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponseWriter) WriteHeader(code int)        { d.status = code }
