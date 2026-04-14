package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	appuser "github.com/gravity-zero/evey/internal/app/user"

	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

const (
	oidcSessionCookieName = "evey_oidc_session"
	oidcSessionCookieAge  = 5 * 60 // 5 minutes — one-time exchange

	oidcIDTokenCookieName = "evey_oidc_id_token"

	refreshCookieName   = "evey_refresh"
	refreshCookieMaxAge = 30 * 24 * 3600 // 30 days
)

func registerOIDCRoutes(mux *http.ServeMux, oidcSvc *appuser.OIDCService, httpsOnly bool, oidcLoginRL ...*ratelimit.Limiter) {
	mux.HandleFunc("GET /api/v1/auth/oidc/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"enabled": true}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	oidcLoginHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var nonceBuf [16]byte
		if _, err := rand.Read(nonceBuf[:]); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		nonce := hex.EncodeToString(nonceBuf[:])
		authURL := oidcSvc.AuthURL(nonce)
		http.Redirect(w, r, authURL, http.StatusFound)
	})
	if len(oidcLoginRL) > 0 && oidcLoginRL[0] != nil {
		mux.HandleFunc("GET /api/v1/auth/oidc/login", ratelimit.MiddlewareFunc(oidcLoginRL[0], oidcLoginHandler))
	} else {
		mux.HandleFunc("GET /api/v1/auth/oidc/login", oidcLoginHandler)
	}

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

		accessToken, refreshToken, rawIDToken, err := oidcSvc.HandleCallback(r.Context(), code)
		if err != nil {
			if errors.Is(err, appuser.ErrEmailDomainNotAllowed) {
				http.Error(w, "access denied: email domain not allowed", http.StatusForbidden)
				return
			}
			slog.Error("oidc callback: handle failed", slog.Any("err", err))
			http.Redirect(w, r, "/login?oidc_error=1", http.StatusFound)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     oidcSessionCookieName,
			Value:    accessToken,
			Path:     "/api/v1/auth/oidc/session",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   oidcSessionCookieAge,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    refreshToken,
			Path:     "/api/v1/auth/",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   refreshCookieMaxAge,
		})

		// store raw OIDC id_token for RP-Initiated Logout (id_token_hint)
		if rawIDToken != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     oidcIDTokenCookieName,
				Value:    rawIDToken,
				Path:     "/api/v1/auth/",
				HttpOnly: true,
				Secure:   httpsOnly,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   refreshCookieMaxAge,
			})
		}

		http.Redirect(w, r, "/oidc-callback", http.StatusFound)
	})

	mux.HandleFunc("GET /api/v1/auth/oidc/session", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(oidcSessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"message":"no session"}`, http.StatusUnauthorized)
			return
		}
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
		if err := json.NewEncoder(w).Encode(map[string]any{"accessToken": cookie.Value}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}

type parentalHandler struct {
	profileRepo domainuser.ProfileRepository
	hasher      domainuser.PasswordHasher
	pinRL       *ratelimit.Limiter
}

func (h *parentalHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")
	if profileID != claims.ProfileID && !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}
	type resp struct {
		MaxRating   string   `json:"maxRating"`
		BlockedTags []string `json:"blockedTags"`
		HasPIN      bool     `json:"hasPin"`
		Enabled     bool     `json:"enabled"`
	}
	out := resp{BlockedTags: []string{}}
	if profile.Parental != nil {
		out.MaxRating = profile.Parental.MaxRating
		if profile.Parental.BlockedTags != nil {
			out.BlockedTags = profile.Parental.BlockedTags
		}
		out.HasPIN = profile.Parental.PINHash != ""
		out.Enabled = profile.Parental.Enabled
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *parentalHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")
	if profileID != claims.ProfileID && !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body struct {
		MaxRating   *string  `json:"maxRating"`
		BlockedTags []string `json:"blockedTags"`
		PIN         *string  `json:"pin"`
		CurrentPIN  string   `json:"currentPin"`
		Enabled     *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}

	if profile.Parental != nil && profile.Parental.PINHash != "" {
		if !h.pinRL.Allow(profileID) {
			http.Error(w, `{"message":"too many attempts"}`, http.StatusTooManyRequests)
			return
		}
		valid, err := h.hasher.Verify(body.CurrentPIN, profile.Parental.PINHash)
		if err != nil || !valid {
			http.Error(w, `{"message":"invalid current PIN"}`, http.StatusForbidden)
			return
		}
	}

	if profile.Parental == nil {
		profile.Parental = &domainuser.ParentalSettings{}
	}
	if body.MaxRating != nil {
		profile.Parental.MaxRating = *body.MaxRating
	}
	if body.BlockedTags != nil {
		profile.Parental.BlockedTags = body.BlockedTags
	}
	if body.PIN != nil {
		if *body.PIN == "" {
			profile.Parental.PINHash = ""
		} else {
			hash, err := h.hasher.Hash(*body.PIN)
			if err != nil {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
			profile.Parental.PINHash = hash
		}
	}
	if body.Enabled != nil {
		profile.Parental.Enabled = *body.Enabled
	}

	if err := h.profileRepo.Update(r.Context(), profile); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.Warn("write failed", slog.Any("err", err))
	}
}

func (h *parentalHandler) verifyPIN(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")
	if !h.pinRL.Allow(profileID) {
		http.Error(w, `{"message":"too many attempts"}`, http.StatusTooManyRequests)
		return
	}
	var body struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && string(profile.UserID) != claims.UserID {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	if profile.Parental == nil || profile.Parental.PINHash == "" {
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]any{"valid": true}); encErr != nil {
			slog.Warn("json encode failed", slog.Any("err", encErr))
		}
		return
	}
	valid, err := h.hasher.Verify(body.PIN, profile.Parental.PINHash)
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"valid": valid}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func registerParentalRoutes(
	mux *http.ServeMux,
	profileRepo domainuser.ProfileRepository,
	hasher domainuser.PasswordHasher,
	tokenGen *auth.JWTGenerator,
) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &parentalHandler{
		profileRepo: profileRepo,
		hasher:      hasher,
		pinRL:       ratelimit.NewWithBurst(3, 0.6),
	}
	mux.HandleFunc("GET /api/v1/profiles/{id}/parental", authMw(h.getSettings))
	mux.HandleFunc("PUT /api/v1/profiles/{id}/parental", authMw(h.updateSettings))
	mux.HandleFunc("POST /api/v1/profiles/{id}/parental/verify-pin", authMw(h.verifyPIN))
}
