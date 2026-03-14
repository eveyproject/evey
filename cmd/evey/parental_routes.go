package main

import (
	"encoding/json"
	"net/http"

	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func registerParentalRoutes(
	mux *http.ServeMux,
	profileRepo domainuser.ProfileRepository,
	hasher domainuser.PasswordHasher,
	tokenGen *auth.JWTGenerator,
) {
	// 5 attempts per profile per 15 minutes (brute-force protection on 4-6 digit PINs)
	pinRL := ratelimit.NewWithBurst(5, 5.0/15.0)

	extractClaims := func(r *http.Request) (*domainuser.Claims, bool) {
		h := r.Header.Get("Authorization")
		if len(h) < 8 || h[:7] != "Bearer " {
			return nil, false
		}
		claims, err := tokenGen.ValidateAccessToken(h[7:])
		if err != nil {
			return nil, false
		}
		return claims, true
	}

	// GET parental settings
	mux.HandleFunc("GET /api/v1/profiles/{id}/parental", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		profileID := r.PathValue("id")
		if profileID != claims.ProfileID && !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		profile, err := profileRepo.GetByID(r.Context(), profileID)
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
		json.NewEncoder(w).Encode(out)
	})

	// PUT update parental settings (requires current PIN if one is set)
	mux.HandleFunc("PUT /api/v1/profiles/{id}/parental", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
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
		profile, err := profileRepo.GetByID(r.Context(), profileID)
		if err != nil {
			http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
			return
		}

		// if PIN already set, require current PIN to modify
		if profile.Parental != nil && profile.Parental.PINHash != "" {
			valid, err := hasher.Verify(body.CurrentPIN, profile.Parental.PINHash)
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
				hash, err := hasher.Hash(*body.PIN)
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

		if err := profileRepo.Update(r.Context(), profile); err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// POST verify PIN
	mux.HandleFunc("POST /api/v1/profiles/{id}/parental/verify-pin", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		profileID := r.PathValue("id")
		if !pinRL.Allow(profileID) {
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
		profile, err := profileRepo.GetByID(r.Context(), profileID)
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
			json.NewEncoder(w).Encode(map[string]any{"valid": true})
			return
		}
		valid, err := hasher.Verify(body.PIN, profile.Parental.PINHash)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"valid": valid})
	})
}
