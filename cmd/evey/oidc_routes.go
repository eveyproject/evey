package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	appuser "github.com/gravity-zero/evey/internal/app/user"
)

func registerOIDCRoutes(mux *http.ServeMux, oidcSvc *appuser.OIDCService) {
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

	// GET /api/v1/auth/oidc/callback — exchange code, issue JWT, redirect to frontend
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
			redirectURL := "/login?oidc_error=1"
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		redirectURL := "/oidc-callback?" + url.Values{
			"token":   {accessToken},
			"refresh": {refreshToken},
		}.Encode()
		http.Redirect(w, r, redirectURL, http.StatusFound)
	})
}
