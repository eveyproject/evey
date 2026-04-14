package httpapi

import (
	"context"
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
// For opaque stream tokens (already resolved by TokenRedactMiddleware), it creates
// limited claims from the stream token.
func httpAuthMiddleware(verifier TokenVerifier) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if st, ok := StreamTokenFromContext(r.Context()); ok {
				claims := &domainuser.Claims{UserID: string(st.UserID)}
				ctx := context.WithValue(r.Context(), httpClaimsCtxKey{}, claims)
				next(w, r.WithContext(ctx))
				return
			}
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
