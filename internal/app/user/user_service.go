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

type UserService struct {
	users         user.UserRepository
	profiles      user.ProfileRepository
	refreshTkns   user.RefreshTokenRepository
	hasher        user.PasswordHasher
	events        shared.EventBus
	uow           shared.TransactionalUnit
	defaultLocale func() string // optional: resolves server.language, defaults to "en"
}

type UserServiceOption func(*UserService)

// WithDefaultLocale lets wire inject a function that returns the server-wide
// default locale (typically reading `server.language` from ServerConfig).
// Profile.Locale is set to this value at creation when empty.
func WithDefaultLocale(fn func() string) UserServiceOption {
	return func(s *UserService) { s.defaultLocale = fn }
}

func NewUserService(
	users user.UserRepository,
	profiles user.ProfileRepository,
	refreshTkns user.RefreshTokenRepository,
	hasher user.PasswordHasher,
	events shared.EventBus,
	uow shared.TransactionalUnit,
	opts ...UserServiceOption,
) *UserService {
	s := &UserService{
		users:       users,
		profiles:    profiles,
		refreshTkns: refreshTkns,
		hasher:      hasher,
		events:      events,
		uow:         uow,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// HashPIN validates a 4-digit PIN and returns its hash, suitable for
// assigning to Profile.Parental.PINHash before Create/UpdateProfile.
// Empty input returns an empty hash (used to clear a PIN).
func (s *UserService) HashPIN(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if len(plain) != 4 {
		return "", ErrInvalidPINFormat
	}
	for _, r := range plain {
		if r < '0' || r > '9' {
			return "", ErrInvalidPINFormat
		}
	}
	return s.hasher.Hash(plain)
}

func (s *UserService) resolveDefaultLocale() string {
	if s.defaultLocale == nil {
		return ""
	}
	return s.defaultLocale()
}

func (s *UserService) CreateUser(ctx context.Context, callerIsAdmin bool, u *user.User, password string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.CreateUser")
	defer span.End()
	if !callerIsAdmin {
		return ErrNotAdmin
	}
	if err := u.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()

	p := &user.Profile{
		ID:        shared.ProfileID(id.New("prf-")),
		UserID:    u.ID,
		Name:      u.Username,
		IsDefault: true,
		Locale:    s.resolveDefaultLocale(),
		CreatedAt: time.Now(),
	}

	if err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.users.Create(ctx, u); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if err := s.profiles.Create(ctx, p); err != nil {
			return fmt.Errorf("create default profile: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("UserService.CreateUser: %w", err)
	}

	evtUserCreated := user.UserCreated{UserID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtUserCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtUserCreated.EventName()), slog.Any("err", err))
	}
	evtProfile := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfile); err != nil {
		slog.Warn("publish event", slog.String("event", evtProfile.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) UpdateUser(ctx context.Context, u *user.User) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.UpdateUser")
	defer span.End()
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.UpdateUser: %w", err)
	}
	return nil
}

func (s *UserService) SetUserBitrateLimit(ctx context.Context, userID string, maxBitrateKbps int) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.SetUserBitrateLimit")
	defer span.End()
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("UserService.SetUserBitrateLimit: %w", err)
	}
	u.MaxBitrateKbps = maxBitrateKbps
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.SetUserBitrateLimit: %w", err)
	}
	return nil
}

