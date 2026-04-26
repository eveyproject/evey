package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	loginMaxAttempts   = 5
	loginAttemptWindow = 15 * time.Minute
)

// safeTokenPrefix returns the first 8 chars of a token (safe for logs).
func safeTokenPrefix(token string) string {
	if len(token) < 8 {
		return "<short>"
	}
	return token[:8] + "…"
}

// CommandService handles auth, setup and token management.
type CommandService struct {
	users           user.UserRepository
	profiles        user.ProfileRepository
	refreshTkns     user.RefreshTokenRepository
	resetTokens     user.PasswordResetTokenRepository
	roles           user.RoleRepository
	userRoles       user.UserRoleRepository
	hasher          user.PasswordHasher
	tokens          user.TokenGenerator
	events          shared.EventBus
	quickConnect    user.QuickConnectRepository
	serverCfg       shared.ServerConfigRepository
	smtpConfigured  bool
	refreshTokenTTL time.Duration
	loginAttempts   user.LoginAttemptRepository
	loginMaxFails   int
	loginWindow     time.Duration
	setupMu         sync.Mutex
	setupDone       bool
	uow             shared.TransactionalUnit
}

func NewCommandService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	refreshTkns user.RefreshTokenRepository,
	roles user.RoleRepository,
	userRoles user.UserRoleRepository,
	hasher user.PasswordHasher,
	tokens user.TokenGenerator,
	events shared.EventBus,
	refreshTokenTTL time.Duration,
	quickConnect user.QuickConnectRepository,
	opts ...CommandServiceOption,
) *CommandService {
	s := &CommandService{
		users:           users,
		profiles:        profiles,
		refreshTkns:     refreshTkns,
		roles:           roles,
		userRoles:       userRoles,
		hasher:          hasher,
		tokens:          tokens,
		events:          events,
		refreshTokenTTL: refreshTokenTTL,
		quickConnect:    quickConnect,
		loginAttempts:   newLoginAttemptLimiter(loginMaxAttempts, loginAttemptWindow),
		loginMaxFails:   loginMaxAttempts,
		loginWindow:     loginAttemptWindow,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type CommandServiceOption func(*CommandService)

func WithPasswordReset(resetTokens user.PasswordResetTokenRepository, serverCfg shared.ServerConfigRepository, smtpConfigured bool) CommandServiceOption {
	return func(s *CommandService) {
		s.resetTokens = resetTokens
		s.serverCfg = serverCfg
		s.smtpConfigured = smtpConfigured
	}
}

// WithUnitOfWork wires a transactional unit so multi-step writes (e.g.
// password reset: user update + token mark-used) commit/rollback atomically.
func WithUnitOfWork(uow shared.TransactionalUnit) CommandServiceOption {
	return func(s *CommandService) { s.uow = uow }
}

// WithLoginAttemptRepository replaces the default in-memory rate limiter with
// a persistent one (e.g. postgres) so lockout state survives restarts and is
// shared across instances.
func WithLoginAttemptRepository(repo user.LoginAttemptRepository) CommandServiceOption {
	return func(s *CommandService) { s.loginAttempts = repo }
}

func (s *CommandService) InitSetupState(ctx context.Context) {
	count, err := s.users.CountAdmins(ctx)
	if err == nil && count > 0 {
		s.setupDone = true
	}
}

// SetupOption configures optional inputs for the initial admin Setup.
type SetupOption func(*setupOptions)

type setupOptions struct {
	locale string
	region string
}

// WithSetupLocale seeds the admin default profile with the language the user
// picked in the setup wizard, so the JWT locale claim and subsequent sessions
// stay in that language instead of falling back to the DB column default.
func WithSetupLocale(locale string) SetupOption {
	return func(o *setupOptions) { o.locale = locale }
}

// WithSetupRegion seeds the admin default profile with an ISO-3166 region.
// Empty string is ignored (region stays nil → server fallback at runtime).
func WithSetupRegion(region string) SetupOption {
	return func(o *setupOptions) { o.region = region }
}

func (s *CommandService) Setup(ctx context.Context, username, password, email string, opts ...SetupOption) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Setup")
	defer span.End()
	var cfg setupOptions
	for _, fn := range opts {
		fn(&cfg)
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupDone {
		return nil, ErrSetupDone
	}
	if username == "" {
		return nil, errors.New("username is required")
	}
	if len(password) < 8 {
		return nil, ErrPasswordTooShort
	}
	admins, err := s.users.CountAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	if admins > 0 {
		s.setupDone = true
		return nil, ErrSetupDone
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := user.NewUser(username, email, hash)
	if err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	u.IsAdmin = true
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	evtSetupCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: true, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtSetupCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtSetupCreated.EventName()), slog.Any("err", err))
	}

	p := &user.Profile{
		ID:        shared.ProfileID(id.New("prf-")),
		UserID:    u.ID,
		Name:      u.Username,
		IsDefault: true,
		Locale:    cfg.locale,
	}
	if cfg.region != "" {
		r := cfg.region
		p.Region = &r
	}
	if err := s.profiles.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create default profile: %w", err)
	}
	evtProfileCreated := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfileCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
	}

	s.setupDone = true
	return u, nil
}

