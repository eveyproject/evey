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
	tokenRevoker  TokenRevoker  // optional: bumps token_epoch on security events
}

type UserServiceOption func(*UserService)

// WithDefaultLocale lets wire inject a function that returns the server-wide
// default locale (typically reading `server.language` from ServerConfig).
// Profile.Locale is set to this value at creation when empty.
func WithDefaultLocale(fn func() string) UserServiceOption {
	return func(s *UserService) { s.defaultLocale = fn }
}

// WithTokenRevoker wires access-token revocation (token_epoch bump) into the
// security-event mutations: disable, delete, role change, password change.
func WithTokenRevoker(rev TokenRevoker) UserServiceOption {
	return func(s *UserService) { s.tokenRevoker = rev }
}

// RevokeUserTokens invalidates every credential of the user: refresh tokens
// are deleted immediately and outstanding access tokens are revoked via the
// token_epoch bump. Used by the security mutations (DisableUser, ChangePassword,
// ResetPassword, role change) - NOT by KillSession, which only stops playback.
func (s *UserService) RevokeUserTokens(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.RevokeUserTokens")
	defer span.End()
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		return fmt.Errorf("UserService.RevokeUserTokens: %w", err)
	}
	s.revokeTokens(ctx, userID)
	return nil
}

// revokeTokens bumps the user's token_epoch (nil-safe). Best-effort like the
// neighbouring refresh-token deletes: the mutation itself already succeeded,
// and a failed bump only leaves the old 15min expiry window.
func (s *UserService) revokeTokens(ctx context.Context, userID string) {
	if s.tokenRevoker == nil {
		return
	}
	if err := s.tokenRevoker.Revoke(ctx, userID); err != nil {
		slog.WarnContext(ctx, "revoke access tokens", slog.String("userID", userID), slog.Any("err", err))
	}
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
	password = normalizePassword(password)
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

	p, err := user.NewProfile(shared.ProfileID(id.New("prf-")), u.ID, u.Username,
		user.WithProfileDefault(true),
		user.WithProfileLocale(s.resolveDefaultLocale()),
	)
	if err != nil {
		return fmt.Errorf("build default profile: %w", err)
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evtUserCreated.EventName()), slog.Any("err", err))
	}
	evtProfile := user.ProfileCreated{ProfileID: p.ID, UserID: p.UserID, Name: p.Name, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtProfile); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtProfile.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *UserService) UpdateUser(ctx context.Context, u *user.User) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.UpdateUser")
	defer span.End()
	// The invariant NewUser enforces at creation has to hold on every write, or
	// it only ever held for the first millisecond of the row's life: the gRPC
	// handler assigns req.Email straight onto the aggregate, so without this an
	// admin could park a syntactically invalid address on an existing account
	// through a path the create path forbids.
	if err := u.Validate(); err != nil {
		return fmt.Errorf("UserService.UpdateUser: %w", err)
	}
	// An admin account must never OWN a kids profile: its tokens carry is_admin,
	// which exempts the caller from the parental gate (checkParental / the gRPC
	// resolver), so a kids profile under an admin would stream unrestricted.
	// checkProfileInvariants enforces this when a PROFILE is written; enforce the
	// inverse here, or PROMOTING a user who already has a kids profile to admin
	// silently disables that profile's parental controls - the invariant held for
	// every profile write yet broke on the user write.
	if u.IsAdmin {
		profs, perr := s.profiles.GetByUserID(ctx, u.ID)
		if perr != nil {
			return fmt.Errorf("UserService.UpdateUser: load profiles for admin/kids invariant: %w", perr)
		}
		for _, p := range profs {
			if p != nil && p.IsKids {
				return ErrAdminCannotBeKids
			}
		}
	}
	// A role change must invalidate outstanding access tokens (the JWT carries
	// is_admin). Compare against the stored row to detect it.
	roleChanged := false
	if s.tokenRevoker != nil {
		before, err := s.users.GetByID(ctx, u.ID)
		if err != nil {
			// Can't determine whether is_admin changed - revoke to be safe rather
			// than risk leaving a just-demoted admin's is_admin token valid.
			roleChanged = true
		} else {
			roleChanged = before.IsAdmin != u.IsAdmin
		}
	}
	u.UpdatedAt = time.Now()
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("UserService.UpdateUser: %w", err)
	}
	if roleChanged {
		s.revokeTokens(ctx, string(u.ID))
	}
	return nil
}