func (s *UserService) ChangePassword(ctx context.Context, userID, newPassword string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.ChangePassword")
	defer span.End()
	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("UserService.ChangePassword: %w", err)
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.ChangePassword: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		slog.Warn("delete refresh tokens after password change", slog.String("userID", userID), slog.Any("err", err))
	}
	evt := user.PasswordChanged{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) DeleteUser(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.DeleteUser")
	defer span.End()
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("UserService.DeleteUser: %w", err)
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
	if err := s.users.Delete(ctx, shared.UserID(userID)); err != nil {
		return fmt.Errorf("UserService.DeleteUser: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		slog.Warn("delete refresh tokens for deleted user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	evtDeleted := user.UserDeleted{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.Warn("publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) DisableUser(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.DisableUser")
	defer span.End()
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("UserService.DisableUser: %w", err)
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
		return fmt.Errorf("UserService.DisableUser: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		slog.Warn("delete refresh tokens for disabled user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	evtDisabled := user.UserDisabled{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDisabled); err != nil {
		slog.Warn("publish event", slog.String("event", evtDisabled.EventName()), slog.Any("err", err))
	}
	return nil
}

// EnableUser re-enables a previously-disabled account. Idempotent if already
// enabled. Publishes UserEnabled for audit-trail symmetry with UserDisabled.
func (s *UserService) EnableUser(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.EnableUser")
	defer span.End()
	u, err := s.users.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return fmt.Errorf("UserService.EnableUser: %w", err)
	}
	u.Enabled = true
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.EnableUser: %w", err)
	}
	evt := user.UserEnabled{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) CreateProfile(ctx context.Context, p *user.Profile) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.CreateProfile")
	defer span.End()
	if err := p.Validate(); err != nil {
		return fmt.Errorf("invalid profile: %w", err)
	}
	if err := s.checkProfileInvariants(ctx, p, false); err != nil {
		return err
	}
	p.CreatedAt = time.Now()
	// The cap check above is advisory (it races with concurrent creates); the
	// authoritative, atomic enforcement happens in CreateWithinCap under a
	// per-user lock. maxProfiles<=0 (incl. missing user/cap) means unlimited.
	maxProfiles := 0
	if u, uErr := s.users.GetByID(ctx, p.UserID); uErr == nil && u != nil && u.MaxProfiles != nil {
		maxProfiles = *u.MaxProfiles
	}
	if err := s.profiles.CreateWithinCap(ctx, p, maxProfiles); err != nil {
		if errors.Is(err, user.ErrProfileCapReached) {
			return ErrMaxProfilesReached
		}
		return fmt.Errorf("UserService.CreateProfile: %w", err)
	}
	evtProfileCreated := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfileCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) UpdateProfile(ctx context.Context, p *user.Profile) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.UpdateProfile")
	defer span.End()
	if err := s.checkProfileInvariants(ctx, p, true); err != nil {
		return err
	}
	if err := s.profiles.Update(ctx, p); err != nil {
		return fmt.Errorf("UserService.UpdateProfile: %w", err)
	}
	return nil
}

// SetDefaultProfile atomically promotes profileID to be the default profile
// of its owning user. Caller is expected to be admin (RBAC handles that);
// the service only enforces the data invariant that exactly one profile per
// user holds is_default=true.
func (s *UserService) SetDefaultProfile(ctx context.Context, profileID shared.ProfileID) (*user.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.SetDefaultProfile")
	defer span.End()
	target, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("UserService.SetDefaultProfile: %w", err)
	}
	if err := s.profiles.SetDefault(ctx, target.UserID, target.ID); err != nil {
		return nil, fmt.Errorf("UserService.SetDefaultProfile: %w", err)
	}
	target.IsDefault = true
	return target, nil
}

//  1. User.MaxProfiles cap (create only, skipped for updates)
//  2. Kids invariant: when any kids profile exists on this user, every
//     non-kids profile must have a PIN set.
//
// isUpdate skips the profile-count check (updating an existing profile
// doesn't increase the count).
func (s *UserService) checkProfileInvariants(ctx context.Context, p *user.Profile, isUpdate bool) error {
	// Load user opportunistically to read MaxProfiles + IsAdmin. Missing
	// user is not fatal for the cap check (FK will reject the insert
	// downstream), but the admin/kids invariant is conditional on the
	// lookup succeeding — without the user we can't tell.
	var maxProfiles *int
	if u, err := s.users.GetByID(ctx, p.UserID); err == nil && u != nil {
		maxProfiles = u.MaxProfiles
		// Invariant: an admin owner cannot have a kids profile. Filtering
		// would otherwise gate the admin's own library, which is
		// semantically incoherent on single-user servers and a
		// foot-gun on multi-user ones.
		if u.IsAdmin && p.IsKids {
			return ErrAdminCannotBeKids
		}
	}
	existing, err := s.profiles.GetByUserID(ctx, p.UserID)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	if !isUpdate && maxProfiles != nil && *maxProfiles > 0 && len(existing) >= *maxProfiles {
		return ErrMaxProfilesReached
	}
	hasKidsSibling := false
	var pinlessNonKidsSibling *user.Profile
	for _, other := range existing {
		if string(other.ID) == string(p.ID) {
			continue // ignore self when updating
		}
		if other.IsKids {
			hasKidsSibling = true
		} else if !other.HasPIN() {
			pinlessNonKidsSibling = other
		}
	}
	// Invariant 1: the profile being created/updated, if non-kids, must have
	// a PIN when any kids sibling exists.
	if (hasKidsSibling || p.IsKids) && !p.IsKids && !p.HasPIN() {
		return ErrKidsRequiresPIN
	}
	// Invariant 2: we cannot create or flip-to-kids a profile while a sibling
	// non-kids profile still lacks a PIN — otherwise we'd break the symmetry.
	if p.IsKids && pinlessNonKidsSibling != nil {
		return ErrKidsRequiresPIN
	}
	return nil
}

func (s *UserService) DeleteProfile(ctx context.Context, profileID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.DeleteProfile")
	defer span.End()
	p, err := s.profiles.GetByID(ctx, shared.ProfileID(profileID))
	if err != nil {
		return fmt.Errorf("UserService.DeleteProfile: %w", err)
	}
	if p.IsDefault {
		return ErrCannotDeleteDefault
	}
	if err := s.profiles.Delete(ctx, shared.ProfileID(profileID)); err != nil {
		return fmt.Errorf("UserService.DeleteProfile: %w", err)
	}
	evt := user.ProfileDeleted{ProfileID: shared.ProfileID(profileID), UserID: p.UserID, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}
