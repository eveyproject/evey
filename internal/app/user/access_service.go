package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	uow           shared.TransactionalUnit
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

// WithAccessUnitOfWork wires a transactional unit so RegisterWithInvite runs
// its 4 writes (invite claim + user create + library grants + default profile)
// atomically. Without it the flow falls back to best-effort sequential
// execution with manual decrement-on-failure (legacy partial-rollback path).
func WithAccessUnitOfWork(uow shared.TransactionalUnit) AccessServiceOption {
	return func(s *AccessService) { s.uow = uow }
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
	ctx, span := tracing.StartSpan(ctx, "app.user.CreateInviteLink")
	defer span.End()
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
	ctx, span := tracing.StartSpan(ctx, "app.user.RevokeInviteLink")
	defer span.End()
	if err := s.invites.Delete(ctx, inviteID); err != nil {
		return fmt.Errorf("AccessService.RevokeInviteLink: %w", err)
	}
	return nil
}

// ValidateInvite reports whether an invite code is still usable and whether it
// would create an admin account. Used by the registration page to gate the
// form. An unknown/expired/revoked/exhausted code returns (false, false, nil) -
// not an error - so the caller shows the same neutral "link no longer valid"
// state regardless of the reason (no oracle on which codes ever existed).
func (s *AccessService) ValidateInvite(ctx context.Context, token string) (valid bool, isAdmin bool, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.ValidateInvite")
	defer span.End()
	invite, err := s.invites.GetByToken(ctx, token)
	if err != nil {
		return false, false, nil
	}
	if !invite.IsUsable() {
		return false, false, nil
	}
	return true, invite.IsAdmin, nil
}

func (s *AccessService) RegisterWithInvite(ctx context.Context, token, username, password, email string) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.RegisterWithInvite")
	defer span.End()
	password = normalizePassword(password)
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
	// The invite's profile cap becomes the account's, exactly as an admin
	// would have set it creating the user directly.
	if invite.MaxProfiles != nil {
		mp := *invite.MaxProfiles
		u.MaxProfiles = &mp
	}

	// Pre-build the default Profile so its ID is stable before the tx; we
	// publish the ProfileCreated event with this exact ID after commit.
	var p *user.Profile
	if s.profiles != nil {
		locale := ""
		if s.defaultLocale != nil {
			locale = s.defaultLocale()
		}
		p, err = user.NewProfile(shared.ProfileID(id.New("prf-")), u.ID, u.Username,
			user.WithProfileDefault(true),
			user.WithProfileLocale(locale),
		)
		if err != nil {
			return nil, fmt.Errorf("AccessService.RegisterWithInvite: build default profile: %w", err)
		}
	}

	// apply runs the four mutations atomically. With a TransactionalUnit
	// (production), any failure rolls back: no partial user, no consumed
	// invite slot, no orphan grants. Without one (legacy unit-tested path)
	// we fall back to best-effort sequential ops with the historical
	// IncrementUsedCount/DecrementUsedCount compensation around user.Create.
	apply := func(ctx context.Context) error {
		// Atomic claim of one invite use-slot. The repo's IncrementUsedCount
		// rejects the claim if the invite has been exhausted since our
		// IsUsable() check above, closing a TOCTOU window where N concurrent
		// registrations on a MaxUses=1 token all succeeded.
		if err := s.invites.IncrementUsedCount(ctx, invite.ID); err != nil {
			return ErrInviteInvalid
		}
		if err := s.users.Create(ctx, u); err != nil {
			if s.uow == nil {
				// No tx: best-effort decrement so the slot doesn't leak.
				if decErr := s.invites.DecrementUsedCount(ctx, invite.ID); decErr != nil {
					slog.WarnContext(ctx, "invite: decrement after user.Create failed", slog.String("inviteID", invite.ID), slog.Any("err", decErr))
				}
			}
			return fmt.Errorf("create user: %w", err)
		}
		if s.libraryAccess != nil && !u.IsAdmin && len(invite.LibraryIDs) > 0 {
			if err := s.libraryAccess.GrantMany(ctx, u.ID, invite.LibraryIDs); err != nil {
				return fmt.Errorf("grant library access: %w", err)
			}
		}
		if p != nil {
			if err := s.profiles.Create(ctx, p); err != nil {
				return fmt.Errorf("create default profile: %w", err)
			}
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
		// Surface ErrInviteInvalid unchanged so callers can errors.Is it.
		if errors.Is(txErr, ErrInviteInvalid) {
			return nil, ErrInviteInvalid
		}
		return nil, fmt.Errorf("AccessService.RegisterWithInvite: %w", txErr)
	}

	// Events are published POST-commit so a tx rollback never produces a
	// phantom UserCreated/ProfileCreated.
	evtUserCreated := user.UserCreated{UserID: u.ID, Username: u.Username, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUserCreated); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtUserCreated.EventName()), slog.Any("err", err))
	}
	if p != nil {
		evtProfile := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
		if err := s.events.Publish(ctx, evtProfile); err != nil {
			slog.WarnContext(ctx, "publish event", slog.String("event", evtProfile.EventName()), slog.Any("err", err))
		}
	}
	return u, nil
}

func (s *AccessService) InitiateQuickConnect(ctx context.Context, req *user.QuickConnectRequest) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.InitiateQuickConnect")
	defer span.End()
	req.CreatedAt = time.Now()
	if err := s.quickConnect.Create(ctx, req); err != nil {
		return fmt.Errorf("AccessService.InitiateQuickConnect: %w", err)
	}
	return nil
}

