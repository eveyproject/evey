package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"

	domainuser "github.com/evey-project/evey/internal/domain/user"
)

type httpClaimsCtxKey struct{}

// HTTPClaimsFromContext returns the claims injected by httpAuthMiddleware.
func HTTPClaimsFromContext(ctx context.Context) *domainuser.Claims {
	c, _ := ctx.Value(httpClaimsCtxKey{}).(*domainuser.Claims)
	return c
}

// httpAuthMiddleware validates JWT bearer tokens and injects claims into context.
//
// It deliberately does NOT accept opaque stream tokens: a stream token is a
// narrowly-scoped capability (one media item, short-lived) that travels in URLs
// (history, proxy logs, referer, Chromecast handoff), so it must not authenticate
// generic endpoints. Routes where a stream token IS a legitimate credential (the
// raw-URL media streaming routes the player can't attach a Bearer header to) use
// httpStreamAuthMiddleware instead.
func httpAuthMiddleware(verifier TokenVerifier) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			claims, err := verifier.ValidateAccessToken(token)
			if err != nil {
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), httpClaimsCtxKey{}, claims)
			next(w, r.WithContext(ctx))
		}
	}
}

// httpStreamAuthMiddleware authenticates the media streaming routes the player
// loads as raw URLs (<video src>, hls.js XHR, download anchor) where a Bearer
// header cannot be attached. It accepts EITHER an opaque stream token (resolved
// by TokenRedactMiddlewareWithStore into the request context) OR a JWT. The
// synthesized stream-token claims carry only UserID (never IsAdmin), and each
// handler additionally enforces st.MediaItemID == {id}, so the token stays
// scoped to its one media item. Apply ONLY to /api/v1/media/{id}/* play routes,
// never to generic user/admin endpoints (those use httpAuthMiddleware).
func httpStreamAuthMiddleware(verifier TokenVerifier) func(http.HandlerFunc) http.HandlerFunc {
	authMw := httpAuthMiddleware(verifier)
	return func(next http.HandlerFunc) http.HandlerFunc {
		jwtPath := authMw(next)
		return func(w http.ResponseWriter, r *http.Request) {
			if st, ok := StreamTokenFromContext(r.Context()); ok {
				claims := &domainuser.Claims{UserID: string(st.UserID)}
				ctx := context.WithValue(r.Context(), httpClaimsCtxKey{}, claims)
				next(w, r.WithContext(ctx))
				return
			}
			jwtPath(w, r)
		}
	}
}

// httpAdminMiddleware validates JWT bearer token AND checks IsAdmin.
func httpAdminMiddleware(verifier TokenVerifier) func(http.HandlerFunc) http.HandlerFunc {
	authMw := httpAuthMiddleware(verifier)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return authMw(func(w http.ResponseWriter, r *http.Request) {
			claims := HTTPClaimsFromContext(r.Context())
			if claims == nil || !claims.IsAdmin {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			next(w, r)
		})
	}
}

// httpScrapeOrAdminMiddleware accepts either a static scrape bearer
// (constant-time compared against scrapeToken) OR a valid admin JWT.
// Scoped to GET /metrics so Prometheus/Grafana can scrape without
// renewing a ~15-min JWT every interval. Empty scrapeToken disables
// the static path; only the admin fallback then applies.
func httpScrapeOrAdminMiddleware(verifier TokenVerifier, scrapeToken string) func(http.HandlerFunc) http.HandlerFunc {
	adminMw := httpAdminMiddleware(verifier)
	return func(next http.HandlerFunc) http.HandlerFunc {
		adminPath := adminMw(next)
		return func(w http.ResponseWriter, r *http.Request) {
			if scrapeToken != "" {
				if tok := extractBearerToken(r); tok != "" &&
					subtle.ConstantTimeCompare([]byte(tok), []byte(scrapeToken)) == 1 {
					next(w, r)
					return
				}
			}
			adminPath(w, r)
		}
	}
}
