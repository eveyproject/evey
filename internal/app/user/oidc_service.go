package user

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

var ErrOIDCDisabled = errors.New("OIDC not enabled")
var ErrEmailDomainNotAllowed = errors.New("email domain not allowed")
var ErrStateExpired = errors.New("OIDC state expired")

// ErrOIDCUnavailable: SSO is switched on but the provider cannot be used
// (incomplete settings, discovery failing). Local login is unaffected.
var ErrOIDCUnavailable = errors.New("OIDC provider unavailable")

// ErrOIDCSignupDisabled: the identity is unknown and the policy forbids
// creating accounts for new SSO users.
var ErrOIDCSignupDisabled = errors.New("OIDC sign-up disabled")

// ErrOIDCAccountExists: a local account already uses the e-mail and cannot be
// linked (unverified e-mail, linking off, or already tied to another identity).
var ErrOIDCAccountExists = errors.New("an account already uses this e-mail")

const oidcStateTTL = 10 * time.Minute

// OIDCStateTTL is how long a login may take; the login cookie lives as long.
const OIDCStateTTL = oidcStateTTL

// oidcHTTPTimeout bounds every outbound call to the IdP (discovery, token
// exchange, JWKS fetch). Without it those calls ride http.DefaultClient
// (Timeout=0, no ResponseHeaderTimeout) and a hung issuer stalls login-callback
// handler goroutines. A plain timeout, NOT the ssrf.SafeHTTPClient: the issuer
// is ADMIN-configured (trusted operator input), and self-hosted IdPs on private
// LAN addresses are a first-class deployment for a self-hosted media server -
// the SSRF dialer would block exactly those.
const oidcHTTPTimeout = 15 * time.Second

// oidcRetryBackoff spaces discovery attempts after a failure, so a provider
// that is down costs one outbound call per window, not one per page load.
const oidcRetryBackoff = 30 * time.Second

// OIDCConfig is the resolved SSO configuration: connection plus policy.
type OIDCConfig struct {
	Enabled             bool
	Issuer              string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	AllowedEmailDomains []string
	ButtonLabel         string
	// LinkByEmail attaches a first SSO login to the local account holding the
	// same e-mail, only when the provider asserts email_verified.
	LinkByEmail bool
	// AllowSignup creates accounts for unknown identities; AllowSignupDefault
	// is its value when nobody chose (on only behind a domain allowlist).
	AllowSignup        bool
	AllowSignupDefault bool
	// DefaultLibraryIDs restricts new SSO accounts; empty = every library.
	DefaultLibraryIDs     []shared.LibraryID
	PostLogoutRedirectURL string
	FromPanel             bool
}

// MissingField names the first connection setting left empty, or "".
func (c OIDCConfig) MissingField() string {
	switch {
	case c.Issuer == "":
		return "issuer"
	case c.ClientID == "":
		return "client_id"
	case c.ClientSecret == "":
		return "client_secret"
	case c.RedirectURL == "":
		return "redirect_url"
	}
	return ""
}

// connectionKey fingerprints what discovery and the OAuth client depend on;
// policy changes keep the discovered provider.
func (c OIDCConfig) connectionKey() string {
	h := sha256.Sum256([]byte(c.Issuer + "\x00" + c.ClientID + "\x00" + c.ClientSecret + "\x00" + c.RedirectURL))
	return hex.EncodeToString(h[:])
}

// oidcClient is one discovered provider and the OAuth client built on it.
type oidcClient struct {
	cfg       OIDCConfig
	provider  *gooidc.Provider
	oauth2Cfg oauth2.Config
	verifier  *gooidc.IDTokenVerifier
}

// OIDCStatus is what the admin panel reports about the provider.
type OIDCStatus struct {
	// State: "disabled", "incomplete", "ok" or "error".
	State              string
	Error              string
	CheckedAt          time.Time
	EndSessionEndpoint string
}