// code identifies the pending QuickConnect request; userID identifies the
// already-authenticated caller approving it. Two adjacent bare strings here
// would compile if swapped - the same shape fixed in fcdef433d - and a swap
// would record the WRONG user as having approved the login, an authorization
// mixup, not just a lookup miss. userID is typed shared.UserID so that no
// longer compiles.
func (s *AccessService) AuthorizeQuickConnect(ctx context.Context, code string, userID shared.UserID) (fullCode string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.AuthorizeQuickConnect")
	defer span.End()
	req, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeQuickConnect: %w", err)
	}
	uid := string(userID)
	if err := s.quickConnect.UpdateStatus(ctx, req.ID, string(user.Authorized), &uid); err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeQuickConnect: %w", err)
	}
	return req.Code, nil
}

// See AuthorizeQuickConnect: same identity mixup risk between the request's
// short code and the approving user's id.
func (s *AccessService) AuthorizeByShortCode(ctx context.Context, shortCode string, userID shared.UserID) (fullCode string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.AuthorizeByShortCode")
	defer span.End()
	req, err := s.quickConnect.GetByShortCode(ctx, shortCode)
	if err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeByShortCode: %w", err)
	}
	uid := string(userID)
	if err := s.quickConnect.UpdateStatus(ctx, req.ID, string(user.Authorized), &uid); err != nil {
		return "", fmt.Errorf("AccessService.AuthorizeByShortCode: %w", err)
	}
	return req.Code, nil
}

func (s *AccessService) CreateAPIKey(ctx context.Context, key *user.APIKeyModel) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.CreateAPIKey")
	defer span.End()
	if err := key.Validate(); err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}
	key.CreatedAt = time.Now()
	if err := s.apiKeys.Create(ctx, key); err != nil {
		return fmt.Errorf("AccessService.CreateAPIKey: %w", err)
	}
	evt := user.APIKeyCreated{KeyID: key.ID, UserID: key.UserID, Name: key.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *AccessService) RevokeAPIKey(ctx context.Context, keyID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.RevokeAPIKey")
	defer span.End()
	if err := s.apiKeys.Delete(ctx, keyID); err != nil {
		return fmt.Errorf("AccessService.RevokeAPIKey: %w", err)
	}
	evt := user.APIKeyRevoked{KeyID: keyID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

// Les identifiants sont TYPÉS. C'était `(userID, libraryID string)` : deux
// chaînes voisines dans LE contrôle d'accès aux bibliothèques, appelé par 11
// handlers, donc les intervertir compilait sans un mot. Un contrôle d'accès
// inversé autorise en silence au lieu de refuser bruyamment.
func (s *AccessService) HasLibraryAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.HasLibraryAccess")
	defer span.End()
	return s.libraryAccess.HasAccess(ctx, userID, libraryID)
}

// GetLibraryAccessByUserID returns the list of library IDs the user may see.
// Empty slice (non-nil) means "user has zero granted libraries"; nil means
// "unrestricted".
//
// ⚠️ This method preserves that distinction, but EVERY caller deliberately
// collapses it again: playback_handler.go:1041, library_handler.go:952,
// collection_handler.go:354, notification_handler.go:262, playlist_handler.go:48,
// feed_handler.go:75 and music_handler.go:46 all treat an empty result as
// "unrestricted", because the product rule is opt-out - a user with no explicit
// grants sees everything.
//
// The comment that used to sit here warned that returning nil "silently
// promoted users with zero grants to admin-level visibility". That warning is
// now FALSE as written: the promotion is the intended behaviour and is applied
// uniformly by all seven callers. It is kept as this note rather than deleted,
// because a future reader who "restores the invariant" here would change the
// visibility rules of seven handlers at once without touching any of them.
func (s *AccessService) GetLibraryAccessByUserID(ctx context.Context, userID string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.GetLibraryAccessByUserID")
	defer span.End()
	ids, err := s.libraryAccess.GetByUserID(ctx, shared.UserID(userID))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out, nil
}

func (s *AccessService) GrantLibraryAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.GrantLibraryAccess")
	defer span.End()
	return s.libraryAccess.Grant(ctx, userID, libraryID)
}

func (s *AccessService) RevokeLibraryAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.RevokeLibraryAccess")
	defer span.End()
	return s.libraryAccess.Revoke(ctx, userID, libraryID)
}

// ReplaceLibraryAccess atomically swaps userID's whole library grant set for
// libraryIDs. Backs SetUserLibraryAccess so the operation can never leave the
// user revoked-from-everything with the new set never granted.
func (s *AccessService) ReplaceLibraryAccess(ctx context.Context, userID string, libraryIDs []string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.ReplaceLibraryAccess")
	defer span.End()
	ids := make([]shared.LibraryID, len(libraryIDs))
	for i, id := range libraryIDs {
		ids[i] = shared.LibraryID(id)
	}
	return s.libraryAccess.ReplaceAccess(ctx, shared.UserID(userID), ids)
}

// RevokeAllLibraryAccess atomically clears every library grant for userID in
// a single statement, backing RevokeAllUserLibraryAccess.
func (s *AccessService) RevokeAllLibraryAccess(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.RevokeAllLibraryAccess")
	defer span.End()
	return s.libraryAccess.DeleteByUserID(ctx, shared.UserID(userID))
}
