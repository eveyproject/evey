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

	"github.com/lib/pq"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type CommandService struct {
	users        user.UserRepository
	profiles     user.ProfileRepository
	refreshTkns  user.RefreshTokenRepository
	apiKeys      user.APIKeyRepository
	invites      user.InviteLinkRepository
	quickConnect user.QuickConnectRepository
	roles        user.RoleRepository
	userRoles    user.UserRoleRepository
	hasher       user.PasswordHasher
	tokens       user.TokenGenerator
	events       shared.EventBus
	setupMu      sync.Mutex
	setupDone    bool
}

func NewCommandService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	refreshTkns user.RefreshTokenRepository,
	apiKeys user.APIKeyRepository,
	invites user.InviteLinkRepository,
	quickConnect user.QuickConnectRepository,
	roles user.RoleRepository,
	userRoles user.UserRoleRepository,
	hasher user.PasswordHasher,
	tokens user.TokenGenerator,
	events shared.EventBus,
) *CommandService {
	return &CommandService{
		users:        users,
		profiles:     profiles,
		refreshTkns:  refreshTkns,
		apiKeys:      apiKeys,
		invites:      invites,
		quickConnect: quickConnect,
		roles:        roles,
		userRoles:    userRoles,
		hasher:       hasher,
		tokens:       tokens,
		events:       events,
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
	if !u.Enabled {
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

	if time.Now().After(rt.ExpiresAt) {
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

func (s *CommandService) CreateUser(ctx context.Context, callerIsAdmin bool, u *user.User, password string) error {
	if !callerIsAdmin {
		return ErrNotAdmin
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	if err := s.users.Create(ctx, u); err != nil {
		return err
	}
	evtUserCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUserCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtUserCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) UpdateUser(ctx context.Context, u *user.User) error {
	u.UpdatedAt = time.Now()
	return s.users.Update(ctx, u)
}

func (s *CommandService) SetUserBitrateLimit(ctx context.Context, userID string, maxBitrateKbps int) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	u.MaxBitrateKbps = maxBitrateKbps
	u.UpdatedAt = time.Now()
	return s.users.Update(ctx, u)
}

func (s *CommandService) ChangePassword(ctx context.Context, userID, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	evt := user.PasswordChanged{UserID: userID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) DeleteUser(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.IsAdmin {
		count, err := s.users.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	if err := s.users.Delete(ctx, userID); err != nil {
		return err
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("delete refresh tokens for deleted user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	evtDeleted := user.UserDeleted{UserID: userID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.Warn("publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) DisableUser(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.IsAdmin {
		count, err := s.users.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	u.Enabled = false
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, userID); err != nil {
		slog.Warn("delete refresh tokens for disabled user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	evtDisabled := user.UserDisabled{UserID: userID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDisabled); err != nil {
		slog.Warn("publish event", slog.String("event", evtDisabled.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) CreateProfile(ctx context.Context, p *user.Profile) error {
	p.CreatedAt = time.Now()
	if err := s.profiles.Create(ctx, p); err != nil {
		return err
	}
	evtProfileCreated := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfileCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) UpdateProfile(ctx context.Context, p *user.Profile) error {
	return s.profiles.Update(ctx, p)
}

func (s *CommandService) DeleteProfile(ctx context.Context, profileID string) error {
	p, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if err := s.profiles.Delete(ctx, profileID); err != nil {
		return err
	}
	evt := user.ProfileDeleted{ProfileID: profileID, UserID: p.UserID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) CreateInviteLink(ctx context.Context, link *user.InviteLink) error {
	link.CreatedAt = time.Now()
	return s.invites.Create(ctx, link)
}

func (s *CommandService) RevokeInviteLink(ctx context.Context, inviteID string) error {
	return s.invites.Delete(ctx, inviteID)
}

func (s *CommandService) RegisterWithInvite(ctx context.Context, token, username, password, email string) (*user.User, error) {
	if len(password) < 8 {
		return nil, ErrPasswordTooShort
	}
	invite, err := s.invites.GetByToken(ctx, token)
	if err != nil {
		return nil, ErrInviteInvalid
	}
	if invite.ExpiresAt != nil && time.Now().After(*invite.ExpiresAt) {
		return nil, ErrInviteInvalid
	}
	if invite.MaxUses > 0 && invite.UsedCount >= invite.MaxUses {
		return nil, ErrInviteInvalid
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, err
	}

	u := &user.User{
		ID:           id.New("usr-"),
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		IsAdmin:      invite.IsAdmin,
		Enabled:      true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	if err := s.invites.IncrementUsedCount(ctx, invite.ID); err != nil {
		slog.Warn("increment invite used count failed", slog.String("inviteID", invite.ID), slog.Any("err", err))
	}
	evtInviteCreated := user.UserCreated{UserID: u.ID, Username: u.Username, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInviteCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtInviteCreated.EventName()), slog.Any("err", err))
	}
	return u, nil
}

func (s *CommandService) InitiateQuickConnect(ctx context.Context, req *user.QuickConnectRequest) error {
	req.CreatedAt = time.Now()
	return s.quickConnect.Create(ctx, req)
}

func (s *CommandService) AuthorizeQuickConnect(ctx context.Context, code string, userID string) error {
	req, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return err
	}
	return s.quickConnect.UpdateStatus(ctx, req.ID, "authorized", &userID)
}

func (s *CommandService) CreateAPIKey(ctx context.Context, key *user.APIKeyModel) error {
	key.CreatedAt = time.Now()
	if err := s.apiKeys.Create(ctx, key); err != nil {
		return err
	}
	evt := user.APIKeyCreated{KeyID: key.ID, UserID: key.UserID, Name: key.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) RevokeAPIKey(ctx context.Context, keyID string) error {
	if err := s.apiKeys.Delete(ctx, keyID); err != nil {
		return err
	}
	evt := user.APIKeyRevoked{KeyID: keyID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

const maxRefreshTokenRetries = 3

func (s *CommandService) generateAndStoreRefreshToken(ctx context.Context, userID string) (string, error) {
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
		var pqErr *pq.Error
		if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
			return "", fmt.Errorf("store refresh token: %w", err)
		}
	}
	return "", errors.New("store refresh token: duplicate hash after retries")
}

func (s *CommandService) StartCleanup(ctx context.Context) {
	go func() {
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
				if err := s.quickConnect.DeleteExpired(ctx); err != nil {
					slog.Warn("quick connect cleanup", slog.Any("err", err))
				}
			}
		}
	}()
}
