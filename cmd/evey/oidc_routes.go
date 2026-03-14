package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	appuser "github.com/gravity-zero/evey/internal/app/user"
)

const (
	oidcSessionCookieName = "evey_oidc_session"
	oidcSessionCookieAge  = 5 * 60 // 5 minutes — one-time exchange
)

func registerOIDCRoutes(mux *http.ServeMux, oidcSvc *appuser.OIDCService, httpsOnly bool) {
	// GET /api/v1/auth/oidc/config — frontend polls this to show/hide SSO button
	mux.HandleFunc("GET /api/v1/auth/oidc/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"enabled": true}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	// GET /api/v1/auth/oidc/login — redirect to provider
	mux.HandleFunc("GET /api/v1/auth/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		var nonceBuf [16]byte
		if _, err := rand.Read(nonceBuf[:]); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		nonce := hex.EncodeToString(nonceBuf[:])
		authURL := oidcSvc.AuthURL(nonce)
		http.Redirect(w, r, authURL, http.StatusFound)
	})

	// GET /api/v1/auth/oidc/callback — exchange code, set HttpOnly cookies, redirect without tokens in URL
	mux.HandleFunc("GET /api/v1/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := q.Get("code")
		state := q.Get("state")

		if code == "" || state == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if _, err := oidcSvc.ValidateState(state); err != nil {
			slog.Warn("oidc callback: invalid state", slog.Any("err", err))
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}

		accessToken, refreshToken, err := oidcSvc.HandleCallback(r.Context(), code)
		if err != nil {
			if errors.Is(err, appuser.ErrEmailDomainNotAllowed) {
				http.Error(w, "access denied: email domain not allowed", http.StatusForbidden)
				return
			}
			slog.Error("oidc callback: handle failed", slog.Any("err", err))
			http.Redirect(w, r, "/login?oidc_error=1", http.StatusFound)
			return
		}

		// Access token: short-lived HttpOnly session cookie (exchanged once by /oidc/session).
		http.SetCookie(w, &http.Cookie{
			Name:     oidcSessionCookieName,
			Value:    accessToken,
			Path:     "/api/v1/auth/oidc/session",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   oidcSessionCookieAge,
		})

		// Refresh token: long-lived HttpOnly cookie (same as regular login).
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    refreshToken,
			Path:     "/api/v1/auth/",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   refreshCookieMaxAge,
		})

		// Redirect to frontend with no tokens in URL — browser history stays clean.
		http.Redirect(w, r, "/oidc-callback", http.StatusFound)
	})

	// GET /api/v1/auth/oidc/session — one-time endpoint: reads session cookie, returns access token, clears cookie.
	mux.HandleFunc("GET /api/v1/auth/oidc/session", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(oidcSessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"message":"no session"}`, http.StatusUnauthorized)
			return
		}
		// Clear the session cookie immediately — single use.
		http.SetCookie(w, &http.Cookie{
			Name:     oidcSessionCookieName,
			Value:    "",
			Path:     "/api/v1/auth/oidc/session",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"accessToken": cookie.Value})
	})
}
