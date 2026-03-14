package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/notification"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerPushRoutes(mux *http.ServeMux, tokens notification.DeviceTokenRepository, tokenGen *auth.JWTGenerator) {
	extractClaims := func(r *http.Request) (*domainuser.Claims, bool) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return nil, false
		}
		claims, err := tokenGen.ValidateAccessToken(h[7:])
		if err != nil {
			return nil, false
		}
		return claims, true
	}

	mux.HandleFunc("POST /api/v1/push/register", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Token    string `json:"token"`
			Platform string `json:"platform"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
			http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
			return
		}
		platform := body.Platform
		if platform == "" {
			platform = "web"
		}
		if err := tokens.Upsert(r.Context(), claims.UserID, claims.ProfileID, body.Token, platform); err != nil {
			slog.Warn("push register: upsert failed", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("DELETE /api/v1/push/unregister", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
			http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if err := tokens.Delete(r.Context(), claims.UserID, body.Token); err != nil {
			slog.Warn("push unregister: delete failed", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
}
