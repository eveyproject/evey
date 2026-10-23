package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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
				// Routine on expired sessions (a tab waking up 401s once
				// before the refresh) - counted for the ops dashboard, logged
				// at debug only so default logs stay quiet.
				metrics.AuthFailuresTotal.WithLabelValues("missing_bearer").Inc()
				slog.Debug("auth rejected", slog.String("reason", "missing_bearer"),
					slog.String("method", r.Method), slog.String("path", r.URL.Path),
					slog.String("client_ip", ratelimit.ClientIP(r)))
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			claims, err := verifier.ValidateAccessToken(token)
			if err != nil {
				// A credential was PRESENTED and rejected - the bucket a
				// stuffing run with stolen bearers lands in, mirroring the
				// per-attempt login log in user/command_service.go. Never log
				// the token itself.
				// A token that cannot be CHECKED is not a token that is wrong.
				// When the epoch lookup fails (the DB is slow, locked or gone),
				// every authenticated request in the server starts failing with
				// this same line - and an operator reading "invalid_token" goes
				// hunting for a rotated JWT key that is fine. Separate label,
				// separate wording, cause attached.
				reason := "invalid_token"
				if errors.Is(err, appuser.ErrEpochUnavailable) {
					reason = "epoch_lookup_failed"
				}
				metrics.AuthFailuresTotal.WithLabelValues(reason).Inc()
				// The client address is what makes this bucket usable: without
				// it a stuffing run is a pile of identical lines with nothing to
				// group or block on.
				attrs := []any{
					slog.String("reason", reason),
					slog.String("method", r.Method), slog.String("path", r.URL.Path),
					slog.String("client_ip", ratelimit.ClientIP(r)),
				}
				if reason == "epoch_lookup_failed" {
					// Only on the abnormal branch: a stuffing run must not be
					// able to write an attacker-chosen error into the log.
					attrs = append(attrs, slog.Any("cause", err))
					slog.ErrorContext(r.Context(), "auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key", attrs...)
				} else {
					slog.WarnContext(r.Context(), "auth rejected", attrs...)
				}
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			w, r, refused := gateProfile(w, r, claims)
			if refused {
				return
			}
			ctx := context.WithValue(activity.WithImpersonator(r.Context(), claims.ImpersonatedBy), httpClaimsCtxKey{}, claims)
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
				if st.Scope == shared.ScopeBackupDownload {
					// Bound to a backup id, never to a media route.
					metrics.AuthzDeniedTotal.WithLabelValues("http", "backup_token_off_route").Inc()
					http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
					return
				}
				claims := &domainuser.Claims{UserID: string(st.UserID), ProfileID: string(st.ProfileID)}
				gw, gr, refused := gateProfile(w, r, claims)
				if refused {
					return
				}
				ctx := context.WithValue(gr.Context(), httpClaimsCtxKey{}, claims)
				next(gw, gr.WithContext(ctx))
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
				// Same reasoning as the gRPC interceptor: the invalid-token path
				// a few lines up logs and counts, this one said nothing at all.
				metrics.AuthzDeniedTotal.WithLabelValues("http", "admin_required").Inc()
				slog.WarnContext(r.Context(), "admin access denied",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("client_ip", ratelimit.ClientIP(r)))
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
					next(w, metrics.WithScrapeAuth(r))
					return
				}
			}
			adminPath(w, r)
		}
	}
}
