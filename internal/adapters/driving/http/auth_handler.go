package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	appuser "github.com/evey-project/evey/internal/app/user"

	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	oidcSessionCookieName = "evey_oidc_session"
	oidcSessionCookieAge  = 5 * 60 // 5 minutes - one-time exchange

	oidcIDTokenCookieName = "evey_oidc_id_token"

	refreshCookieName   = "evey_refresh"
	refreshCookieMaxAge = 30 * 24 * 3600 // 30 days
)

func registerOIDCRoutes(mux *http.ServeMux, oidcSvc *appuser.OIDCService, httpsOnly bool, oidcLoginRL ...*ratelimit.Limiter) {
	registerOIDCRoutesFn(mux, oidcSvc, fixedBool(httpsOnly), oidcLoginRL...)
}

// oidcErrorReason is the code the login screens turn into a sentence. Only
// these values ever reach a redirect: nothing from the IdP is echoed.
func oidcErrorReason(err error) string {
	switch {
	case errors.Is(err, appuser.ErrEmailDomainNotAllowed):
		return "domain_not_allowed"
	case errors.Is(err, appuser.ErrOIDCSignupDisabled):
		return "signup_disabled"
	case errors.Is(err, appuser.ErrUserDisabled):
		return "account_disabled"
	case errors.Is(err, appuser.ErrOIDCAccountExists):
		return "account_exists"
	case errors.Is(err, appuser.ErrOIDCUnavailable), errors.Is(err, appuser.ErrOIDCDisabled):
		return "unavailable"
	case errors.Is(err, appuser.ErrStateExpired):
		return "expired"
	}
	return "failed"
}

// oidcEndSessionURL builds the provider logout URL with id_token_hint and the
// registered post-logout URI (fallback when the settings give none).
func oidcEndSessionURL(endpoint, postLogout, fallback, idToken string) string {
	if postLogout == "" {
		postLogout = fallback
	}
	q := url.Values{}
	q.Set("post_logout_redirect_uri", postLogout)
	q.Set("id_token_hint", idToken)
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return endpoint + sep + q.Encode()
}