type OIDCService struct {
	// httpClient carries the bounded-timeout client every IdP call must use
	// (see oidcHTTPTimeout). nil (hand-built test instances) falls back to the
	// caller's ctx client / http.DefaultClient.
	httpClient      *http.Client
	userRepo        domainuser.UserRepository
	profiles        domainuser.ProfileRepository
	refreshTkns     domainuser.RefreshTokenRepository
	userRoles       domainuser.UserRoleRepository
	libraryAccess   domainuser.LibraryAccessRepository
	tokens          domainuser.TokenGenerator
	events          shared.EventBus
	uow             shared.TransactionalUnit
	stateSecret     []byte
	refreshTokenTTL time.Duration

	// settings resolves the live configuration (panel, then env). nil = the
	// client in cur is fixed (hand-built test instances).
	settings func(ctx context.Context) (OIDCConfig, error)

	discoverMu sync.Mutex // one discovery at a time
	mu         sync.Mutex // guards the fields below
	cur        *oidcClient
	curKey     string
	failKey    string
	failErr    error
	checkedAt  time.Time
}

// OIDCOption configures optional dependencies.
type OIDCOption func(*OIDCService)

// WithOIDCLibraryAccess enables the default library access of new accounts.
func WithOIDCLibraryAccess(la domainuser.LibraryAccessRepository) OIDCOption {
	return func(s *OIDCService) { s.libraryAccess = la }
}

// NewOIDCService never contacts the provider: discovery is lazy and retried,
// so an unreachable IdP disables the SSO button instead of the server.
func NewOIDCService(
	settings func(ctx context.Context) (OIDCConfig, error),
	userRepo domainuser.UserRepository,
	profiles domainuser.ProfileRepository,
	refreshTkns domainuser.RefreshTokenRepository,
	userRoles domainuser.UserRoleRepository,
	tokens domainuser.TokenGenerator,
	events shared.EventBus,
	uow shared.TransactionalUnit,
	refreshTokenTTL time.Duration,
	opts ...OIDCOption,
) (*OIDCService, error) {
	// The state is signed with a per-process key: independent of the client
	// secret (which the panel can now change) and never stored anywhere.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("oidc state secret: %w", err)
	}
	s := &OIDCService{
		httpClient:      &http.Client{Timeout: oidcHTTPTimeout},
		userRepo:        userRepo,
		profiles:        profiles,
		refreshTkns:     refreshTkns,
		userRoles:       userRoles,
		tokens:          tokens,
		events:          events,
		uow:             uow,
		stateSecret:     secret,
		refreshTokenTTL: refreshTokenTTL,
		settings:        settings,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Config returns the configuration in force now.
func (s *OIDCService) Config(ctx context.Context) (OIDCConfig, error) {
	if s.settings == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cur == nil {
			return OIDCConfig{}, nil
		}
		return s.cur.cfg, nil
	}
	return s.settings(ctx)
}

// client returns a usable provider for the current settings, discovering it
// on first use and after a failure once the backoff has elapsed.
func (s *OIDCService) client(ctx context.Context) (*oidcClient, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCUnavailable, err)
	}
	return s.clientFor(ctx, cfg, false)
}

func (s *OIDCService) clientFor(ctx context.Context, cfg OIDCConfig, force bool) (*oidcClient, error) {
	if !cfg.Enabled {
		return nil, ErrOIDCDisabled
	}
	if s.settings == nil { // fixed client (tests)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cur == nil {
			return nil, ErrOIDCUnavailable
		}
		return s.cur, nil
	}
	if f := cfg.MissingField(); f != "" {
		return nil, fmt.Errorf("%w: %s is not set", ErrOIDCUnavailable, f)
	}
	key := cfg.connectionKey()
	if c, ok, err := s.cached(cfg, key, force); ok {
		return c, err
	}
	s.discoverMu.Lock()
	defer s.discoverMu.Unlock()
	// Another caller may have finished the same discovery while we waited.
	if !force {
		if c, ok, err := s.cached(cfg, key, false); ok {
			return c, err
		}
	}
	// Detached: the result is shared, so one caller hanging up must not record
	// a failure for everybody (httpClient still bounds the call).
	built, err := s.discover(context.WithoutCancel(ctx), cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkedAt = time.Now()
	if err != nil {
		s.failKey, s.failErr = key, err
		slog.WarnContext(ctx, "oidc: provider discovery failed", slog.String("issuer", cfg.Issuer), slog.Any("err", err))
		return nil, fmt.Errorf("%w: %w", ErrOIDCUnavailable, err)
	}
	s.cur, s.curKey, s.failKey, s.failErr = built, key, "", nil
	return built, nil
}