func (s *CommandService) Login(ctx context.Context, username, password string, ttlOverride ...time.Duration) (accessToken, refreshToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.Login")
	defer span.End()
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	locked, lockErr := s.loginAttempts.IsLocked(ctx, username)
	if lockErr != nil {
		slog.Warn("login attempt IsLocked", slog.String("username", username), slog.Any("err", lockErr))
	}
	if locked {
		return "", "", ErrAccountLocked
	}

	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if recErr := s.loginAttempts.Record(ctx, username, s.loginMaxFails, s.loginWindow); recErr != nil {
			slog.Warn("login attempt Record", slog.String("username", username), slog.Any("err", recErr))
		}
		return "", "", ErrInvalidCredentials
	}
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}

	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if err != nil || !ok {
		if recErr := s.loginAttempts.Record(ctx, username, s.loginMaxFails, s.loginWindow); recErr != nil {
			slog.Warn("login attempt Record", slog.String("username", username), slog.Any("err", recErr))
		}
		return "", "", ErrInvalidCredentials
	}
	if resetErr := s.loginAttempts.Reset(ctx, username); resetErr != nil {
		slog.Warn("login attempt Reset", slog.String("username", username), slog.Any("err", resetErr))
	}

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", string(u.ID)), slog.Any("err", err))
	}
	var perms []string
	for _, r := range roles {
		perms = append(perms, r.Permissions...)
	}

	var defaultProfileID string
	if profs, pErr := s.profiles.GetByUserID(ctx, u.ID); pErr == nil {
		for _, p := range profs {
			if p.IsDefault {
				defaultProfileID = string(p.ID)
				break
			}
		}
		if defaultProfileID == "" && len(profs) > 0 {
			defaultProfileID = string(profs[0].ID)
		}
	}
	var profileLocale string
	if defaultProfileID != "" {
		if prof, pErr := s.profiles.GetByID(ctx, shared.ProfileID(defaultProfileID)); pErr == nil && prof != nil {
			profileLocale = prof.Locale
		}
	}

	accessToken, err = s.tokens.GenerateAccessToken(user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Locale:      profileLocale,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.Login: %w", err)
	}

	evtLogin := user.LoginSucceeded{UserID: u.ID, AuthMethod: "local", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtLogin); err != nil {
		slog.Warn("publish event", slog.String("event", evtLogin.EventName()), slog.Any("err", err))
	}
	return accessToken, refreshToken, nil
}

func (s *CommandService) GenerateTokensForUser(ctx context.Context, userID string, ttlOverride ...time.Duration) (accessToken, refreshToken string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.GenerateTokensForUser")
	defer span.End()
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return "", "", fmt.Errorf("user not found: %w", err)
	}
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", string(u.ID)), slog.Any("err", err))
	}
	var perms []string
	for _, r := range roles {
		perms = append(perms, r.Permissions...)
	}

	var defaultProfileID string
	if profs, pErr := s.profiles.GetByUserID(ctx, u.ID); pErr == nil {
		for _, p := range profs {
			if p.IsDefault {
				defaultProfileID = string(p.ID)
				break
			}
		}
		if defaultProfileID == "" && len(profs) > 0 {
			defaultProfileID = string(profs[0].ID)
		}
	}
	var profileLocale string
	if defaultProfileID != "" {
		if prof, pErr := s.profiles.GetByID(ctx, shared.ProfileID(defaultProfileID)); pErr == nil && prof != nil {
			profileLocale = prof.Locale
		}
	}

	accessToken, err = s.tokens.GenerateAccessToken(user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Locale:      profileLocale,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, string(u.ID), ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.GenerateTokensForUser: %w", err)
	}
	return accessToken, refreshToken, nil
}

