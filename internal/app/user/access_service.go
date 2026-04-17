package user

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
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
	defaultLocale func() string
}

type AccessServiceOption func(*AccessService)

func WithProfiles(profiles user.ProfileRepository) AccessServiceOption {
	return func(s *AccessService) { s.profiles = profiles }
}

func WithLibraryAccess(la user.LibraryAccessRepository) AccessServiceOption {
	return func(s *AccessService) { s.libraryAccess = la }
}

// WithAccessDefaultLocale sets the server-wide default locale resolver used
// when creating a new Profile via invite registration (mirrors UserService).
func WithAccessDefaultLocale(fn func() string) AccessServiceOption {
	return func(s *AccessService) { s.defaultLocale = fn }
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

	// Build + validate the user BEFORE claiming the invite slot so a malformed
	// input doesn't burn a MaxUses=1 slot.
	u, err := user.NewUser(username, email, hash)
	if err != nil {
		return nil, fmt.Errorf("AccessService.RegisterWithInvite: %w", err)
	}
	u.IsAdmin = invite.IsAdmin

	// Atomically claim one invite use-slot. The repo's IncrementUsedCount
	// rejects the claim if the invite has been exhausted since our IsUsable()
	// check above, closing a TOCTOU window where N concurrent registrations
	// on a MaxUses=1 token all succeeded.
	if err := s.invites.IncrementUsedCount(ctx, invite.ID); err != nil {
		return nil, ErrInviteInvalid
	}

	if err := s.users.Create(ctx, u); err != nil {
		// Best-effort rollback of the claim we just took. If this too fails,
		// the slot stays consumed — that's a degraded outcome (admin can
		// re-create the invite) rather than a security risk.
		if decErr := s.invites.DecrementUsedCount(ctx, invite.ID); decErr != nil {
			slog.Warn("invite: decrement after user.Create failed", slog.String("inviteID", invite.ID), slog.Any("err", decErr))
		}
		return nil, fmt.Errorf("AccessService.RegisterWithInvite: %w", err)
	}
	if s.libraryAccess != nil && !u.IsAdmin && len(invite.LibraryIDs) > 0 {
		if grantErr := s.libraryAccess.GrantMany(ctx, u.ID, invite.LibraryIDs); grantErr != nil {
			slog.Warn("invite: grant library access", slog.String("userID", string(u.ID)), slog.Any("err", grantErr))
		}
	}
	evtInviteCreated := user.UserCreated{UserID: u.ID, Username: u.Username, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtInviteCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtInviteCreated.EventName()), slog.Any("err", err))
	}

	if s.profiles != nil {
		locale := ""
		if s.defaultLocale != nil {
			locale = s.defaultLocale()
		}
		p := &user.Profile{
			ID:        shared.ProfileID(id.New("prf-")),
			UserID:    u.ID,
			Name:      u.Username,
			IsDefault: true,
			Locale:    locale,
			CreatedAt: time.Now(),
		}
		if pErr := s.profiles.Create(ctx, p); pErr != nil {
			slog.Warn("auto-create profile", slog.String("userID", string(u.ID)), slog.Any("err", pErr))
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
	if err := s.quickConnect.UpdateStatus(ctx, req.ID, string(user.Authorized), &userID); err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeQuickConnect: %w", err)
	}
	return req.Code, nil
}

func (s *AccessService) AuthorizeByShortCode(ctx context.Context, shortCode string, userID string) (fullCode string, err error) {
	req, err := s.quickConnect.GetByShortCode(ctx, shortCode)
	if err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeByShortCode: %w", err)
	}
	if err := s.quickConnect.UpdateStatus(ctx, req.ID, string(user.Authorized), &userID); err != nil {
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
	return s.libraryAccess.HasAccess(ctx, shared.UserID(userID), shared.LibraryID(libraryID))
}

func (s *AccessService) GetLibraryAccessByUserID(ctx context.Context, userID string) ([]string, error) {
	ids, err := s.libraryAccess.GetByUserID(ctx, shared.UserID(userID))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out, nil
}

func (s *AccessService) GrantLibraryAccess(ctx context.Context, userID, libraryID string) error {
	return s.libraryAccess.Grant(ctx, shared.UserID(userID), shared.LibraryID(libraryID))
}

func (s *AccessService) RevokeLibraryAccess(ctx context.Context, userID, libraryID string) error {
	return s.libraryAccess.Revoke(ctx, shared.UserID(userID), shared.LibraryID(libraryID))
}
