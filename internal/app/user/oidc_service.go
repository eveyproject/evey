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
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

var ErrOIDCDisabled = errors.New("OIDC not enabled")
var ErrEmailDomainNotAllowed = errors.New("email domain not allowed")

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
	cfg *config.Config,
	userRepo domainuser.UserRepository,
	profiles domainuser.ProfileRepository,
	refreshTkns domainuser.RefreshTokenRepository,
	userRoles domainuser.UserRoleRepository,
	tokens domainuser.TokenGenerator,
	events shared.EventBus,
	refreshTokenTTL time.Duration,
) (*OIDCService, error) {
	if !cfg.OIDC.Enabled {
		return nil, ErrOIDCDisabled
	}
	if cfg.OIDC.ClientSecret == "" {
		return nil, errors.New("OIDC: client secret must not be empty")
	}
	provider, err := gooidc.NewProvider(ctx, cfg.OIDC.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider discovery: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		RedirectURL:  cfg.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.OIDC.ClientID})

	// derive state signing secret from client secret
	h := sha256.Sum256([]byte(cfg.OIDC.ClientSecret + "oidc-state"))

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
		issuer:              cfg.OIDC.Issuer,
		allowedEmailDomains: cfg.OIDC.AllowedEmailDomains,
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
func (s *OIDCService) AuthURL(state string) string {
	sig := s.signState(state)
	return s.oauth2Cfg.AuthCodeURL(state + "." + sig)
}

// ValidateState checks the signed state from the callback.
func (s *OIDCService) ValidateState(rawState string) (string, error) {
	parts := strings.SplitN(rawState, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid state")
	}
	nonce, sig := parts[0], parts[1]
	expected := s.signState(nonce)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", errors.New("state signature mismatch")
	}
	return nonce, nil
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
	newUser := &domainuser.User{
		ID:          id.New("usr-"),
		Username:    username,
		Email:       email,
		IsAdmin:     false,
		Enabled:     true,
		OIDCSubject: &sub,
		OIDCIssuer:  &issuer,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
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