// RefreshToken rotates the refresh token, returning a new access+refresh pair.
// requestedProfileID (optional) activates a specific profile on the new access
// token; must belong to the refreshing user or the refresh is rejected.
func (s *CommandService) RefreshToken(ctx context.Context, token, requestedProfileID string, ttlOverride ...time.Duration) (string, string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.RefreshToken")
	defer span.End()
	rt, err := s.claimRefreshToken(ctx, token)
	if err != nil {
		return "", "", err
	}

	u, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}
	if !u.Enabled {
		return "", "", ErrUserDisabled
	}

	perms := s.collectPermissions(ctx, u.ID)

	profs, pErr := s.profiles.GetByUserID(ctx, u.ID)
	if pErr != nil {
		slog.Warn("get profiles by user failed", slog.String("userID", string(u.ID)), slog.Any("err", pErr))
	}
	activeProfileID, ok := resolveActiveProfileID(profs, requestedProfileID)
	if !ok {
		// Caller asked for a profile that doesn't belong to them — reject.
		return "", "", ErrInvalidCredentials
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   activeProfileID,
		Locale:      s.lookupProfileLocale(ctx, activeProfileID),
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}

	newRefresh, err := s.generateAndStoreRefreshToken(ctx, string(u.ID), ttlOverride...)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}

	return accessToken, newRefresh, nil
}

// claimRefreshToken performs the full hash-lookup + reuse/expiry gating +
// atomic claim (MarkUsedIfNotUsed). Any failure path returns
// ErrInvalidCredentials; reuse and concurrent-claim races purge all tokens
// for the owning user to limit blast radius.
func (s *CommandService) claimRefreshToken(ctx context.Context, token string) (*user.RefreshToken, error) {
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])

	rt, err := s.refreshTkns.GetByTokenHash(ctx, hash)
	if err != nil {
		slog.Info("refresh rejected: token not found", slog.String("tokenPrefix", safeTokenPrefix(token)))
		return nil, ErrInvalidCredentials
	}

	if rt.Used {
		slog.Warn("refresh rejected: token reuse detected, purging all user tokens",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID))
		if delErr := s.refreshTkns.DeleteByUserID(ctx, rt.UserID); delErr != nil {
			slog.Warn("delete refresh tokens by user failed", slog.String("userID", string(rt.UserID)), slog.Any("err", delErr))
		}
		return nil, ErrInvalidCredentials
	}

	if rt.IsExpired() {
		slog.Info("refresh rejected: token expired",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID),
			slog.Time("expiredAt", rt.ExpiresAt))
		if delErr := s.refreshTkns.DeleteByID(ctx, rt.ID); delErr != nil {
			slog.Warn("delete refresh token failed", slog.String("tokenID", rt.ID), slog.Any("err", delErr))
		}
		return nil, ErrInvalidCredentials
	}

	claimed, markErr := s.refreshTkns.MarkUsedIfNotUsed(ctx, rt.ID)
	if markErr != nil {
		return nil, fmt.Errorf("CommandService.RefreshToken: %w", markErr)
	}
	if !claimed {
		slog.Warn("refresh rejected: concurrent claim race, purging all user tokens",
			slog.String("userID", string(rt.UserID)),
			slog.String("tokenID", rt.ID))
		if delErr := s.refreshTkns.DeleteByUserID(ctx, rt.UserID); delErr != nil {
			slog.Warn("delete refresh tokens by user failed", slog.String("userID", string(rt.UserID)), slog.Any("err", delErr))
		}
		return nil, ErrInvalidCredentials
	}
	return rt, nil
}

// collectPermissions flattens all role permissions for the user. Errors
// are logged and swallowed — the caller gets an access token with an
// empty perm set rather than an outright 500.
func (s *CommandService) collectPermissions(ctx context.Context, userID shared.UserID) []string {
	roles, err := s.userRoles.GetRolesByUserID(ctx, userID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", string(userID)), slog.Any("err", err))
		return nil
	}
	var perms []string
	for _, r := range roles {
		perms = append(perms, r.Permissions...)
	}
	return perms
}

// resolveActiveProfileID picks the profile ID to embed in the access token.
// Returns ok=false only when the caller asked for a specific profile that
// does not belong to them (authorization failure). When no profile matches
// but the caller didn't request one, falls back to the default profile then
// the first profile; ok remains true in that case.
func resolveActiveProfileID(profs []*user.Profile, requestedProfileID string) (string, bool) {
	if requestedProfileID != "" {
		for _, p := range profs {
			if string(p.ID) == requestedProfileID {
				return requestedProfileID, true
			}
		}
		return "", false
	}
	for _, p := range profs {
		if p.IsDefault {
			return string(p.ID), true
		}
	}
	if len(profs) > 0 {
		return string(profs[0].ID), true
	}
	return "", true
}