func registerOIDCRoutesFn(mux *http.ServeMux, oidcSvc *appuser.OIDCService, httpsOnly func() bool, oidcLoginRL ...*ratelimit.Limiter) {
	var loginRL *ratelimit.Limiter
	if len(oidcLoginRL) > 0 {
		loginRL = oidcLoginRL[0]
	}
	limited := func(h http.HandlerFunc) http.HandlerFunc {
		if loginRL == nil {
			return h
		}
		return ratelimit.MiddlewareFunc(loginRL, h)
	}
	flows := newOIDCAppFlows()

	// fail sends the browser back to whoever started the login: the app's
	// deep link for an app flow, the web login page otherwise.
	fail := func(w http.ResponseWriter, r *http.Request, nonce, reason string) {
		if nonce != "" && flows.fail(nonce) {
			http.Redirect(w, r, oidcAppRedirect+"?oidc_error="+url.QueryEscape(reason), http.StatusFound)
			return
		}
		http.Redirect(w, r, "/login?oidc_error="+url.QueryEscape(reason), http.StatusFound)
	}

	mux.HandleFunc("GET /api/v1/auth/oidc/config", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.auth.oidc.GetConfig")
		defer span.End()
		// Enabled only once the provider answered discovery: a dead IdP hides
		// the button instead of sending people to an error page.
		ok, label := oidcSvc.Available(ctx)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"enabled": ok, "buttonLabel": label}); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/auth/oidc/login", limited(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.auth.oidc.Login")
		defer span.End()
		// An app flow reuses its id as the nonce, so the callback knows where
		// to send the result. An unknown id is refused, never minted here.
		nonce := r.URL.Query().Get("flow")
		if nonce != "" && !flows.pending(nonce) {
			span.SetStatus(codes.Error, "unknown app flow")
			fail(w, r, "", "expired")
			return
		}
		if nonce == "" {
			var nonceBuf [16]byte
			if _, err := rand.Read(nonceBuf[:]); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "rand read failed")
				logInternal(ctx, "oidc login: nonce", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			nonce = hex.EncodeToString(nonceBuf[:])
		}
		authURL, err := oidcSvc.AuthURL(ctx, nonce)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "provider unavailable")
			slog.WarnContext(ctx, "oidc login: provider unavailable", slog.Any("err", err))
			fail(w, r, nonce, oidcErrorReason(err))
			return
		}
		http.Redirect(w, r, authURL, http.StatusFound)
	}))

	mux.HandleFunc("GET /api/v1/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.auth.oidc.Callback")
		defer span.End()
		r = r.WithContext(ctx)
		q := r.URL.Query()
		code := q.Get("code")
		state := q.Get("state")

		// The IdP reports a refusal or a cancel as ?error=...; the state, when
		// valid, still says which client to send the person back to.
		if idpErr := q.Get("error"); idpErr != "" {
			nonce, stateErr := oidcSvc.ValidateState(state)
			if stateErr != nil {
				nonce = "" // unknown origin: the web login page
			}
			reason := "failed"
			if idpErr == "access_denied" {
				reason = "cancelled"
			}
			span.SetStatus(codes.Error, "idp error")
			slog.WarnContext(ctx, "oidc callback: provider returned an error", slog.String("error", idpErr))
			fail(w, r, nonce, reason)
			return
		}

		if code == "" || state == "" {
			span.SetStatus(codes.Error, "missing code or state")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		nonce, err := oidcSvc.ValidateState(state)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid state")
			slog.WarnContext(ctx, "oidc callback: invalid state", slog.Any("err", err))
			if errors.Is(err, appuser.ErrStateExpired) {
				fail(w, r, "", "expired")
				return
			}
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}

		accessToken, refreshToken, rawIDToken, err := oidcSvc.HandleCallback(r.Context(), code, nonce)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "handle callback failed")
			reason := oidcErrorReason(err)
			if reason == "failed" {
				slog.ErrorContext(ctx, "oidc callback: handle failed", slog.Any("err", err))
			} else {
				slog.WarnContext(ctx, "oidc callback: sign-in refused", slog.String("reason", reason), slog.Any("err", err))
			}
			fail(w, r, nonce, reason)
			return
		}

		// App flow: the tokens wait server-side for the app, which redeems them
		// with the one-time code below AND the poll secret only it holds.
		if oneTime, ok := flows.complete(nonce, accessToken, refreshToken); ok {
			http.Redirect(w, r, oidcAppRedirect+"?code="+url.QueryEscape(oneTime), http.StatusFound)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     oidcSessionCookieName,
			Value:    accessToken,
			Path:     "/api/v1/auth/oidc/session",
			HttpOnly: true,
			Secure:   httpsOnly(),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   oidcSessionCookieAge,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    refreshToken,
			Path:     "/api/v1/auth/",
			HttpOnly: true,
			Secure:   httpsOnly(),
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
				Secure:   httpsOnly(),
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
			Secure:   httpsOnly(),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"accessToken": cookie.Value}); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	})

	registerOIDCAppRoutes(mux, oidcSvc, flows, limited)
}

type parentalHandler struct {
	profileRepo domainuser.ProfileRepository
	userRepo    domainuser.UserRepository
	hasher      domainuser.PasswordHasher
	tokenGen    domainuser.TokenGenerator
	pinRL       *ratelimit.Limiter
}

// pinProofTTL is how long a verified-PIN proof stays usable for a profile
// switch. Short: it only needs to survive the click that follows the dialog.
const pinProofTTL = 5 * time.Minute

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
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