func (s *UserService) SetUserBitrateLimit(ctx context.Context, userID string, maxBitrateKbps int) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.SetUserBitrateLimit")
	defer span.End()
	if err := setBitrateLimit(ctx, s.users, userID, maxBitrateKbps); err != nil {
		return fmt.Errorf("UserService.SetUserBitrateLimit: %w", err)
	}
	return nil
}

func (s *UserService) ChangePassword(ctx context.Context, userID, newPassword string) error {
	ctx, span := tracing.StartSpan(ctx, "app.user.ChangePassword")
	defer span.End()
	newPassword = normalizePassword(newPassword)
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
	// Password update and refresh-token eviction must commit together: a
	// surviving refresh token can be exchanged for a fresh access token, so a
	// swallowed delete would leave the password change bypassable (the epoch
	// bump below only revokes outstanding access tokens, not refresh tokens).
	apply := func(ctx context.Context) error {
		if err := s.users.Update(ctx, u); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
			return fmt.Errorf("delete refresh tokens: %w", err)
		}
		return nil
	}
	if s.uow != nil {
		if err := s.uow.Do(ctx, apply); err != nil {
			return fmt.Errorf("UserService.ChangePassword: %w", err)
		}
	} else if err := apply(ctx); err != nil {
		return fmt.Errorf("UserService.ChangePassword: %w", err)
	}
	s.revokeTokens(ctx, userID)
	evt := user.PasswordChanged{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
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
		// The guard and the delete are one serialized check-then-act: two
		// concurrent removals of distinct admins must not both pass a stale
		// count and leave zero enabled admins.
		if err := s.users.DeleteIfNotLastAdmin(ctx, shared.UserID(userID)); err != nil {
			if errors.Is(err, user.ErrLastEnabledAdmin) {
				return ErrLastAdmin
			}
			return fmt.Errorf("UserService.DeleteUser: %w", err)
		}
	} else if err := s.users.Delete(ctx, shared.UserID(userID)); err != nil {
		return fmt.Errorf("UserService.DeleteUser: %w", err)
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		slog.WarnContext(ctx, "delete refresh tokens for deleted user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	// No epoch bump: the row is gone, so the fail-closed epoch lookup already
	// rejects any outstanding token for this user.
	evtDeleted := user.UserDeleted{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
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
		// Atomic guard+disable - see DeleteUser.
		if err := s.users.DisableIfNotLastAdmin(ctx, shared.UserID(userID)); err != nil {
			if errors.Is(err, user.ErrLastEnabledAdmin) {
				return ErrLastAdmin
			}
			return fmt.Errorf("UserService.DisableUser: %w", err)
		}
	} else {
		u.Enabled = false
		u.UpdatedAt = time.Now()
		if err := s.users.Update(ctx, u); err != nil {
			return fmt.Errorf("UserService.DisableUser: %w", err)
		}
	}
	if err := s.refreshTkns.DeleteByUserID(ctx, shared.UserID(userID)); err != nil {
		slog.WarnContext(ctx, "delete refresh tokens for disabled user failed", slog.String("userID", userID), slog.Any("err", err))
	}
	s.revokeTokens(ctx, userID)
	evtDisabled := user.UserDisabled{UserID: shared.UserID(userID), OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtDisabled); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtDisabled.EventName()), slog.Any("err", err))
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evtProfileCreated.EventName()), slog.Any("err", err))
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
	// lookup succeeding - without the user we can't tell.
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
	// non-kids profile still lacks a PIN - otherwise we'd break the symmetry.
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return nil
}