func (s *CommandService) lookupProfileLocale(ctx context.Context, profileID string) string {
	if profileID == "" {
		return ""
	}
	prof, err := s.profiles.GetByID(ctx, shared.ProfileID(profileID))
	if err != nil || prof == nil {
		return ""
	}
	return prof.Locale
}

func (s *CommandService) Logout(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.Logout")
	defer span.End()
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		return fmt.Errorf("CommandService.Logout: %w", err)
	}
	return nil
}

const maxRefreshTokenRetries = 3

func (s *CommandService) generateAndStoreRefreshToken(ctx context.Context, userID string, ttlOverride ...time.Duration) (string, error) {
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		slog.Warn("cleanup old refresh tokens", slog.String("userID", userID), slog.Any("err", err))
	}
	ttl := s.refreshTokenTTL
	if len(ttlOverride) > 0 && ttlOverride[0] > 0 {
		ttl = ttlOverride[0]
	}
	for range maxRefreshTokenRetries {
		token, err := s.tokens.GenerateRefreshToken()
		if err != nil {
			return "", fmt.Errorf("generate refresh token: %w", err)
		}

		h := sha256.Sum256([]byte(token))
		rt := &user.RefreshToken{
			ID:        id.New("rt-"),
			UserID:    shared.UserID(userID),
			TokenHash: hex.EncodeToString(h[:]),
			ExpiresAt: time.Now().Add(ttl),
			CreatedAt: time.Now(),
		}
		err = s.refreshTkns.Create(ctx, rt)
		if err == nil {
			return token, nil
		}
		if !errors.Is(err, shared.ErrConflict) {
			return "", fmt.Errorf("store refresh token: %w", err)
		}
	}
	return "", errors.New("store refresh token: duplicate hash after retries")
}

func (s *CommandService) StartCleanup(ctx context.Context) {
	s.StartCleanupWithInterval(ctx, 1*time.Hour)
}

func (s *CommandService) StartCleanupWithInterval(ctx context.Context, interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in user cleanup", slog.Any("recover", r))
			}
		}()
		s.runCleanup(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runCleanup(ctx)
			}
		}
	}()
}

func (s *CommandService) runCleanup(ctx context.Context) {
	if err := s.refreshTkns.DeleteExpired(ctx); err != nil {
		slog.Warn("refresh token cleanup", slog.Any("err", err))
	}
	if err := s.quickConnect.DeleteExpired(ctx); err != nil {
		slog.Warn("quick connect cleanup", slog.Any("err", err))
	}
	if s.resetTokens != nil {
		if err := s.resetTokens.DeleteExpired(ctx); err != nil {
			slog.Warn("password reset token cleanup", slog.Any("err", err))
		}
	}
}

func (s *CommandService) IsPasswordRecoveryEnabled(ctx context.Context) bool {
	if !s.smtpConfigured || s.serverCfg == nil {
		return false
	}
	entry, err := s.serverCfg.Get(ctx, "auth.password_recovery_enabled")
	if err != nil {
		return false
	}
	return entry.Value == "true"
}

const passwordResetTokenTTL = 30 * time.Minute

func (s *CommandService) RequestPasswordReset(ctx context.Context, email string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.RequestPasswordReset")
	defer span.End()
	if !s.IsPasswordRecoveryEnabled(ctx) {
		return ErrPasswordRecoveryDisabled
	}
	if s.resetTokens == nil {
		return ErrPasswordRecoveryDisabled
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil // no leak
	}
	if !u.Enabled {
		return nil
	}

	if err := s.resetTokens.DeleteByUserID(ctx, u.ID); err != nil {
		slog.Warn("cleanup old reset tokens", slog.String("userID", string(u.ID)), slog.Any("err", err))
	}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}
	rawHex := hex.EncodeToString(rawToken)
	h := sha256.Sum256([]byte(rawHex))

	t := &user.PasswordResetToken{
		ID:        id.New("prt-"),
		UserID:    u.ID,
		TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(passwordResetTokenTTL),
		CreatedAt: time.Now(),
	}
	if err := s.resetTokens.Create(ctx, t); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	evt := user.PasswordResetRequested{
		UserID:     u.ID,
		Email:      u.Email,
		ResetToken: rawHex,
		ExpiresAt:  t.ExpiresAt,
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.ResetPassword")
	defer span.End()
	if s.resetTokens == nil {
		return ErrResetTokenInvalid
	}

	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])

	t, err := s.resetTokens.GetByTokenHash(ctx, hash)
	if err != nil {
		return ErrResetTokenInvalid
	}
	if !t.IsValid() {
		return ErrResetTokenInvalid
	}

	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}

	u, err := s.users.GetByID(ctx, shared.UserID(t.UserID))
	if err != nil {
		return ErrResetTokenInvalid
	}

	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = newHash
	u.UpdatedAt = time.Now()

	// Atomic: password update and token mark-used must commit together,
	// otherwise a MarkUsed failure leaves the token reusable (TOCTOU).
	apply := func(ctx context.Context) error {
		if err := s.users.Update(ctx, u); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		if err := s.resetTokens.MarkUsed(ctx, t.ID); err != nil {
			return fmt.Errorf("mark reset token used: %w", err)
		}
		return nil
	}
	var txErr error
	if s.uow != nil {
		txErr = s.uow.Do(ctx, apply)
	} else {
		txErr = apply(ctx)
	}
	if txErr != nil {
		return fmt.Errorf("CommandService.ResetPassword: %w", txErr)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, u.ID); err != nil {
		slog.Warn("delete refresh tokens after password reset", slog.Any("err", err))
	}

	evt := user.PasswordChanged{UserID: u.ID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

// loginAttemptLimiter tracks failed login attempts per username.
type loginAttemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptEntry
	maxFails int
	maxSize  int
	window   time.Duration
}