// parentalUpdateBody is the PUT /parental payload. It accepts both `pin`
// (legacy, kept for existing tests) and `newPin` (the field name the
// proto/orval-generated client actually sends) - the handler is not wired
// through the gRPC gateway, so without this accommodation the frontend call
// to save a PIN was a silent no-op.
type parentalUpdateBody struct {
	MaxAge          *int     `json:"maxAge"`
	UnsetMaxAge     bool     `json:"unsetMaxAge"`
	BlockedTags     []string `json:"blockedTags"`
	PIN             *string  `json:"pin"`
	NewPIN          *string  `json:"newPin"`
	CurrentPIN      string   `json:"currentPin"`
	CurrentPassword string   `json:"currentPassword"`
	Enabled         *bool    `json:"enabled"`
}

func (h *parentalHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.parental.UpdateSettings")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	profileID := r.PathValue("id")

	body, ok := decodeParentalUpdateBody(w, r, span)
	if !ok {
		return
	}

	profile, ok := h.loadOwnedProfileForUpdate(w, r, span, claims, profileID)
	if !ok {
		return
	}

	// The caller must not be sitting on a restricted profile. Ownership alone is
	// not enough here: a kids profile OWNS itself, so loadOwnedProfileForUpdate
	// happily let it raise its own MaxAge, wipe its blocked tags, or switch
	// protection off - neutralising the control on the whole catalogue with one
	// request, no PIN and no profile switch. The gRPC twin of this mutation
	// (enforceParentalUpdateGate) has always refused that; this route is the
	// hand-written sibling that never got the gate.
	if !h.refuseFromKidsProfile(w, r, span, claims, profile) {
		return
	}

	if !validateNewPINFormat(w, span, body.PIN) {
		return
	}

	if !h.authorizeParentalChange(w, r, span, profile, body, profileID) {
		return
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
	if !h.applyPINChange(w, r, span, profile, body) {
		return
	}
	if body.Enabled != nil {
		profile.Parental.Enabled = *body.Enabled
	}

	h.persistAndRespond(w, r, span, profile)
}

// decodeParentalUpdateBody parses the request payload and unifies the two
// PIN field names the frontend may send.
func decodeParentalUpdateBody(w http.ResponseWriter, r *http.Request, span trace.Span) (parentalUpdateBody, bool) {
	var body parentalUpdateBody
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "bad request body")
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return body, false
	}
	// Unify the two field names - prefer `pin` when both are sent.
	if body.PIN == nil {
		body.PIN = body.NewPIN
	}
	return body, true
}

// loadOwnedProfileForUpdate fetches the target profile and enforces that the
// caller is either an admin or the account owner before it can be mutated.
func (h *parentalHandler) loadOwnedProfileForUpdate(w http.ResponseWriter, r *http.Request, span trace.Span, claims *domainuser.Claims, profileID string) (*domainuser.Profile, bool) {
	profile, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "profile not found")
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return nil, false
	}
	if !claims.IsAdmin && string(profile.UserID) != claims.UserID {
		span.SetStatus(codes.Error, "forbidden")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return nil, false
	}
	return profile, true
}

// refuseFromKidsProfile blocks any parental mutation made while the CALLER is
// on a kids profile, whatever profile is being edited. Mirrors the gRPC
// enforceParentalUpdateGate so the two routes onto the same fields can't
// disagree.
//
// Deliberately not admin-exempt: the question is not "is this account
// privileged" but "is this device currently driven by a restricted profile" -
// an admin who switched to a kids profile is, for the duration, restricted.
// Same stance as the profile-management gate.
//
// Fails CLOSED: if the caller's profile can't be loaded we refuse, because the
// only thing worse than a false refusal here is a silent bypass.
func (h *parentalHandler) refuseFromKidsProfile(w http.ResponseWriter, r *http.Request, span trace.Span, claims *domainuser.Claims, target *domainuser.Profile) bool {
	if claims == nil || claims.ProfileID == "" {
		return true // no profile context (service/stream tokens): nothing to restrict
	}
	callerIsKids := target.IsKids && claims.ProfileID == string(target.ID)
	if !callerIsKids && claims.ProfileID != string(target.ID) {
		caller, err := h.profileRepo.GetByID(r.Context(), shared.ProfileID(claims.ProfileID))
		if err != nil || caller == nil {
			span.SetStatus(codes.Error, "parental gate: caller profile unreadable")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return false
		}
		callerIsKids = caller.IsKids
	}
	if callerIsKids {
		span.SetStatus(codes.Error, "parental change from kids profile")
		http.Error(w, `{"message":"errors.auth.parental_needs_non_kids"}`, http.StatusForbidden)
		return false
	}
	return true
}

