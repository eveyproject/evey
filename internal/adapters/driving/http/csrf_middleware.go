package httpapi

import (
	"crypto/rand"
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
func csrfMiddleware(httpsOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			header := r.Header.Get(csrfHeaderName)

			// Browser session is identified by the refresh-token cookie. If present,
			// CSRF enforcement is mandatory; if absent, the caller is Bearer-only
			// (mobile/API) and immune to CSRF.
			if _, err := r.Cookie(refreshCookieName); err != nil {
				break
			}

			if header == "" {
				slog.Warn("csrf header missing on session request", slog.String("path", r.URL.Path))
				http.Error(w, `{"message":"csrf token missing"}`, http.StatusForbidden)
				return
			}

			cookie, err := r.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				http.Error(w, `{"message":"csrf token missing"}`, http.StatusForbidden)
				return
			}
			if header != cookie.Value {
				slog.Warn("csrf token mismatch", slog.String("path", r.URL.Path))
				http.Error(w, `{"message":"csrf token invalid"}`, http.StatusForbidden)
				return
			}
		}

		// Ensure a CSRF cookie is always present
		if _, err := r.Cookie(csrfCookieName); err != nil {
			token, genErr := generateCSRFToken()
			if genErr != nil {
				slog.Error("csrf token generation failed", slog.Any("err", genErr))
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // JS must read this
				Secure:   httpsOnly,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   86400, // 24h
			})
		}

		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware is the exported wrapper for use in main wiring.
func CSRFMiddleware(httpsOnly bool, next http.Handler) http.Handler {
	return csrfMiddleware(httpsOnly, next)
}