type attemptEntry struct {
	count    int
	lockedAt time.Time
	lastSeen time.Time
}

func newLoginAttemptLimiter(maxFails int, window time.Duration) *loginAttemptLimiter {
	return &loginAttemptLimiter{
		attempts: make(map[string]*attemptEntry),
		maxFails: maxFails,
		maxSize:  10000,
		window:   window,
	}
}

func (l *loginAttemptLimiter) record(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	if len(l.attempts) >= l.maxSize {
		l.evictLocked(now)
	}

	e, ok := l.attempts[username]
	if !ok {
		e = &attemptEntry{}
		l.attempts[username] = e
	} else if now.Sub(e.lastSeen) > l.window {
		e = &attemptEntry{}
		l.attempts[username] = e
	}
	e.count++
	e.lastSeen = now
	if e.count >= l.maxFails {
		e.lockedAt = now
	}
}

// evictLocked removes expired entries, then oldest if still over cap. Caller holds mu.
func (l *loginAttemptLimiter) evictLocked(now time.Time) {
	// pass 1: remove expired
	for k, e := range l.attempts {
		if !e.lockedAt.IsZero() && now.Sub(e.lockedAt) > l.window {
			delete(l.attempts, k)
		} else if e.lockedAt.IsZero() && now.Sub(e.lastSeen) > l.window {
			delete(l.attempts, k)
		}
	}
	if len(l.attempts) < l.maxSize {
		return
	}
	// pass 2: evict oldest entries until under cap
	for len(l.attempts) >= l.maxSize {
		oldestKey := ""
		var oldestTime time.Time
		for k, e := range l.attempts {
			if oldestKey == "" || e.lastSeen.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.lastSeen
			}
		}
		if oldestKey == "" {
			break
		}
		delete(l.attempts, oldestKey)
	}
}

func (l *loginAttemptLimiter) isLocked(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.attempts[username]
	if !ok {
		return false
	}
	if e.lockedAt.IsZero() {
		return false
	}
	if time.Since(e.lockedAt) < l.window {
		return true
	}
	delete(l.attempts, username)
	return false
}

func (l *loginAttemptLimiter) reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username)
}

// IsLocked implements user.LoginAttemptRepository.
func (l *loginAttemptLimiter) IsLocked(_ context.Context, username string) (bool, error) {
	return l.isLocked(username), nil
}

// Record implements user.LoginAttemptRepository.
// maxFails and window override the limiter's own values when provided.
func (l *loginAttemptLimiter) Record(_ context.Context, username string, maxFails int, window time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	if len(l.attempts) >= l.maxSize {
		l.evictLocked(now)
	}

	e, ok := l.attempts[username]
	if !ok {
		e = &attemptEntry{}
		l.attempts[username] = e
	} else if now.Sub(e.lastSeen) > window {
		e = &attemptEntry{}
		l.attempts[username] = e
	}
	e.count++
	e.lastSeen = now
	if e.count >= maxFails {
		e.lockedAt = now
	}
	return nil
}

// Reset implements user.LoginAttemptRepository.
func (l *loginAttemptLimiter) Reset(_ context.Context, username string) error {
	l.reset(username)
	return nil
}

var _ user.LoginAttemptRepository = (*loginAttemptLimiter)(nil)
