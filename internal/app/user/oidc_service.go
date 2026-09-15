package user

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

var ErrOIDCDisabled = errors.New("OIDC not enabled")
var ErrEmailDomainNotAllowed = errors.New("email domain not allowed")
var ErrStateExpired = errors.New("OIDC state expired")

const oidcStateTTL = 10 * time.Minute

// oidcHTTPTimeout bounds every outbound call to the IdP (discovery, token
// exchange, JWKS fetch). Without it those calls ride http.DefaultClient
// (Timeout=0, no ResponseHeaderTimeout): a hung issuer used to block server
// BOOT indefinitely (discovery runs at startup - on a NAS that auto-restarts,
// that's a boot-loop while the IdP is down) and stall login-callback handler
// goroutines. A plain timeout, NOT the ssrf.SafeHTTPClient: the issuer is
// ADMIN-configured (trusted operator input), and self-hosted IdPs on private
// LAN addresses are a first-class deployment for a self-hosted media server -
// the SSRF dialer would block exactly those.
const oidcHTTPTimeout = 15 * time.Second

type OIDCConfig struct {
	Enabled             bool
	Issuer              string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	AllowedEmailDomains []string
}

type OIDCService struct {
	provider  *gooidc.Provider
	oauth2Cfg oauth2.Config
	verifier  *gooidc.IDTokenVerifier
	// httpClient carries the bounded-timeout client every IdP call must use
	// (see oidcHTTPTimeout). nil (hand-built test instances) falls back to the
	// caller's ctx client / http.DefaultClient.
	httpClient          *http.Client
	userRepo            domainuser.UserRepository
	profiles            domainuser.ProfileRepository
	refreshTkns         domainuser.RefreshTokenRepository
	userRoles           domainuser.UserRoleRepository
	tokens              domainuser.TokenGenerator
	events              shared.EventBus
	uow                 shared.TransactionalUnit
	stateSecret         []byte
	issuer              string
	allowedEmailDomains []string
	refreshTokenTTL     time.Duration
}

func NewOIDCService(
	ctx context.Context,
	cfg OIDCConfig,
	userRepo domainuser.UserRepository,
	profiles domainuser.ProfileRepository,
	refreshTkns domainuser.RefreshTokenRepository,
	userRoles domainuser.UserRoleRepository,
	tokens domainuser.TokenGenerator,
	events shared.EventBus,
	uow shared.TransactionalUnit,
	refreshTokenTTL time.Duration,
) (*OIDCService, error) {
	if !cfg.Enabled {
		return nil, ErrOIDCDisabled
	}
	if cfg.ClientSecret == "" {
		return nil, errors.New("OIDC: client secret must not be empty")
	}
	// Discovery runs at BOOT (wire.go) on a root ctx with no deadline - the
	// bounded client is what keeps a hung issuer from blocking startup.
	httpClient := &http.Client{Timeout: oidcHTTPTimeout}
	provider, err := gooidc.NewProvider(gooidc.ClientContext(ctx, httpClient), cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider discovery: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})

	// derive state signing secret from client secret
	h := sha256.Sum256([]byte(cfg.ClientSecret + "oidc-state"))

	return &OIDCService{
		provider:            provider,
		oauth2Cfg:           oauth2Cfg,
		verifier:            verifier,
		httpClient:          httpClient,
		userRepo:            userRepo,
		profiles:            profiles,
		refreshTkns:         refreshTkns,
		userRoles:           userRoles,
		tokens:              tokens,
		events:              events,
		uow:                 uow,
		stateSecret:         h[:],
		issuer:              cfg.Issuer,
		allowedEmailDomains: cfg.AllowedEmailDomains,
		refreshTokenTTL:     refreshTokenTTL,
	}, nil
}

// EndSessionEndpoint returns the OIDC end_session_endpoint from provider discovery, or "".
func (s *OIDCService) EndSessionEndpoint() string {
	var claims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := s.provider.Claims(&claims); err != nil {
		return ""
	}
	return claims.EndSessionEndpoint
}

// AuthURL returns the authorization URL. state is a random nonce from the caller.
// The state encodes a unix timestamp for TTL enforcement: "ts|nonce.sig"
func (s *OIDCService) AuthURL(state string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := ts + "|" + state
	sig := s.signState(payload)
	// Also send `state` as the OpenID Connect nonce. The IdP must echo it in the
	// id_token's nonce claim, which HandleCallback verifies - defence-in-depth on
	// top of the signed/TTL'd state that binds the id_token to this exact login
	// and blocks id_token replay/injection.
	return s.oauth2Cfg.AuthCodeURL(payload+"."+sig, oauth2.SetAuthURLParam("nonce", state))
}

