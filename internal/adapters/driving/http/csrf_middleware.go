package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
)

const (
	csrfCookieName = "_csrf"
	csrfHeaderName = "X-CSRF-Token"
	csrfTokenBytes = 16
)

func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// csrfMiddleware implements double-submit cookie pattern.
// On every response it sets a _csrf cookie (HttpOnly=false so JS can read it).
// On state-changing methods (POST/PUT/PATCH/DELETE) it verifies
// the X-CSRF-Token header matches the cookie value.
func csrfMiddlewareFn(httpsOnly func() bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			// CSRF is a browser-only threat: it relies on the browser
			// auto-submitting cookies on cross-origin requests. A client that
			// did NOT send our _csrf cookie cannot be tricked into doing so,
			// so it cannot be a CSRF victim. This catches every native client
			// (React Native, Expo, CLI, curl, server-to-server) plus a fresh
			// browser session that hasn't yet seen its first response.
			//
			// React Native's fetch sometimes sets an `Origin` header that
			// passes the older Sec-Fetch-Site/Origin sniff but never carries
			// the cookie jar - relying on those headers alone would 403 the
			// mobile /auth/refresh and bounce the user to /login on every
			// access-token rotation. The cookie test is the load-bearing
			// signal here; the Sec-Fetch sniff is kept as a redundant guard
			// for environments that strip cookies but still look browser-y.
			if c, err := r.Cookie(csrfCookieName); err != nil || c.Value == "" {
				break
			}
			if r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Origin") == "" {
				break
			}

			header := r.Header.Get(csrfHeaderName)
			if header == "" {
				slog.WarnContext(r.Context(), "csrf header missing on session request", slog.String("path", r.URL.Path))
				http.Error(w, `{"message":"csrf token missing"}`, http.StatusForbidden)
				return
			}

			cookie, err := r.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				http.Error(w, `{"message":"csrf token missing"}`, http.StatusForbidden)
				return
			}
			if subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
				slog.WarnContext(r.Context(), "csrf token mismatch", slog.String("path", r.URL.Path))
				http.Error(w, `{"message":"csrf token invalid"}`, http.StatusForbidden)
				return
			}
		}

		// Ensure a CSRF cookie is always present
		if _, err := r.Cookie(csrfCookieName); err != nil {
			token, genErr := generateCSRFToken()
			if genErr != nil {
				slog.ErrorContext(r.Context(), "csrf token generation failed", slog.Any("err", genErr))
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // JS must read this
				Secure:   httpsOnly(),
				SameSite: http.SameSiteLaxMode,
				MaxAge:   86400, // 24h
			})
		}

		next.ServeHTTP(w, r)
	})
}

func csrfMiddleware(httpsOnly bool, next http.Handler) http.Handler {
	return csrfMiddlewareFn(fixedBool(httpsOnly), next)
}

// CSRFMiddleware is the exported wrapper for use in main wiring.
func CSRFMiddleware(httpsOnly bool, next http.Handler) http.Handler {
	return csrfMiddleware(httpsOnly, next)
}

// CSRFMiddlewareLive reads the HTTPS flag on every request (Admin -> Remote
// Access can change it while the server runs).
func CSRFMiddlewareLive(httpsOnly func() bool, next http.Handler) http.Handler {
	return csrfMiddlewareFn(httpsOnly, next)
}