// cached answers from the last discovery when it is for the same connection
// settings; ok=false means a discovery must run.
func (s *OIDCService) cached(cfg OIDCConfig, key string, force bool) (*oidcClient, bool, error) {
	if force {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.curKey == key {
		c := *s.cur
		c.cfg = cfg // policy fields are read live
		return &c, true, nil
	}
	if s.failKey == key && s.failErr != nil && time.Since(s.checkedAt) < oidcRetryBackoff {
		return nil, true, fmt.Errorf("%w: %w", ErrOIDCUnavailable, s.failErr)
	}
	return nil, false, nil
}

func (s *OIDCService) discover(ctx context.Context, cfg OIDCConfig) (*oidcClient, error) {
	provider, err := gooidc.NewProvider(gooidc.ClientContext(ctx, s.httpClient), cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("provider discovery: %w", err)
	}
	return &oidcClient{
		cfg:      cfg,
		provider: provider,
		oauth2Cfg: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

// Invalidate drops the discovered provider and the failure backoff, so the
// next use re-reads the settings and discovers again.
func (s *OIDCService) Invalidate() {
	if s.settings == nil {
		return
	}
	s.mu.Lock()
	s.cur, s.curKey, s.failKey, s.failErr = nil, "", "", nil
	s.mu.Unlock()
}

// Status reports the configuration and the provider state, discovering when
// needed (or always, with refresh).
func (s *OIDCService) Status(ctx context.Context, refresh bool) (OIDCConfig, OIDCStatus) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return cfg, OIDCStatus{State: "error", Error: err.Error(), CheckedAt: time.Now()}
	}
	if !cfg.Enabled {
		return cfg, OIDCStatus{State: "disabled"}
	}
	if f := cfg.MissingField(); f != "" && s.settings != nil {
		return cfg, OIDCStatus{State: "incomplete", Error: f + " is not set"}
	}
	c, err := s.clientFor(ctx, cfg, refresh)
	s.mu.Lock()
	checked := s.checkedAt
	s.mu.Unlock()
	if err != nil {
		return cfg, OIDCStatus{State: "error", Error: strings.TrimPrefix(err.Error(), ErrOIDCUnavailable.Error()+": "), CheckedAt: checked}
	}
	return cfg, OIDCStatus{State: "ok", CheckedAt: checked, EndSessionEndpoint: endSessionOf(c.provider)}
}

// Probe runs discovery against issuer without touching the cache, for the
// panel's "Test connection" on values not saved yet.
func (s *OIDCService) Probe(ctx context.Context, issuer string) (string, error) {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		return "", errors.New("issuer is not set")
	}
	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: oidcHTTPTimeout}
	}
	provider, err := gooidc.NewProvider(gooidc.ClientContext(ctx, client), issuer)
	if err != nil {
		return "", err
	}
	return endSessionOf(provider), nil
}

// Available reports whether the login page should offer SSO, and its label.
func (s *OIDCService) Available(ctx context.Context) (bool, string) {
	c, err := s.client(ctx)
	if err != nil {
		return false, ""
	}
	return true, c.cfg.ButtonLabel
}

// EndSessionEndpoint returns the OIDC end_session_endpoint from provider discovery, or "".
func (s *OIDCService) EndSessionEndpoint(ctx context.Context) string {
	c, err := s.client(ctx)
	if err != nil {
		return ""
	}
	return endSessionOf(c.provider)
}

// PostLogoutRedirectURL is the post_logout_redirect_uri registered at the provider.
func (s *OIDCService) PostLogoutRedirectURL(ctx context.Context) string {
	cfg, err := s.Config(ctx)
	if err != nil {
		return ""
	}
	return cfg.PostLogoutRedirectURL
}

func endSessionOf(p *gooidc.Provider) string {
	var claims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if p == nil || p.Claims(&claims) != nil {
		return ""
	}
	return claims.EndSessionEndpoint
}

// ErrStateUnbound: the callback's state was not issued to this browser (its
// PKCE verifier cookie is missing or belongs to another login).
var ErrStateUnbound = errors.New("OIDC state not bound to this browser")

