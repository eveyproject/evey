package user

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type AccessService struct {
	users        user.UserRepository
	apiKeys      user.APIKeyRepository
	invites      user.InviteLinkRepository
	quickConnect user.QuickConnectRepository
	hasher       user.PasswordHasher
	events       shared.EventBus
}

func NewAccessService(
	users user.UserRepository,
	apiKeys user.APIKeyRepository,
	invites user.InviteLinkRepository,
	quickConnect user.QuickConnectRepository,
	hasher user.PasswordHasher,
	events shared.EventBus,
) *AccessService {
	return &AccessService{
		users:        users,
		apiKeys:      apiKeys,
		invites:      invites,
		quickConnect: quickConnect,
		hasher:       hasher,
		events:       events,
	}
}

func (s *AccessService) CreateInviteLink(ctx context.Context, link *user.InviteLink) error {
	if err := link.Validate(); err != nil {
		return fmt.Errorf("invalid invite link: %w", err)
	}
	link.CreatedAt = time.Now()
	return s.invites.Create(ctx, link)
}

func (s *AccessService) RevokeInviteLink(ctx context.Context, inviteID string) error {
	return s.invites.Delete(ctx, inviteID)
}

func (s *AccessService) RegisterWithInvite(ctx context.Context, token, username, password, email string) (*user.User, error) {
	if len(password) < 8 {
		return nil, ErrPasswordTooShort
	}
	invite, err := s.invites.GetByToken(ctx, token)
	if err != nil {
		return nil, ErrInviteInvalid
	}
	if !invite.IsUsable() {
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

func (s *AccessService) InitiateQuickConnect(ctx context.Context, req *user.QuickConnectRequest) error {
	req.CreatedAt = time.Now()
	return s.quickConnect.Create(ctx, req)
}

func (s *AccessService) AuthorizeQuickConnect(ctx context.Context, code string, userID string) error {
	req, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return err
	}
	return s.quickConnect.UpdateStatus(ctx, req.ID, "authorized", &userID)
}

func (s *AccessService) CreateAPIKey(ctx context.Context, key *user.APIKeyModel) error {
	if err := key.Validate(); err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}
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

func (s *AccessService) RevokeAPIKey(ctx context.Context, keyID string) error {
	if err := s.apiKeys.Delete(ctx, keyID); err != nil {
		return err
	}
	evt := user.APIKeyRevoked{KeyID: keyID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}