// ValidateState checks the signed state from the callback.
// Expected format: "ts|nonce.sig". Rejects states older than oidcStateTTL.
func (s *OIDCService) ValidateState(rawState string) (string, error) {
	parts := strings.SplitN(rawState, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid state")
	}
	payload, sig := parts[0], parts[1]
	expected := s.signState(payload)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", errors.New("state signature mismatch")
	}

	tsParts := strings.SplitN(payload, "|", 2)
	if len(tsParts) != 2 {
		return "", errors.New("invalid state")
	}
	tsUnix, err := strconv.ParseInt(tsParts[0], 10, 64)
	if err != nil {
		return "", errors.New("invalid state")
	}
	if time.Since(time.Unix(tsUnix, 0)) > oidcStateTTL {
		return "", ErrStateExpired
	}
	return tsParts[1], nil
}

func (s *OIDCService) signState(nonce string) string {
	mac := hmac.New(sha256.New, s.stateSecret)
	mac.Write([]byte(nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// HandleCallback exchanges the code, verifies the ID token, finds/creates the user,
// and returns Evey access + refresh tokens plus the raw OIDC id_token for RP-Initiated Logout.
func (s *OIDCService) HandleCallback(ctx context.Context, code, expectedNonce string) (accessToken, refreshToken, rawOIDCIDToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.HandleCallback")
	defer span.End()
	// Bound the token exchange and the verifier's JWKS fetch: without the
	// injected client both ride http.DefaultClient (no timeout) on a request
	// ctx the server never deadlines (no ReadTimeout/TimeoutHandler), so a
	// slow IdP stalls login handler goroutines indefinitely. nil-safe for
	// hand-built test instances.
	if s.httpClient != nil {
		ctx = gooidc.ClientContext(ctx, s.httpClient)                 // JWKS (verifier.Verify)
		ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient) // token exchange
	}
	oauth2Token, err := s.oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		return "", "", "", fmt.Errorf("token exchange: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return "", "", "", errors.New("missing id_token in response")
	}

	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", "", fmt.Errorf("id_token verification: %w", err)
	}
	// The id_token's nonce must match the one we sent in AuthURL (carried in the
	// signed state). A mismatch means this id_token was not minted for this login
	// request - reject it (blocks replay/injection of a token obtained elsewhere).
	if idToken.Nonce != expectedNonce {
		return "", "", "", errors.New("id_token nonce mismatch")
	}

	var claims struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if claimsErr := idToken.Claims(&claims); claimsErr != nil {
		return "", "", "", fmt.Errorf("parse id_token claims: %w", claimsErr)
	}

	u, err := s.findOrCreateUser(ctx, claims.Sub, claims.Email, claims.PreferredUsername, claims.EmailVerified)
	if err != nil {
		return "", "", "", fmt.Errorf("OIDCService.HandleCallback: %w", err)
	}

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.WarnContext(ctx, "oidc: get roles", slog.String("userID", string(u.ID)), slog.Any("err", err))
	}
	var perms []string
	for _, r := range roles {
		perms = append(perms, r.Permissions...)
	}

	var defaultProfileID string
	// Fail CLOSED, same as the password and token-generation paths: an empty
	// ProfileID is what every parental gate reads as "no restriction", so
	// completing an SSO login past this fault would hand out an unrestricted
	// token. An SSO callback that errors is visible and retryable.
	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		slog.WarnContext(ctx, "oidc: profile lookup failed, refusing rather than issuing a profile-less token",
			slog.String("userID", string(u.ID)), slog.Any("err", pErr))
		return "", "", "", fmt.Errorf("oidc: load profiles: %w", pErr)
	}
	for _, p := range profs {
		if p.IsDefault {
			defaultProfileID = string(p.ID)
			break
		}
	}
	if defaultProfileID == "" && len(profs) > 0 {
		defaultProfileID = string(profs[0].ID)
	}

	var oidcLocale string
	if defaultProfileID != "" {
		if prof, pErr := s.profiles.GetByID(ctx, shared.ProfileID(defaultProfileID)); pErr == nil && prof != nil {
			oidcLocale = prof.Locale
		}
	}
	accessToken, err = s.tokens.GenerateAccessToken(domainuser.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Locale:      oidcLocale,
		Permissions: perms,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), defaultProfileID)
	if err != nil {
		return "", "", "", fmt.Errorf("OIDCService.HandleCallback: %w", err)
	}

	evt := domainuser.LoginSucceeded{UserID: u.ID, AuthMethod: "oidc", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "oidc: publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	if err := s.userRepo.TouchLastLogin(ctx, u.ID, evt.OccurredAt); err != nil {
		slog.WarnContext(ctx, "oidc: touch last_login", slog.String("user_id", string(u.ID)), slog.Any("err", err))
	}
	if defaultProfileID != "" {
		if err := s.profiles.TouchLastActive(ctx, shared.ProfileID(defaultProfileID), evt.OccurredAt); err != nil {
			slog.WarnContext(ctx, "oidc: touch profile last_active", slog.String("profile_id", defaultProfileID), slog.Any("err", err))
		}
	}

	return accessToken, refreshToken, rawIDToken, nil
}