// pkceChallenge is the S256 code_challenge of a PKCE verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthURL returns the authorization URL for a login whose PKCE verifier the
// caller keeps on the browser (never in the URL). The signed state is
// "ts|nonce|challenge.sig": TTL, the caller's nonce and the browser binding.
func (s *OIDCService) AuthURL(ctx context.Context, nonce, verifier string) (string, error) {
	if verifier == "" || strings.Contains(nonce, "|") {
		return "", errors.New("oidc: auth url needs a verifier and a nonce without '|'")
	}
	c, err := s.client(ctx)
	if err != nil {
		return "", err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := ts + "|" + nonce + "|" + pkceChallenge(verifier)
	sig := s.signState(payload)
	// `nonce` also goes out as the OpenID Connect nonce, checked in the id_token
	// by HandleCallback. A provider without PKCE ignores the challenge.
	return c.oauth2Cfg.AuthCodeURL(payload+"."+sig,
		oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// ValidateState checks signature and TTL and returns the nonce. It does NOT
// prove the callback comes from the browser that logged in: only use it to
// route an error; a sign-in goes through ValidateBoundState.
func (s *OIDCService) ValidateState(rawState string) (string, error) {
	nonce, _, err := s.parseState(rawState)
	return nonce, err
}

// ValidateBoundState is ValidateState plus the browser binding: verifier (from
// the login cookie) must hash to the challenge signed into the state.
func (s *OIDCService) ValidateBoundState(rawState, verifier string) (string, error) {
	nonce, challenge, err := s.parseState(rawState)
	if err != nil {
		return "", err
	}
	if verifier == "" || challenge == "" || !hmac.Equal([]byte(pkceChallenge(verifier)), []byte(challenge)) {
		return nonce, ErrStateUnbound
	}
	return nonce, nil
}

// parseState accepts "ts|nonce[|challenge].sig", rejecting states older than
// oidcStateTTL.
func (s *OIDCService) parseState(rawState string) (nonce, challenge string, err error) {
	parts := strings.SplitN(rawState, ".", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid state")
	}
	payload, sig := parts[0], parts[1]
	expected := s.signState(payload)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", "", errors.New("state signature mismatch")
	}

	tsParts := strings.SplitN(payload, "|", 3)
	if len(tsParts) < 2 {
		return "", "", errors.New("invalid state")
	}
	tsUnix, err := strconv.ParseInt(tsParts[0], 10, 64)
	if err != nil {
		return "", "", errors.New("invalid state")
	}
	if time.Since(time.Unix(tsUnix, 0)) > oidcStateTTL {
		return "", "", ErrStateExpired
	}
	if len(tsParts) == 3 {
		challenge = tsParts[2]
	}
	return tsParts[1], challenge, nil
}

func (s *OIDCService) signState(nonce string) string {
	mac := hmac.New(sha256.New, s.stateSecret)
	mac.Write([]byte(nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// oidcIdentity is what the id_token asserts about the person signing in.
type oidcIdentity struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
}

// HandleCallback exchanges the code with the login's PKCE verifier, verifies
// the ID token, finds/creates the user, and returns Evey access + refresh
// tokens plus the raw OIDC id_token for RP-Initiated Logout.
func (s *OIDCService) HandleCallback(ctx context.Context, code, expectedNonce, verifier string) (accessToken, refreshToken, rawOIDCIDToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.HandleCallback")
	defer span.End()
	if verifier == "" {
		return "", "", "", errors.New("oidc callback: missing PKCE verifier")
	}
	c, err := s.client(ctx)
	if err != nil {
		return "", "", "", err
	}
	// Bound the token exchange and the verifier's JWKS fetch: without the
	// injected client both ride http.DefaultClient (no timeout) on a request
	// ctx the server never deadlines (no ReadTimeout/TimeoutHandler), so a
	// slow IdP stalls login handler goroutines indefinitely. nil-safe for
	// hand-built test instances.
	if s.httpClient != nil {
		ctx = gooidc.ClientContext(ctx, s.httpClient)                 // JWKS (verifier.Verify)
		ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient) // token exchange
	}
	oauth2Token, err := c.oauth2Cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", "", "", fmt.Errorf("token exchange: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return "", "", "", errors.New("missing id_token in response")
	}

	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", "", fmt.Errorf("id_token verification: %w", err)
	}
	// The id_token's nonce must match the one we sent in AuthURL (carried in the
	// signed state). A mismatch means this id_token was not minted for this login
	// request - reject it (blocks replay/injection of a token obtained elsewhere).
	if idToken.Nonce != expectedNonce {
		return "", "", "", errors.New("id_token nonce mismatch")
	}

	var claims oidcIdentity
	if claimsErr := idToken.Claims(&claims); claimsErr != nil {
		return "", "", "", fmt.Errorf("parse id_token claims: %w", claimsErr)
	}

	u, err := s.findOrCreateUser(ctx, c.cfg, claims)
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

// findOrCreateUser resolves the identity in this order: an account already
// linked to (issuer, sub); else a local account linked by verified e-mail;
// else a new account when the sign-up policy allows it.
func (s *OIDCService) findOrCreateUser(ctx context.Context, cfg OIDCConfig, ident oidcIdentity) (*domainuser.User, error) {
	u, err := s.userRepo.GetByOIDCSubject(ctx, cfg.Issuer, ident.Sub)
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
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("lookup oidc identity: %w", err)
	}

	email := strings.TrimSpace(ident.Email)
	// When an allowlist is configured the email is a trust boundary, so require
	// the IdP to have verified it: a lax provider could otherwise assert an
	// arbitrary address in an allowed domain to slip past the allowlist.
	if len(cfg.AllowedEmailDomains) > 0 && !ident.EmailVerified {
		slog.WarnContext(ctx, "oidc: unverified email rejected under domain allowlist", slog.String("email", email))
		return nil, ErrEmailDomainNotAllowed
	}
	if !IsEmailAllowed(email, cfg.AllowedEmailDomains) {
		slog.WarnContext(ctx, "oidc: email domain not allowed", slog.String("email", email))
		return nil, ErrEmailDomainNotAllowed
	}
	// An IdP claim that is not an address must not lock the user out of SSO:
	// the account is created without an email rather than not at all.
	if email != "" && !domainuser.IsPlainAddress(email) {
		slog.WarnContext(ctx, "oidc: ignoring malformed email claim", slog.String("email", email))
		email = ""
	}

	if email != "" {
		existing, lookupErr := s.userRepo.GetByEmail(ctx, email)
		switch {
		case lookupErr == nil:
			return s.linkExisting(ctx, cfg, ident, existing)
		case !errors.Is(lookupErr, shared.ErrNotFound):
			return nil, fmt.Errorf("lookup account by email: %w", lookupErr)
		}
	}

	if !cfg.AllowSignup {
		slog.InfoContext(ctx, "oidc: unknown identity refused, sign-up disabled", slog.String("email", email))
		return nil, ErrOIDCSignupDisabled
	}
	return s.createUser(ctx, cfg, ident, email)
}

// linkExisting ties a first SSO login to the local account with the same
// e-mail. Only on a provider-verified e-mail: otherwise anybody able to type
// an address at the provider would take over the account holding it.
func (s *OIDCService) linkExisting(ctx context.Context, cfg OIDCConfig, ident oidcIdentity, u *domainuser.User) (*domainuser.User, error) {
	if !cfg.LinkByEmail || !ident.EmailVerified {
		slog.WarnContext(ctx, "oidc: e-mail matches a local account, not linked",
			slog.String("user_id", string(u.ID)), slog.Bool("email_verified", ident.EmailVerified), slog.Bool("link_enabled", cfg.LinkByEmail))
		return nil, ErrOIDCAccountExists
	}
	if !u.Enabled {
		return nil, ErrUserDisabled
	}
	if u.OIDCSubject != nil {
		slog.WarnContext(ctx, "oidc: e-mail matches an account linked to another identity", slog.String("user_id", string(u.ID)))
		return nil, ErrOIDCAccountExists
	}
	// Never by e-mail alone for an administrator: a misconfigured provider must
	// not be enough to take over an admin account.
	if u.IsAdmin {
		slog.WarnContext(ctx, "oidc: e-mail matches an administrator account, not linked automatically", slog.String("user_id", string(u.ID)))
		return nil, ErrOIDCAccountExists
	}
	if err := s.userRepo.LinkOIDCIdentity(ctx, u.ID, cfg.Issuer, ident.Sub); err != nil {
		if errors.Is(err, shared.ErrConflict) {
			return nil, ErrOIDCAccountExists
		}
		return nil, fmt.Errorf("link oidc identity: %w", err)
	}
	slog.InfoContext(ctx, "oidc: identity linked to existing account by verified e-mail",
		slog.String("user_id", string(u.ID)), slog.String("issuer", cfg.Issuer))
	iss, sub := cfg.Issuer, ident.Sub
	u.OIDCIssuer, u.OIDCSubject = &iss, &sub
	return u, nil
}

// oidcUsernameCandidates derives the usernames to try, in order: the claim,
// then suffixes taken from a hash of the identity, so the same person always
// gets the same fallback name.
func oidcUsernameCandidates(issuer, sub, preferred, email string) []string {
	base := strings.TrimSpace(preferred)
	if base == "" && email != "" {
		if at := strings.Index(email, "@"); at > 0 {
			base = email[:at]
		}
	}
	if base == "" {
		base = "oidc-" + hex.EncodeToString([]byte(sub))[:8]
	}
	h := sha256.Sum256([]byte(issuer + "\x00" + sub))
	tag := hex.EncodeToString(h[:])
	return []string{base, base + "-" + tag[:4], base + "-" + tag[:8], "oidc-" + tag[:12]}
}

func (s *OIDCService) createUser(ctx context.Context, cfg OIDCConfig, ident oidcIdentity, email string) (*domainuser.User, error) {
	sub := ident.Sub
	issuer := cfg.Issuer
	var lastErr error
	for _, username := range oidcUsernameCandidates(issuer, sub, ident.PreferredUsername, email) {
		if _, err := s.userRepo.GetByUsername(ctx, username); err == nil {
			continue // taken: try the next deterministic candidate
		}
		u, err := s.insertUser(ctx, cfg, username, email, sub)
		if err == nil {
			return u, nil
		}
		lastErr = err
		if !errors.Is(err, shared.ErrConflict) {
			return nil, err
		}
		// A concurrent first-login for the same (issuer, subject) can win the
		// race: the partial unique index idx_users_oidc_identity (migration
		// 000113) rejects our INSERT as a conflict. Recover by returning the row
		// the winner created rather than failing an otherwise-valid login.
		if existing, getErr := s.userRepo.GetByOIDCSubject(ctx, issuer, sub); getErr == nil {
			if !existing.Enabled {
				return nil, ErrUserDisabled
			}
			return existing, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: no free username", shared.ErrConflict)
	}
	return nil, fmt.Errorf("OIDCService.findOrCreateUser: %w", lastErr)
}

func (s *OIDCService) insertUser(ctx context.Context, cfg OIDCConfig, username, email, sub string) (*domainuser.User, error) {
	issuer := cfg.Issuer
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
	libs := cfg.DefaultLibraryIDs
	if len(libs) > 0 && s.libraryAccess == nil {
		return nil, errors.New("create oidc user: library restriction configured but no access store")
	}
	// The user row, its default profile and its library grants commit
	// together: a profile failure after the user commits would leave the OIDC
	// user with NO default profile, and a missing grant would leave it seeing
	// every library. nil uow = sequential, for tests building the struct.
	create := func(ctx context.Context) error {
		if uErr := s.userRepo.Create(ctx, newUser); uErr != nil {
			return fmt.Errorf("create oidc user: %w", uErr)
		}
		if pErr := s.profiles.Create(ctx, p); pErr != nil {
			return fmt.Errorf("create default profile: %w", pErr)
		}
		if len(libs) == 0 {
			return nil
		}
		if gErr := s.libraryAccess.GrantMany(ctx, newUser.ID, libs); gErr != nil {
			return fmt.Errorf("grant default libraries: %w", gErr)
		}
		// GrantMany skips deleted libraries; zero grants means "everything",
		// so a selection that no longer exists must refuse, not widen.
		granted, rErr := s.libraryAccess.GetByUserID(ctx, newUser.ID)
		if rErr != nil {
			return fmt.Errorf("read default libraries: %w", rErr)
		}
		if len(granted) == 0 {
			return errors.New("create oidc user: none of the default libraries exists any more")
		}
		return nil
	}
	if s.uow != nil {
		err = s.uow.Do(ctx, create)
	} else {
		err = create(ctx)
	}
	if err != nil {
		return nil, err
	}
	evt := domainuser.UserCreated{UserID: newUser.ID, Username: newUser.Username, OccurredAt: time.Now()}
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
