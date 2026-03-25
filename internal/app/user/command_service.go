package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

// CommandService handles auth, setup and token management.
type CommandService struct {
	users       user.UserRepository
	profiles    user.ProfileRepository
	refreshTkns user.RefreshTokenRepository
	roles       user.RoleRepository
	userRoles   user.UserRoleRepository
	hasher      user.PasswordHasher
	tokens      user.TokenGenerator
	events      shared.EventBus
	setupMu     sync.Mutex
	setupDone   bool
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
) *CommandService {
	return &CommandService{
		users:       users,
		profiles:    profiles,
		refreshTkns: refreshTkns,
		roles:       roles,
		userRoles:   userRoles,
		hasher:      hasher,
		tokens:      tokens,
		events:      events,
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
		return nil, err
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
		return nil, err
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
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return "", "", ErrInvalidCredentials
	}
	if !u.CanLogin() {
		return "", "", ErrUserDisabled
	}

	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if err != nil || !ok {
		return "", "", ErrInvalidCredentials
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
		return "", "", err
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
		return "", "", err
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
		return "", "", err
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
		return "", "", err
	}

	newRefresh, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}
	nh := sha256.Sum256([]byte(newRefresh))
	newRT := &user.RefreshToken{
		ID:        id.New("rt-"),
		UserID:    u.ID,
		TokenHash: hex.EncodeToString(nh[:]),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := s.refreshTkns.Create(ctx, newRT); err != nil {
		return "", "", err
	}

	return accessToken, newRefresh, nil
}

func (s *CommandService) Logout(ctx context.Context, userID string) error {
	return s.refreshTkns.DeleteByUserID(ctx, userID)
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
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
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

func (s *CommandService) StartCleanup(ctx context.Context, quickConnect user.QuickConnectRepository) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in user cleanup", slog.Any("recover", r))
			}
		}()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.refreshTkns.DeleteExpired(ctx); err != nil {
					slog.Warn("refresh token cleanup", slog.Any("err", err))
				}
				if err := quickConnect.DeleteExpired(ctx); err != nil {
					slog.Warn("quick connect cleanup", slog.Any("err", err))
				}
			}
		}
	}()
}
