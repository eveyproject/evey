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

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user is disabled")
	ErrNotAdmin           = errors.New("admin required")
	ErrLastAdmin          = errors.New("cannot remove last admin")
	ErrSetupDone          = errors.New("setup already completed")
	ErrInviteInvalid      = errors.New("invalid or expired invite")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
)

type Service struct {
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

func NewService(
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
) *Service {
	return &Service{
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

func (s *Service) InitSetupState(ctx context.Context) {
	count, err := s.users.CountAdmins(ctx)
	if err == nil && count > 0 {
		s.setupDone = true
	}
}

func (s *Service) CountAdmins(ctx context.Context) (int, error) {
	return s.users.CountAdmins(ctx)
}

func (s *Service) Setup(ctx context.Context, username, password, email string) (*user.User, error) {
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

	s.setupDone = true
	return u, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (accessToken, refreshToken string, err error) {
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

func (s *Service) RefreshToken(ctx context.Context, token string) (string, string, error) {
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

func (s *Service) Logout(ctx context.Context, userID string) error {
	return s.refreshTkns.DeleteByUserID(ctx, userID)
}

func (s *Service) CreateUser(ctx context.Context, callerIsAdmin bool, u *user.User, password string) error {
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

func (s *Service) UpdateUser(ctx context.Context, u *user.User) error {
	u.UpdatedAt = time.Now()
	return s.users.Update(ctx, u)
}

func (s *Service) DeleteUser(ctx context.Context, id string) error {
	u, err := s.users.GetByID(ctx, id)
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
	if err := s.users.Delete(ctx, id); err != nil {
		return err
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, id); err != nil {
		slog.Warn("delete refresh tokens for deleted user failed", slog.String("userID", id), slog.Any("err", err))
	}
	evtDeleted := user.UserDeleted{UserID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.Warn("publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) DisableUser(ctx context.Context, id string) error {
	u, err := s.users.GetByID(ctx, id)
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
	if err := s.refreshTkns.DeleteByUserID(ctx, id); err != nil {
		slog.Warn("delete refresh tokens for disabled user failed", slog.String("userID", id), slog.Any("err", err))
	}
	evtDisabled := user.UserDisabled{UserID: id, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDisabled); err != nil {
		slog.Warn("publish event", slog.String("event", evtDisabled.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) ListUsers(ctx context.Context) ([]*user.User, error) {
	return s.users.List(ctx)
}

func (s *Service) CreateProfile(ctx context.Context, p *user.Profile) error {
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

func (s *Service) UpdateProfile(ctx context.Context, p *user.Profile) error {
	return s.profiles.Update(ctx, p)
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	return s.profiles.Delete(ctx, id)
}

func (s *Service) ListProfiles(ctx context.Context, userID string) ([]*user.Profile, error) {
	return s.profiles.GetByUserID(ctx, userID)
}

func (s *Service) CreateInviteLink(ctx context.Context, link *user.InviteLink) error {
	link.CreatedAt = time.Now()
	return s.invites.Create(ctx, link)
}

func (s *Service) ListInviteLinks(ctx context.Context, creatorID string) ([]*user.InviteLink, error) {
	return s.invites.ListByCreator(ctx, creatorID)
}

func (s *Service) RevokeInviteLink(ctx context.Context, id string) error {
	return s.invites.Delete(ctx, id)
}

func (s *Service) RegisterWithInvite(ctx context.Context, token, username, password, email string) (*user.User, error) {
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

func (s *Service) InitiateQuickConnect(ctx context.Context, req *user.QuickConnectRequest) error {
	req.CreatedAt = time.Now()
	return s.quickConnect.Create(ctx, req)
}

func (s *Service) AuthorizeQuickConnect(ctx context.Context, code string, userID string) error {
	req, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return err
	}
	return s.quickConnect.UpdateStatus(ctx, req.ID, "authorized", &userID)
}

func (s *Service) CheckQuickConnect(ctx context.Context, code string) (*user.QuickConnectRequest, error) {
	return s.quickConnect.GetByCode(ctx, code)
}

func (s *Service) CreateAPIKey(ctx context.Context, key *user.APIKeyModel) error {
	key.CreatedAt = time.Now()
	return s.apiKeys.Create(ctx, key)
}

func (s *Service) ListAPIKeys(ctx context.Context, userID string) ([]*user.APIKeyModel, error) {
	return s.apiKeys.ListByUserID(ctx, userID)
}

func (s *Service) RevokeAPIKey(ctx context.Context, id string) error {
	return s.apiKeys.Delete(ctx, id)
}

const maxRefreshTokenRetries = 3

func (s *Service) generateAndStoreRefreshToken(ctx context.Context, userID string) (string, error) {
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
