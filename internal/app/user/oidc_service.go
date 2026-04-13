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
	"strconv"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

var ErrOIDCDisabled = errors.New("OIDC not enabled")
var ErrEmailDomainNotAllowed = errors.New("email domain not allowed")
var ErrStateExpired = errors.New("OIDC state expired")

const oidcStateTTL = 10 * time.Minute

type OIDCConfig struct {
	Enabled             bool
	Issuer              string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	AllowedEmailDomains []string
}

type OIDCService struct {
	provider            *gooidc.Provider
	oauth2Cfg           oauth2.Config
	verifier            *gooidc.IDTokenVerifier
	userRepo            domainuser.UserRepository
	profiles            domainuser.ProfileRepository
	refreshTkns         domainuser.RefreshTokenRepository
	userRoles           domainuser.UserRoleRepository
	tokens              domainuser.TokenGenerator
	events              shared.EventBus
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
	refreshTokenTTL time.Duration,
) (*OIDCService, error) {
	if !cfg.Enabled {
		return nil, ErrOIDCDisabled
	}
	if cfg.ClientSecret == "" {
		return nil, errors.New("OIDC: client secret must not be empty")
	}
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
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
		userRepo:            userRepo,
		profiles:            profiles,
		refreshTkns:         refreshTkns,
		userRoles:           userRoles,
		tokens:              tokens,
		events:              events,
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
	return s.oauth2Cfg.AuthCodeURL(payload + "." + sig)
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
func (s *OIDCService) HandleCallback(ctx context.Context, code string) (accessToken, refreshToken, rawOIDCIDToken string, err error) {
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

	var claims struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if claimsErr := idToken.Claims(&claims); claimsErr != nil {
		return "", "", "", fmt.Errorf("parse id_token claims: %w", claimsErr)
	}

	u, err := s.findOrCreateUser(ctx, claims.Sub, claims.Email, claims.PreferredUsername)
	if err != nil {
		return "", "", "", fmt.Errorf("OIDCService.HandleCallback: %w", err)
	}

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.Warn("oidc: get roles", slog.String("userID", u.ID), slog.Any("err", err))
	}
	var perms []string
	for _, r := range roles {
		perms = append(perms, r.Permissions...)
	}

	var defaultProfileID string
	if profs, pErr := s.profiles.GetByUserID(ctx, u.ID); pErr == nil {
		for _, p := range profs {
			if p.IsDefault {
				defaultProfileID = p.ID
				break
			}
		}
		if defaultProfileID == "" && len(profs) > 0 {
			defaultProfileID = profs[0].ID
		}
	}

	var oidcLocale string
	if defaultProfileID != "" {
		if prof, pErr := s.profiles.GetByID(ctx, defaultProfileID); pErr == nil && prof != nil {
			oidcLocale = prof.Locale
		}
	}
	accessToken, err = s.tokens.GenerateAccessToken(domainuser.Claims{
		UserID:      u.ID,
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Locale:      oidcLocale,
		Permissions: perms,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, u.ID)
	if err != nil {
		return "", "", "", fmt.Errorf("OIDCService.HandleCallback: %w", err)
	}

	evt := domainuser.LoginSucceeded{UserID: u.ID, AuthMethod: "oidc", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("oidc: publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}

	return accessToken, refreshToken, rawIDToken, nil
}

func (s *OIDCService) findOrCreateUser(ctx context.Context, sub, email, preferredUsername string) (*domainuser.User, error) {
	u, err := s.userRepo.GetByOIDCSubject(ctx, s.issuer, sub)
	if err == nil {
		if !u.Enabled {
			return nil, ErrUserDisabled
		}
		u.UpdatedAt = time.Now()
		if updateErr := s.userRepo.Update(ctx, u); updateErr != nil {
			slog.Warn("oidc: update user timestamp", slog.String("user_id", u.ID), slog.Any("err", updateErr))
		}
		return u, nil
	}

	// first OIDC login — enforce email domain allowlist before creating user
	if !IsEmailAllowed(email, s.allowedEmailDomains) {
		slog.Warn("oidc: email domain not allowed", slog.String("email", email))
		return nil, ErrEmailDomainNotAllowed
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
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}

	p := &domainuser.Profile{
		ID:        id.New("prf-"),
		UserID:    newUser.ID,
		Name:      newUser.Username,
		IsDefault: true,
		CreatedAt: now,
	}
	if err := s.profiles.Create(ctx, p); err != nil {
		slog.Warn("oidc: create default profile failed", slog.Any("err", err))
	}

	evt := domainuser.UserCreated{UserID: newUser.ID, Username: newUser.Username, OccurredAt: now}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("oidc: publish user.created", slog.Any("err", err))
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

func (s *OIDCService) generateAndStoreRefreshToken(ctx context.Context, userID string) (string, error) {
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("oidc: purge old refresh tokens", slog.String("user_id", userID), slog.Any("err", err))
	}
	for range maxRefreshTokenRetries {
		token, err := s.tokens.GenerateRefreshToken()
		if err != nil {
			return "", fmt.Errorf("generate refresh token: %w", err)
		}
		h := sha256.Sum256([]byte(token))
		rt := &domainuser.RefreshToken{
			ID:        id.New("rt-"),
			UserID:    userID,
			TokenHash: hex.EncodeToString(h[:]),
			ExpiresAt: time.Now().Add(s.refreshTokenTTL),
			CreatedAt: time.Now(),
		}
		if err := s.refreshTkns.Create(ctx, rt); err == nil {
			return token, nil
		}
	}
	return "", errors.New("store refresh token: duplicate hash after retries")
}
