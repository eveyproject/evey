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

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

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
	loginAttempts   *loginAttemptLimiter
	setupMu         sync.Mutex
	setupDone       bool
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
		loginAttempts:   newLoginAttemptLimiter(5, 15*time.Minute),
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

func (s *CommandService) InitSetupState(ctx context.Context) {
	count, err := s.users.CountAdmins(ctx)
	if err == nil && count > 0 {
		s.setupDone = true
	}
}

func (s *CommandService) Setup(ctx context.Context, username, password, email string) (*user.User, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupDone {
		return nil, ErrSetupDone
	}
	if username == "" {
		return nil, fmt.Errorf("username is required")
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

	u := &user.User{
		ID:           id.New("usr-"),
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		IsAdmin:      true,
		Enabled:      true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("CommandService.Setup: %w", err)
	}
	evtSetupCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: true, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtSetupCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtSetupCreated.EventName()), slog.Any("err", err))
	}

	p := &user.Profile{
		ID:        id.New("prf-"),
		UserID:    u.ID,
		Name:      u.Username,
		IsDefault: true,
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

func (s *CommandService) Login(ctx context.Context, username, password string) (accessToken, refreshToken string, err error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if s.loginAttempts.isLocked(username) {
		return "", "", ErrAccountLocked
	}

	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		s.loginAttempts.record(username)
		return "", "", ErrInvalidCredentials
	}
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}

	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if err != nil || !ok {
		s.loginAttempts.record(username)
		return "", "", ErrInvalidCredentials
	}
	s.loginAttempts.reset(username)

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", u.ID), slog.Any("err", err))
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

	accessToken, err = s.tokens.GenerateAccessToken(user.Claims{
		UserID:      u.ID,
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, u.ID)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.Login: %w", err)
	}

	evtLogin := user.LoginSucceeded{UserID: u.ID, AuthMethod: "local", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtLogin); err != nil {
		slog.Warn("publish event", slog.String("event", evtLogin.EventName()), slog.Any("err", err))
	}
	return accessToken, refreshToken, nil
}

func (s *CommandService) GenerateTokensForUser(ctx context.Context, userID string) (accessToken, refreshToken string, err error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", "", fmt.Errorf("user not found: %w", err)
	}
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", u.ID), slog.Any("err", err))
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

	accessToken, err = s.tokens.GenerateAccessToken(user.Claims{
		UserID:      u.ID,
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = s.generateAndStoreRefreshToken(ctx, u.ID)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.GenerateTokensForUser: %w", err)
	}
	return accessToken, refreshToken, nil
}

func (s *CommandService) RefreshToken(ctx context.Context, token string) (string, string, error) {
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])

	rt, err := s.refreshTkns.GetByTokenHash(ctx, hash)
	if err != nil {
		return "", "", ErrInvalidCredentials
	}

	if rt.Used {
		if delErr := s.refreshTkns.DeleteByUserID(ctx, rt.UserID); delErr != nil {
			slog.Warn("delete refresh tokens by user failed", slog.String("userID", rt.UserID), slog.Any("err", delErr))
		}
		return "", "", ErrInvalidCredentials
	}

	if rt.IsExpired() {
		if delErr := s.refreshTkns.DeleteByID(ctx, rt.ID); delErr != nil {
			slog.Warn("delete refresh token failed", slog.String("tokenID", rt.ID), slog.Any("err", delErr))
		}
		return "", "", ErrInvalidCredentials
	}

	if markErr := s.refreshTkns.MarkUsed(ctx, rt.ID); markErr != nil {
		slog.Warn("mark refresh token used failed", slog.String("tokenID", rt.ID), slog.Any("err", markErr))
	}

	u, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}

	roles, err := s.userRoles.GetRolesByUserID(ctx, u.ID)
	if err != nil {
		slog.Warn("get roles by user failed", slog.String("userID", u.ID), slog.Any("err", err))
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

	accessToken, err := s.tokens.GenerateAccessToken(user.Claims{
		UserID:      u.ID,
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		ProfileID:   defaultProfileID,
		Permissions: perms,
	})
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}

	newRefresh, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}
	nh := sha256.Sum256([]byte(newRefresh))
	newRT := &user.RefreshToken{
		ID:        id.New("rt-"),
		UserID:    u.ID,
		TokenHash: hex.EncodeToString(nh[:]),
		ExpiresAt: time.Now().Add(s.refreshTokenTTL),
		CreatedAt: time.Now(),
	}
	if err := s.refreshTkns.Create(ctx, newRT); err != nil {
		return "", "", fmt.Errorf("CommandService.RefreshToken: %w", err)
	}

	return accessToken, newRefresh, nil
}

func (s *CommandService) Logout(ctx context.Context, userID string) error {
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		return fmt.Errorf("CommandService.Logout: %w", err)
	}
	return nil
}

const maxRefreshTokenRetries = 3

func (s *CommandService) generateAndStoreRefreshToken(ctx context.Context, userID string) (string, error) {
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("cleanup old refresh tokens", slog.String("userID", userID), slog.Any("err", err))
	}
	for range maxRefreshTokenRetries {
		token, err := s.tokens.GenerateRefreshToken()
		if err != nil {
			return "", fmt.Errorf("generate refresh token: %w", err)
		}

		h := sha256.Sum256([]byte(token))
		rt := &user.RefreshToken{
			ID:        id.New("rt-"),
			UserID:    userID,
			TokenHash: hex.EncodeToString(h[:]),
			ExpiresAt: time.Now().Add(s.refreshTokenTTL),
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
		slog.Warn("cleanup old reset tokens", slog.String("userID", u.ID), slog.Any("err", err))
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

	u, err := s.users.GetByID(ctx, t.UserID)
	if err != nil {
		return ErrResetTokenInvalid
	}

	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = newHash
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("CommandService.ResetPassword: %w", err)
	}

	if err := s.resetTokens.MarkUsed(ctx, t.ID); err != nil {
		slog.Warn("mark reset token used", slog.Any("err", err))
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
	window   time.Duration
}

type attemptEntry struct {
	count    int
	lockedAt time.Time
}

func newLoginAttemptLimiter(maxFails int, window time.Duration) *loginAttemptLimiter {
	return &loginAttemptLimiter{
		attempts: make(map[string]*attemptEntry),
		maxFails: maxFails,
		window:   window,
	}
}

func (l *loginAttemptLimiter) record(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.attempts[username]
	if !ok || time.Since(e.lockedAt) > l.window {
		e = &attemptEntry{}
		l.attempts[username] = e
	}
	e.count++
	if e.count >= l.maxFails {
		e.lockedAt = time.Now()
	}
}

func (l *loginAttemptLimiter) isLocked(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.attempts[username]
	if !ok {
		return false
	}
	if e.count >= l.maxFails && time.Since(e.lockedAt) < l.window {
		return true
	}
	if time.Since(e.lockedAt) >= l.window {
		delete(l.attempts, username)
	}
	return false
}

func (l *loginAttemptLimiter) reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username)
}