// validateNewPINFormat rejects a proposed PIN that isn't 4 digits. An empty
// or absent PIN is left to the caller (it means "no PIN change" or "clear
// the PIN", not "set one").
func validateNewPINFormat(w http.ResponseWriter, span trace.Span, pin *string) bool {
	if pin == nil || *pin == "" {
		return true
	}
	if !isFourDigit(*pin) {
		span.SetStatus(codes.Error, "PIN not 4 digits")
		http.Error(w, `{"message":"errors.auth.pin_must_be_4_digits"}`, http.StatusBadRequest)
		return false
	}
	return true
}

// authorizeParentalChange enforces the reauthentication gate for security-
// sensitive parental mutations: rotating/clearing an existing PIN or
// disabling protection requires the current PIN, and setting a PIN for the
// first time requires the account password. Other updates (max age, blocked
// tags, enabling) are not gated - the user already authenticated to reach
// this endpoint.
func (h *parentalHandler) authorizeParentalChange(w http.ResponseWriter, r *http.Request, span trace.Span, profile *domainuser.Profile, body parentalUpdateBody, profileID string) bool {
	noExistingPIN := profile.Parental == nil || profile.Parental.PINHash == ""
	settingNewPIN := body.PIN != nil && *body.PIN != ""
	mutatingPIN := body.PIN != nil // rotation, clear, or first-time set
	disablingProtection := body.Enabled != nil && !*body.Enabled
	// Lifting the age cap, raising it or unblocking a tag frees the profile
	// just like disabling does - and went through on the profile's own JWT,
	// PIN or not (audit 2026-10-01). Tightening stays ungated.
	loosening := loosensParental(profile.Parental, body)

	switch {
	case (mutatingPIN || disablingProtection || loosening) && !noExistingPIN:
		return h.requireCurrentPIN(w, r, span, profile, body, profileID)
	case settingNewPIN && noExistingPIN && h.userRepo != nil:
		// First-time PIN set: require the account password. Without this a
		// stolen JWT could silently lock the owner out by enabling parental
		// + setting a PIN the attacker picks.
		return h.requireAccountPassword(w, r, span, profile, body, profileID)
	}
	return true
}

// loosensParental reports whether body relaxes the restrictions cur holds:
// the age cap removed or raised, or a blocked tag no longer blocked.
func loosensParental(cur *domainuser.ParentalSettings, body parentalUpdateBody) bool {
	if cur == nil {
		return false
	}
	if cur.MaxAge != nil {
		if body.UnsetMaxAge {
			return true
		}
		if body.MaxAge != nil && *body.MaxAge > *cur.MaxAge {
			return true
		}
	}
	if body.BlockedTags != nil {
		kept := make(map[string]bool, len(body.BlockedTags))
		for _, tag := range body.BlockedTags {
			kept[tag] = true
		}
		for _, tag := range cur.BlockedTags {
			if !kept[tag] {
				return true
			}
		}
	}
	return false
}

// requireCurrentPIN gates rotating/clearing an existing PIN (or disabling
// protection while one is set) behind proof of the current PIN.
func (h *parentalHandler) requireCurrentPIN(w http.ResponseWriter, r *http.Request, span trace.Span, profile *domainuser.Profile, body parentalUpdateBody, profileID string) bool {
	if !h.pinRL.Allow(profileID) {
		span.SetStatus(codes.Error, "rate limited")
		recordParentalPINThrottle(r, "verify_current_pin", profileID)
		http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
		return false
	}
	valid, err := h.hasher.Verify(body.CurrentPIN, profile.Parental.PINHash)
	if err != nil || !valid {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(codes.Error, "invalid current PIN")
		http.Error(w, `{"message":"errors.auth.invalid_pin"}`, http.StatusForbidden)
		return false
	}
	return true
}

