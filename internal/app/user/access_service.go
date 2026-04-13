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
	users         user.UserRepository
	profiles      user.ProfileRepository
	apiKeys       user.APIKeyRepository
	invites       user.InviteLinkRepository
	quickConnect  user.QuickConnectRepository
	libraryAccess user.LibraryAccessRepository
	hasher        user.PasswordHasher
	events        shared.EventBus
}

type AccessServiceOption func(*AccessService)

func WithProfiles(profiles user.ProfileRepository) AccessServiceOption {
	return func(s *AccessService) { s.profiles = profiles }
}

func WithLibraryAccess(la user.LibraryAccessRepository) AccessServiceOption {
	return func(s *AccessService) { s.libraryAccess = la }
}

func NewAccessService(
	users user.UserRepository,
	apiKeys user.APIKeyRepository,
	invites user.InviteLinkRepository,
	quickConnect user.QuickConnectRepository,
	hasher user.PasswordHasher,
	events shared.EventBus,
	opts ...AccessServiceOption,
) *AccessService {
	s := &AccessService{
		users:        users,
		apiKeys:      apiKeys,
		invites:      invites,
		quickConnect: quickConnect,
		hasher:       hasher,
		events:       events,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *AccessService) CreateInviteLink(ctx context.Context, link *user.InviteLink) error {
	if err := link.Validate(); err != nil {
		return fmt.Errorf("invalid invite link: %w", err)
	}
	link.CreatedAt = time.Now()
	if err := s.invites.Create(ctx, link); err != nil {
		return fmt.Errorf("AccessService.CreateInviteLink: %w", err)
	}
	return nil
}

func (s *AccessService) RevokeInviteLink(ctx context.Context, inviteID string) error {
	if err := s.invites.Delete(ctx, inviteID); err != nil {
		return fmt.Errorf("AccessService.RevokeInviteLink: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("AccessService.RegisterWithInvite: %w", err)
	}

	u, err := user.NewUser(username, email, hash)
	if err != nil {
		return nil, fmt.Errorf("AccessService.RegisterWithInvite: %w", err)
	}
	u.IsAdmin = invite.IsAdmin
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("AccessService.RegisterWithInvite: %w", err)
	}
	if err := s.invites.IncrementUsedCount(ctx, invite.ID); err != nil {
		slog.Warn("increment invite used count failed", slog.String("inviteID", invite.ID), slog.Any("err", err))
	}
	evtInviteCreated := user.UserCreated{UserID: u.ID, Username: u.Username, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInviteCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtInviteCreated.EventName()), slog.Any("err", err))
	}

	if s.profiles != nil {
		p := &user.Profile{
			ID:        id.New("prf-"),
			UserID:    u.ID,
			Name:      u.Username,
			IsDefault: true,
			CreatedAt: time.Now(),
		}
		if pErr := s.profiles.Create(ctx, p); pErr != nil {
			slog.Warn("auto-create profile", slog.String("userID", u.ID), slog.Any("err", pErr))
		} else {
			evtProfile := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
			if err := s.events.Publish(ctx, evtProfile); err != nil {
				slog.Warn("publish event", slog.String("event", evtProfile.EventName()), slog.Any("err", err))
			}
		}
	}
	return u, nil
}

func (s *AccessService) InitiateQuickConnect(ctx context.Context, req *user.QuickConnectRequest) error {
	req.CreatedAt = time.Now()
	if err := s.quickConnect.Create(ctx, req); err != nil {
		return fmt.Errorf("AccessService.InitiateQuickConnect: %w", err)
	}
	return nil
}

func (s *AccessService) AuthorizeQuickConnect(ctx context.Context, code string, userID string) (fullCode string, err error) {
	req, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeQuickConnect: %w", err)
	}
	if err := s.quickConnect.UpdateStatus(ctx, req.ID, "authorized", &userID); err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeQuickConnect: %w", err)
	}
	return req.Code, nil
}

func (s *AccessService) AuthorizeByShortCode(ctx context.Context, shortCode string, userID string) (fullCode string, err error) {
	req, err := s.quickConnect.GetByShortCode(ctx, shortCode)
	if err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeByShortCode: %w", err)
	}
	if err := s.quickConnect.UpdateStatus(ctx, req.ID, "authorized", &userID); err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeByShortCode: %w", err)
	}
	return req.Code, nil
}

func (s *AccessService) CreateAPIKey(ctx context.Context, key *user.APIKeyModel) error {
	if err := key.Validate(); err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}
	key.CreatedAt = time.Now()
	if err := s.apiKeys.Create(ctx, key); err != nil {
		return fmt.Errorf("AccessService.CreateAPIKey: %w", err)
	}
	evt := user.APIKeyCreated{KeyID: key.ID, UserID: key.UserID, Name: key.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *AccessService) RevokeAPIKey(ctx context.Context, keyID string) error {
	if err := s.apiKeys.Delete(ctx, keyID); err != nil {
		return fmt.Errorf("AccessService.RevokeAPIKey: %w", err)
	}
	evt := user.APIKeyRevoked{KeyID: keyID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *AccessService) HasLibraryAccess(ctx context.Context, userID, libraryID string) (bool, error) {
	return s.libraryAccess.HasAccess(ctx, userID, libraryID)
}

func (s *AccessService) GetLibraryAccessByUserID(ctx context.Context, userID string) ([]string, error) {
	return s.libraryAccess.GetByUserID(ctx, userID)
}

func (s *AccessService) GrantLibraryAccess(ctx context.Context, userID, libraryID string) error {
	return s.libraryAccess.Grant(ctx, userID, libraryID)
}

func (s *AccessService) RevokeLibraryAccess(ctx context.Context, userID, libraryID string) error {
	return s.libraryAccess.Revoke(ctx, userID, libraryID)
}
