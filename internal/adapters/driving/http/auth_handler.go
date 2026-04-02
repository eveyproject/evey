package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/notification"
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

var pushRegisterLimiter = ratelimit.New(10)

type pushHandler struct {
	subs        notification.PushSubscriptionRepository
	profileRepo domainuser.ProfileRepository
	userRepo    domainuser.UserRepository
	tokenGen    *auth.JWTGenerator
	vapidPubKey string
}


func (h *pushHandler) register(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	if !pushRegisterLimiter.Allow(claims.UserID) {
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		Consent bool `json:"consent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Endpoint == "" || body.Keys.P256dh == "" || body.Keys.Auth == "" {
		http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if !body.Consent {
		http.Error(w, `{"message":"explicit consent required for push notifications"}`, http.StatusBadRequest)
		return
	}
	if claims.ProfileID != "" {
		profile, err := h.profileRepo.GetByID(r.Context(), claims.ProfileID)
		if err != nil || profile.UserID != claims.UserID {
			slog.Warn("push register: profile ownership mismatch", slog.String("user_id", claims.UserID), slog.String("profile_id", claims.ProfileID))
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}
	usr, err := h.userRepo.GetByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if !usr.PushConsent {
		usr.PushConsent = true
		if err := h.userRepo.Update(r.Context(), usr); err != nil {
			slog.Warn("push register: persist consent failed", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		slog.Info("push consent persisted", slog.String("user_id", claims.UserID))
	}
	if err := h.subs.Upsert(r.Context(), claims.UserID, claims.ProfileID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth); err != nil {
		slog.Warn("push register: upsert failed", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.Warn("write failed", slog.Any("err", err))
	}
}

func (h *pushHandler) unregister(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Endpoint == "" {
		http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if err := h.subs.DeleteByEndpoint(r.Context(), claims.UserID, body.Endpoint); err != nil {
		slog.Warn("push unregister: delete failed", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.Warn("write failed", slog.Any("err", err))
	}
}

func (h *pushHandler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	subs, err := h.subs.ListByUser(r.Context(), claims.UserID)
	if err != nil {
		slog.Warn("push subscriptions: list failed", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type subResp struct {
		Endpoint  string `json:"endpoint"`
		CreatedAt string `json:"createdAt"`
	}
	out := make([]subResp, 0, len(subs))
	for _, s := range subs {
		out = append(out, subResp{
			Endpoint:  s.Endpoint,
			CreatedAt: s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"subscriptions": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *pushHandler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Endpoint == "" {
		http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if err := h.subs.DeleteByEndpoint(r.Context(), claims.UserID, body.Endpoint); err != nil {
		slog.Warn("push subscriptions: delete failed", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.Warn("write failed", slog.Any("err", err))
	}
}

func (h *pushHandler) getConsent(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	usr, err := h.userRepo.GetByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"pushConsent": usr.PushConsent}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *pushHandler) updateConsent(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Consent bool `json:"consent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
		return
	}
	usr, err := h.userRepo.GetByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	usr.PushConsent = body.Consent
	if err := h.userRepo.Update(r.Context(), usr); err != nil {
		slog.Warn("push consent: update failed", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if !body.Consent {
		if err := h.subs.DeleteByUser(r.Context(), claims.UserID); err != nil {
			slog.Warn("push consent: delete subscriptions failed", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		slog.Info("push consent revoked, subscriptions deleted", slog.String("user_id", claims.UserID))
	} else {
		slog.Info("push consent granted", slog.String("user_id", claims.UserID))
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.Warn("write failed", slog.Any("err", err))
	}
}

func registerPushRoutes(mux *http.ServeMux, subs notification.PushSubscriptionRepository, profileRepo domainuser.ProfileRepository, userRepo domainuser.UserRepository, tokenGen *auth.JWTGenerator, vapidPubKey string) {
	h := &pushHandler{subs: subs, profileRepo: profileRepo, userRepo: userRepo, tokenGen: tokenGen, vapidPubKey: vapidPubKey}
	mux.HandleFunc("GET /api/v1/push/vapid-key", h.vapidKey)
	mux.HandleFunc("POST /api/v1/push/register", h.register)
	mux.HandleFunc("DELETE /api/v1/push/unregister", h.unregister)
	mux.HandleFunc("GET /api/v1/push/subscriptions", h.listSubscriptions)
	mux.HandleFunc("DELETE /api/v1/push/subscriptions", h.deleteSubscription)
	mux.HandleFunc("GET /api/v1/push/consent", h.getConsent)
	mux.HandleFunc("PUT /api/v1/push/consent", h.updateConsent)
}

func (h *pushHandler) vapidKey(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"publicKey": h.vapidPubKey}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

type parentalHandler struct {
	profileRepo domainuser.ProfileRepository
	hasher      domainuser.PasswordHasher
	tokenGen    *auth.JWTGenerator
	pinRL       *ratelimit.Limiter
}

func (h *parentalHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	profileID := r.PathValue("id")
	if profileID != claims.ProfileID && !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	profile, err := h.profileRepo.GetByID(r.Context(), profileID)
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
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
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
	profile, err := h.profileRepo.GetByID(r.Context(), profileID)
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
	claims, ok := extractClaims(r, h.tokenGen)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
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
	profile, err := h.profileRepo.GetByID(r.Context(), profileID)
	if err != nil {
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && profile.UserID != claims.UserID {
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
	h := &parentalHandler{
		profileRepo: profileRepo,
		hasher:      hasher,
		tokenGen:    tokenGen,
		pinRL:       ratelimit.NewWithBurst(5, 5.0/15.0),
	}
	mux.HandleFunc("GET /api/v1/profiles/{id}/parental", h.getSettings)
	mux.HandleFunc("PUT /api/v1/profiles/{id}/parental", h.updateSettings)
	mux.HandleFunc("POST /api/v1/profiles/{id}/parental/verify-pin", h.verifyPIN)
}