// requireAccountPassword gates a first-time PIN set behind proof of the
// account password.
func (h *parentalHandler) requireAccountPassword(w http.ResponseWriter, r *http.Request, span trace.Span, profile *domainuser.Profile, body parentalUpdateBody, profileID string) bool {
	if !h.pinRL.Allow(profileID) {
		span.SetStatus(codes.Error, "rate limited")
		recordParentalPINThrottle(r, "verify_account_password", profileID)
		http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
		return false
	}
	u, uerr := h.userRepo.GetByID(r.Context(), profile.UserID)
	if uerr != nil || u == nil {
		if uerr != nil {
			span.RecordError(uerr)
		}
		span.SetStatus(codes.Error, "user lookup failed")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return false
	}
	valid, verr := h.hasher.Verify(body.CurrentPassword, u.PasswordHash)
	if verr != nil || !valid {
		if verr != nil {
			span.RecordError(verr)
		}
		span.SetStatus(codes.Error, "invalid password")
		http.Error(w, `{"message":"errors.auth.invalid_password"}`, http.StatusForbidden)
		return false
	}
	return true
}

// applyPINChange handles the PIN mutation itself: clearing an existing PIN or
// hashing and storing a newly chosen one. A nil PIN means no change. Whether
// clearing is actually allowed (no kids sibling left without a guardian PIN)
// is NOT decided here - it used to reimplement that sibling scan inline,
// duplicating (and able to drift from) the domain invariant; persistAndRespond
// now re-validates the fully mutated profile through the single shared
// enforceKidsPINInvariant choke point immediately before the write.
func (h *parentalHandler) applyPINChange(w http.ResponseWriter, r *http.Request, span trace.Span, profile *domainuser.Profile, body parentalUpdateBody) bool {
	if body.PIN == nil {
		return true
	}
	if *body.PIN == "" {
		profile.Parental.PINHash = ""
		return true
	}
	hash, err := h.hasher.Hash(*body.PIN)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash failed")
		logInternal(r.Context(), "parental settings: hash pin", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return false
	}
	profile.Parental.PINHash = hash
	return true
}

// enforceKidsPINInvariant re-validates the domain kids/PIN data invariant
// against candidate's CURRENT sibling state, immediately before writing a
// profile. It is the single choke point every profile-mutating handler in
// this file (and avatar_handler.go) routes through, instead of each one
// re-deriving its own (and possibly diverging) copy of the rule.
//
// users may be nil (avatar_handler.go has no UserRepository dependency): the
// admin-cannot-be-kids check is then skipped, which is safe because that
// caller never touches IsKids.
func enforceKidsPINInvariant(ctx context.Context, profiles domainuser.ProfileRepository, users domainuser.UserRepository, p *domainuser.Profile) error {
	siblings, err := profiles.GetByUserID(ctx, p.UserID)
	if err != nil {
		return fmt.Errorf("load siblings: %w", err)
	}
	if invErr := domainuser.CheckKidsPINInvariant(p, siblings); invErr != nil {
		return invErr
	}
	if users == nil {
		return nil
	}
	// The owner lookup failing is not fatal - mirrors the CreateProfile
	// precedent: the admin-cannot-be-kids check is a safety rail, not the
	// invariant itself, and refusing an otherwise-valid profile write because a
	// secondary read hiccuped would be worse than skipping the rail.
	owner, ownerErr := users.GetByID(ctx, p.UserID)
	if ownerErr != nil || owner == nil {
		return nil //nolint:nilerr // deliberate: see comment above
	}
	return domainuser.CheckAdminCannotBeKids(owner.IsAdmin, p)
}

