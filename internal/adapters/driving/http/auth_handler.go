package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/codes"

	appuser "github.com/evey-project/evey/internal/app/user"

	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
		_, span := tracing.StartSpan(r.Context(), "http.auth.oidc.GetConfig")
		defer span.End()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"enabled": true}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	oidcLoginHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.auth.oidc.Login")
		defer span.End()
		var nonceBuf [16]byte
		if _, err := rand.Read(nonceBuf[:]); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "rand read failed")
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
		ctx, span := tracing.StartSpan(r.Context(), "http.auth.oidc.Callback")
		defer span.End()
		r = r.WithContext(ctx)
		q := r.URL.Query()
		code := q.Get("code")
		state := q.Get("state")

		if code == "" || state == "" {
			span.SetStatus(codes.Error, "missing code or state")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		nonce, err := oidcSvc.ValidateState(state)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid state")
			slog.Warn("oidc callback: invalid state", slog.Any("err", err))
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}

		accessToken, refreshToken, rawIDToken, err := oidcSvc.HandleCallback(r.Context(), code, nonce)
		if err != nil {
			span.RecordError(err)
			if errors.Is(err, appuser.ErrEmailDomainNotAllowed) {
				span.SetStatus(codes.Error, "email domain not allowed")
				http.Error(w, "access denied: email domain not allowed", http.StatusForbidden)
				return
			}
			span.SetStatus(codes.Error, "handle callback failed")
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
		_, span := tracing.StartSpan(r.Context(), "http.auth.oidc.Session")
		defer span.End()
		cookie, err := r.Cookie(oidcSessionCookieName)
		if err != nil || cookie.Value == "" {
			span.SetStatus(codes.Error, "no session cookie")
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
	userRepo    domainuser.UserRepository
	hasher      domainuser.PasswordHasher
	pinRL       *ratelimit.Limiter
}

func (h *parentalHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.parental.GetSettings")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "profile not found")
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && string(profile.UserID) != claims.UserID {
		span.SetStatus(codes.Error, "forbidden")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	type resp struct {
		MaxAge      *int     `json:"maxAge"`
		BlockedTags []string `json:"blockedTags"`
		HasPIN      bool     `json:"hasPin"`
		Enabled     bool     `json:"enabled"`
	}
	out := resp{BlockedTags: []string{}}
	if profile.Parental != nil {
		out.MaxAge = profile.Parental.MaxAge
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
	ctx, span := tracing.StartSpan(r.Context(), "http.parental.UpdateSettings")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")
	// Accept both `pin` (legacy, kept for existing tests) and `newPin`
	// (the field name the proto/orval-generated client actually sends).
	// The handler is not wired through the gRPC gateway, so without this
	// accommodation the frontend call to save a PIN was a silent no-op.
	var body struct {
		MaxAge          *int     `json:"maxAge"`
		UnsetMaxAge     bool     `json:"unsetMaxAge"`
		BlockedTags     []string `json:"blockedTags"`
		PIN             *string  `json:"pin"`
		NewPIN          *string  `json:"newPin"`
		CurrentPIN      string   `json:"currentPin"`
		CurrentPassword string   `json:"currentPassword"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "bad request body")
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	// Unify the two field names — prefer `pin` when both are sent.
	if body.PIN == nil {
		body.PIN = body.NewPIN
	}
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "profile not found")
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && string(profile.UserID) != claims.UserID {
		span.SetStatus(codes.Error, "forbidden")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	if body.PIN != nil && *body.PIN != "" {
		if !isFourDigit(*body.PIN) {
			span.SetStatus(codes.Error, "PIN not 4 digits")
			http.Error(w, `{"message":"PIN must be 4 digits"}`, http.StatusBadRequest)
			return
		}
	}

	noExistingPIN := profile.Parental == nil || profile.Parental.PINHash == ""
	settingNewPIN := body.PIN != nil && *body.PIN != ""
	mutatingPIN := body.PIN != nil // rotation, clear, or first-time set
	disablingProtection := body.Enabled != nil && !*body.Enabled

	switch {
	case (mutatingPIN || disablingProtection) && !noExistingPIN:
		// Rotating / clearing an existing PIN, or disabling parental controls
		// while a PIN is set, requires the current PIN. Other updates (max age,
		// blocked tags, enabling) are not gated by the PIN — the user already
		// authenticated to reach this endpoint.
		if !h.pinRL.Allow(profileID) {
			span.SetStatus(codes.Error, "rate limited")
			http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
			return
		}
		valid, err := h.hasher.Verify(body.CurrentPIN, profile.Parental.PINHash)
		if err != nil || !valid {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(codes.Error, "invalid current PIN")
			http.Error(w, `{"message":"invalid current PIN"}`, http.StatusForbidden)
			return
		}
	case settingNewPIN && noExistingPIN && h.userRepo != nil:
		// First-time PIN set: require the account password. Without this a
		// stolen JWT could silently lock the owner out by enabling parental
		// + setting a PIN the attacker picks.
		if !h.pinRL.Allow(profileID) {
			span.SetStatus(codes.Error, "rate limited")
			http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
			return
		}
		u, uerr := h.userRepo.GetByID(r.Context(), profile.UserID)
		if uerr != nil || u == nil {
			if uerr != nil {
				span.RecordError(uerr)
			}
			span.SetStatus(codes.Error, "user lookup failed")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		valid, verr := h.hasher.Verify(body.CurrentPassword, u.PasswordHash)
		if verr != nil || !valid {
			if verr != nil {
				span.RecordError(verr)
			}
			span.SetStatus(codes.Error, "invalid password")
			http.Error(w, `{"message":"invalid password"}`, http.StatusForbidden)
			return
		}
	}

	if profile.Parental == nil {
		profile.Parental = &domainuser.ParentalSettings{}
	}
	if body.UnsetMaxAge {
		profile.Parental.MaxAge = nil
	} else if body.MaxAge != nil {
		profile.Parental.MaxAge = body.MaxAge
	}
	if body.BlockedTags != nil {
		profile.Parental.BlockedTags = body.BlockedTags
	}
	if body.PIN != nil {
		if *body.PIN == "" {
			// Clearing PIN is only allowed if this profile is kids OR no sibling is kids.
			if !profile.IsKids {
				siblings, sErr := h.profileRepo.GetByUserID(r.Context(), profile.UserID)
				if sErr != nil {
					span.RecordError(sErr)
					span.SetStatus(codes.Error, "siblings lookup failed")
					http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
					return
				}
				for _, s := range siblings {
					if s != nil && s.ID != profile.ID && s.IsKids {
						span.SetStatus(codes.Error, "kids sibling requires PIN")
						http.Error(w, `{"message":"non-kids profile must have a PIN when a kids profile exists"}`, http.StatusConflict)
						return
					}
				}
			}
			profile.Parental.PINHash = ""
		} else {
			hash, err := h.hasher.Hash(*body.PIN)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "hash failed")
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
		span.RecordError(err)
		span.SetStatus(codes.Error, "profile update failed")
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.Warn("write failed", slog.Any("err", err))
	}
}

func isFourDigit(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (h *parentalHandler) verifyPIN(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.parental.VerifyPIN")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")
	var body struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "bad request body")
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "profile not found")
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && string(profile.UserID) != claims.UserID {
		span.SetStatus(codes.Error, "forbidden")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	// Rate-limit only authorized callers — prevents cross-user DoS of the
	// per-profile bucket.
	if !h.pinRL.Allow(profileID) {
		span.SetStatus(codes.Error, "rate limited")
		http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
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
		span.RecordError(err)
		span.SetStatus(codes.Error, "verify failed")
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
	userRepo domainuser.UserRepository,
	hasher domainuser.PasswordHasher,
	tokenGen *auth.JWTGenerator,
) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &parentalHandler{
		profileRepo: profileRepo,
		userRepo:    userRepo,
		hasher:      hasher,
		// 15 attempts up front, then refill 1 token every 20 seconds (3/min).
		// Comfortable for a household with kids fat-fingering the PIN; after
		// burnout still caps a brute-force attacker to ~180 tries/hour.
		pinRL: ratelimit.NewWithBurst(15, 3.0),
	}
	mux.HandleFunc("GET /api/v1/profiles/{id}/parental", authMw(h.getSettings))
	mux.HandleFunc("PUT /api/v1/profiles/{id}/parental", authMw(h.updateSettings))
	mux.HandleFunc("POST /api/v1/profiles/{id}/parental/verify-pin", authMw(h.verifyPIN))
}