func (s *OIDCService) findOrCreateUser(ctx context.Context, sub, email, preferredUsername string, emailVerified bool) (*domainuser.User, error) {
	u, err := s.userRepo.GetByOIDCSubject(ctx, s.issuer, sub)
	if err == nil {
		if !u.Enabled {
			return nil, ErrUserDisabled
		}
		// NO write here, deliberately. Stamping UpdatedAt went through
		// userRepo.Update, a blind 12-column write with no concurrency control:
		// it replayed a snapshot read microseconds earlier over password_hash,
		// is_admin and enabled, so a concurrent password reset was undone and a
		// demoted user re-promoted himself by logging in. TouchLastLogin (one
		// column) already records the login.
		return u, nil
	}

	// first OIDC login - enforce the email domain allowlist before creating a
	// user. When an allowlist is configured the email is a trust boundary, so
	// require the IdP to have verified it: a lax provider could otherwise assert
	// an arbitrary address in an allowed domain to slip past the allowlist.
	if len(s.allowedEmailDomains) > 0 && !emailVerified {
		slog.WarnContext(ctx, "oidc: unverified email rejected under domain allowlist", slog.String("email", email))
		return nil, ErrEmailDomainNotAllowed
	}
	if !IsEmailAllowed(email, s.allowedEmailDomains) {
		slog.WarnContext(ctx, "oidc: email domain not allowed", slog.String("email", email))
		return nil, ErrEmailDomainNotAllowed
	}

	// An IdP claim that is not an address must not lock the user out of SSO:
	// the account is created without an email rather than not at all.
	if email != "" && !domainuser.IsPlainAddress(email) {
		slog.WarnContext(ctx, "oidc: ignoring malformed email claim", slog.String("email", email))
		email = ""
	}
	username := preferredUsername
	if username == "" && email != "" {
		at := strings.Index(email, "@")
		if at > 0 {
			username = email[:at]
		}
	}
	if username == "" {
		username = "oidc-" + hex.EncodeToString([]byte(sub))[:8]
	}

	now := time.Now()
	issuer := s.issuer
	newUser, err := domainuser.NewUser(username, email, "")
	if err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}
	newUser.OIDCSubject = &sub
	newUser.OIDCIssuer = &issuer
	p, err := domainuser.NewProfile(shared.ProfileID(id.New("prf-")), newUser.ID, newUser.Username,
		domainuser.WithProfileDefault(true),
	)
	if err != nil {
		return nil, fmt.Errorf("build default oidc profile: %w", err)
	}
	// The user row and its default profile must commit together: a profile
	// failure after the user commits would leave the OIDC user with NO default
	// profile, and the next login takes the GetByOIDCSubject fast-path that
	// never heals it → access tokens minted with an empty ProfileID forever.
	// Wrap both in one tx (nil uow → sequential, for tests that build the
	// struct literal directly). Propagate the failure instead of swallowing it.
	create := func(ctx context.Context) error {
		if err := s.userRepo.Create(ctx, newUser); err != nil {
			return fmt.Errorf("create oidc user: %w", err)
		}
		if err := s.profiles.Create(ctx, p); err != nil {
			return fmt.Errorf("create default profile: %w", err)
		}
		return nil
	}
	var createErr error
	if s.uow != nil {
		createErr = s.uow.Do(ctx, create)
	} else {
		createErr = create(ctx)
	}
	if createErr != nil {
		// A concurrent first-login for the same (issuer, subject) can win the
		// race: the partial unique index idx_users_oidc_identity (migration
		// 000113) rejects our INSERT as a conflict. Recover by returning the row
		// the winner created rather than failing an otherwise-valid login.
		if errors.Is(createErr, shared.ErrConflict) {
			if existing, getErr := s.userRepo.GetByOIDCSubject(ctx, s.issuer, sub); getErr == nil {
				if !existing.Enabled {
					return nil, ErrUserDisabled
				}
				return existing, nil
			}
		}
		return nil, fmt.Errorf("OIDCService.findOrCreateUser: %w", createErr)
	}

	evt := domainuser.UserCreated{UserID: newUser.ID, Username: newUser.Username, OccurredAt: now}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "oidc: publish user.created", slog.Any("err", err))
	}

	return newUser, nil
}

func IsEmailAllowed(email string, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	for _, d := range allowedDomains {
		if strings.ToLower(d) == domain {
			return true
		}
	}
	return false
}

func (s *OIDCService) generateAndStoreRefreshToken(ctx context.Context, userID, profileID string) (string, error) {
	var boundProfile *shared.ProfileID
	if profileID != "" {
		pid := shared.ProfileID(profileID)
		boundProfile = &pid
	}
	for range maxRefreshTokenRetries {
		token, err := s.tokens.GenerateRefreshToken()
		if err != nil {
			return "", fmt.Errorf("generate refresh token: %w", err)
		}
		h := sha256.Sum256([]byte(token))
		rt := &domainuser.RefreshToken{
			ID:        id.New("rt-"),
			UserID:    shared.UserID(userID),
			TokenHash: hex.EncodeToString(h[:]),
			ProfileID: boundProfile,
			ExpiresAt: time.Now().Add(s.refreshTokenTTL),
			CreatedAt: time.Now(),
		}
		if err := s.refreshTkns.Create(ctx, rt); err == nil {
			return token, nil
		}
	}
	return "", errors.New("store refresh token: duplicate hash after retries")
}