// persistAndRespond re-validates the invariant against the fully mutated
// profile, then writes ONLY the parental_* columns (this handler never
// touches any other field) and the success payload.
func (h *parentalHandler) persistAndRespond(w http.ResponseWriter, r *http.Request, span trace.Span, profile *domainuser.Profile) {
	if err := enforceKidsPINInvariant(r.Context(), h.profileRepo, h.userRepo, profile); err != nil {
		span.RecordError(err)
		if errors.Is(err, domainuser.ErrKidsRequiresPIN) {
			span.SetStatus(codes.Error, "kids sibling requires PIN")
			http.Error(w, `{"message":"errors.auth.pin_required_for_non_kids"}`, http.StatusConflict)
			return
		}
		span.SetStatus(codes.Error, "invariant check failed")
		logInternal(r.Context(), "parental settings: invariant check", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if err := h.profileRepo.UpdateParentalSettings(r.Context(), profile.ID, profile.Parental); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "profile update failed")
		logInternal(r.Context(), "parental settings: update profile", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		slog.WarnContext(r.Context(), "write failed", slog.Any("err", err))
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
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
	// Rate-limit only authorized callers - prevents cross-user DoS of the
	// per-profile bucket.
	if !h.pinRL.Allow(profileID) {
		span.SetStatus(codes.Error, "rate limited")
		recordParentalPINThrottle(r, "switch_profile_pin", profileID)
		http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
		return
	}
	if profile.Parental == nil || profile.Parental.PINHash == "" {
		// No PIN set → trivially valid; still mint a proof so the switch flow is uniform.
		h.writePINResult(w, profileID, true)
		return
	}
	valid, err := h.hasher.Verify(body.PIN, profile.Parental.PINHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "verify failed")
		logInternal(r.Context(), "parental pin: verify", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.writePINResult(w, profileID, valid)
}

// writePINResult encodes {valid, pinToken?}. On success it mints a short-lived
// proof (consumed by RefreshToken to authorise a switch out of a kids profile).
func (h *parentalHandler) writePINResult(w http.ResponseWriter, profileID string, valid bool) {
	out := map[string]any{"valid": valid}
	if valid && h.tokenGen != nil {
		if tok, err := h.tokenGen.GeneratePINToken(profileID, pinProofTTL); err == nil {
			out["pinToken"] = tok
		} else {
			slog.Warn("mint pin proof", slog.Any("err", err))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func registerParentalRoutes(
	mux *http.ServeMux,
	profileRepo domainuser.ProfileRepository,
	userRepo domainuser.UserRepository,
	hasher domainuser.PasswordHasher,
	tokenGen domainuser.TokenGenerator,
) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &parentalHandler{
		profileRepo: profileRepo,
		userRepo:    userRepo,
		hasher:      hasher,
		tokenGen:    tokenGen,
		// 15 attempts up front, then refill 1 token every 20 seconds (3/min).
		// Comfortable for a household with kids fat-fingering the PIN; after
		// burnout still caps a brute-force attacker to ~180 tries/hour.
		pinRL: ratelimit.NewWithBurst(15, 3.0),
	}
	mux.HandleFunc("GET /api/v1/profiles/{id}/parental", authMw(h.getSettings))
	mux.HandleFunc("PUT /api/v1/profiles/{id}/parental", authMw(h.updateSettings))
	mux.HandleFunc("POST /api/v1/profiles/{id}/parental/verify-pin", authMw(h.verifyPIN))
}

// recordParentalPINThrottle emits the ops signals for a throttled parental-PIN
// attempt. The gRPC limiter has done this since it was written, with the
// reason spelled out there: the limiter runs before any logging, so a throttled
// brute-force attempt is otherwise INVISIBLE. These three HTTP sites - guessing
// a child profile's PIN, or grinding the account password to set a PIN of one's
// own - had only an OTel span, which needs a tracing backend somebody is
// actually watching. On a single-operator NAS, that is nobody.
func recordParentalPINThrottle(r *http.Request, op, profileID string) {
	metrics.RateLimitRejected.WithLabelValues("http_parental_pin").Inc()
	slog.WarnContext(r.Context(), "parental PIN attempt throttled",
		slog.String("operation", op),
		slog.String("profile_id", profileID),
		slog.String("client_ip", ratelimit.ClientIP(r)))
}
