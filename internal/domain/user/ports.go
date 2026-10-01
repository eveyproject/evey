package user

import (
	"context"
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// ErrProfileCapReached is returned by ProfileRepository.CreateWithinCap when the
// per-user profile cap would be exceeded. The app layer maps it onto its own
// caller-facing error.
var ErrProfileCapReached = errors.New("profile cap reached")

// ErrLastEnabledAdmin is returned by the guarded delete/disable when the
// target is the last enabled admin - removing it would lock everyone out.
var ErrLastEnabledAdmin = errors.New("last enabled admin")

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id shared.UserID) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByOIDCSubject(ctx context.Context, issuer, subject string) (*User, error)
	// LinkOIDCIdentity attaches (issuer, subject) to an account that has no SSO
	// identity yet. ErrConflict when the account already carries one or the
	// identity belongs to another account; ErrNotFound when the user is gone.
	LinkOIDCIdentity(ctx context.Context, id shared.UserID, issuer, subject string) error
	List(ctx context.Context) ([]*User, error)
	Update(ctx context.Context, u *User) error
	// TouchLastLogin stamps the user's last_login_at. Best-effort on the login
	// path - failure must not block the login itself.
	TouchLastLogin(ctx context.Context, id shared.UserID, at time.Time) error
	Delete(ctx context.Context, id shared.UserID) error
	// DeleteIfNotLastAdmin and DisableIfNotLastAdmin perform the last-admin
	// check and the mutation atomically (a serialized check-then-act): two
	// concurrent removals of distinct admins must not both pass a stale count
	// and leave zero enabled admins. They return ErrLastEnabledAdmin when the
	// target is an enabled admin and no other enabled admin remains. Deleting
	// or disabling an already-disabled admin is allowed regardless of count.
	DeleteIfNotLastAdmin(ctx context.Context, id shared.UserID) error
	DisableIfNotLastAdmin(ctx context.Context, id shared.UserID) error
	CountAdmins(ctx context.Context) (int, error)
	CountEnabledAdmins(ctx context.Context) (int, error)
}

type ProfileRepository interface {
	Create(ctx context.Context, p *Profile) error
	// CreateWithinCap inserts p only if the owning user has fewer than
	// maxProfiles profiles, enforcing the count and the insert atomically under
	// a per-user lock so concurrent creates cannot race past the cap.
	// maxProfiles <= 0 means unlimited. Returns ErrProfileCapReached when full.
	CreateWithinCap(ctx context.Context, p *Profile, maxProfiles int) error
	GetByID(ctx context.Context, id shared.ProfileID) (*Profile, error)
	GetByUserID(ctx context.Context, userID shared.UserID) ([]*Profile, error)
	GetAll(ctx context.Context) ([]*Profile, error)
	Update(ctx context.Context, p *Profile) error
	// UpdateAvatarPath writes ONLY avatar_path. Column-scoped by design (see
	// TokenEpochRepository above): a profile snapshot read before a concurrent
	// PIN write, then re-saved just to change its avatar, must not be able to
	// resurrect the pre-PIN parental_pin_hash it still holds in memory - that
	// full-row Update once erased a just-set parental PIN.
	UpdateAvatarPath(ctx context.Context, id shared.ProfileID, avatarPath *string) error
	// UpdateParentalSettings writes ONLY the parental_* columns - the mirror
	// image of UpdateAvatarPath: a PIN/age/tags/enabled change must not
	// overwrite a concurrently-changed avatar_path (or any other column)
	// sourced from a stale in-memory snapshot.
	UpdateParentalSettings(ctx context.Context, id shared.ProfileID, settings *ParentalSettings) error
	// UpdateSchedule writes ONLY the opening hours (nil clears them).
	UpdateSchedule(ctx context.Context, id shared.ProfileID, s *AccessSchedule) error
	// SetScheduleOverride writes ONLY how long a parent's PIN keeps a closed
	// profile open (nil clears it).
	SetScheduleOverride(ctx context.Context, id shared.ProfileID, until *time.Time) error
	Delete(ctx context.Context, id shared.ProfileID) error
	// SetDefault atomically clears is_default on every other profile owned
	// by userID and sets it on profileID. Returns ErrNotFound if profileID
	// does not exist or does not belong to userID.
	SetDefault(ctx context.Context, userID shared.UserID, profileID shared.ProfileID) error
	// TouchLastActive stamps last_active_at only. Best-effort on the token
	// paths (login, refresh): a failure is logged, never surfaced.
	TouchLastActive(ctx context.Context, id shared.ProfileID, at time.Time) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, t *RefreshToken) error
	GetByTokenHash(ctx context.Context, hash string) (*RefreshToken, error)
	GetByID(ctx context.Context, id string) (*RefreshToken, error)
	// MarkUsedIfNotUsed atomically marks the token used only if it was unused.
	// Returns true if the token was claimed, false if it was already used.
	MarkUsedIfNotUsed(ctx context.Context, id string) (bool, error)
	// SetSuccessor records which row a rotated token was exchanged for.
	SetSuccessor(ctx context.Context, id, successorID string) error
	DeleteByID(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID shared.UserID) error
	DeleteExpired(ctx context.Context) error
}

type APIKeyRepository interface {
	Create(ctx context.Context, key *APIKeyModel) error
	GetByKeyHash(ctx context.Context, hash string) (*APIKeyModel, error)
	ListByUserID(ctx context.Context, userID shared.UserID) ([]*APIKeyModel, error)
	Delete(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string, t time.Time) error
}

type InviteLinkRepository interface {
	Create(ctx context.Context, link *InviteLink) error
	GetByToken(ctx context.Context, token string) (*InviteLink, error)
	ListByCreator(ctx context.Context, creatorID string) ([]*InviteLink, error)
	IncrementUsedCount(ctx context.Context, id string) error
	DecrementUsedCount(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

type QuickConnectRepository interface {
	Create(ctx context.Context, req *QuickConnectRequest) error
	GetByCode(ctx context.Context, code string) (*QuickConnectRequest, error)
	GetByShortCode(ctx context.Context, shortCode string) (*QuickConnectRequest, error)
	UpdateStatus(ctx context.Context, id, status string, userID *string) error
	DeleteExpired(ctx context.Context) error
}

type RoleRepository interface {
	Create(ctx context.Context, role *Role) error
	GetByID(ctx context.Context, id string) (*Role, error)
	GetByName(ctx context.Context, name string) (*Role, error)
	List(ctx context.Context) ([]*Role, error)
}

type UserRoleRepository interface {
	AssignRole(ctx context.Context, userID shared.UserID, roleID string) error
	RemoveRole(ctx context.Context, userID shared.UserID, roleID string) error
	GetRolesByUserID(ctx context.Context, userID shared.UserID) ([]*Role, error)
}

type LibraryAccessRepository interface {
	Grant(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error
	GrantMany(ctx context.Context, userID shared.UserID, libraryIDs []shared.LibraryID) error
	Revoke(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error
	// ReplaceAccess atomically swaps every grant for userID for exactly
	// libraryIDs (delete-then-insert in one transaction). Callers that used to
	// revoke every existing grant in a loop then grant the new set in a second
	// loop had a real window where a crash or error between the two loops left
	// the user revoked from everything and re-granted nothing.
	ReplaceAccess(ctx context.Context, userID shared.UserID, libraryIDs []shared.LibraryID) error
	// DeleteByUserID atomically revokes every grant for userID in a single
	// statement, replacing a per-library revoke loop that a mid-loop failure
	// could leave partially applied.
	DeleteByUserID(ctx context.Context, userID shared.UserID) error
	GetByUserID(ctx context.Context, userID shared.UserID) ([]shared.LibraryID, error)
	HasAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) (bool, error)
	ListUsersWithRestrictions(ctx context.Context) (map[shared.UserID][]shared.LibraryID, error)
}

type PasswordResetTokenRepository interface {
	Create(ctx context.Context, t *PasswordResetToken) error
	GetByTokenHash(ctx context.Context, hash string) (*PasswordResetToken, error)
	MarkUsed(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID shared.UserID) error
	DeleteExpired(ctx context.Context) error
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) (bool, error)
}

type Claims struct {
	UserID      string
	Username    string
	IsAdmin     bool
	ProfileID   string
	Locale      string
	Permissions []string
	// ImpersonatedBy is the admin user id when this token was minted via
	// ImpersonateUser ("connect as"). Empty for normal sessions. The claims are
	// otherwise the TARGET user's (full power as them); this field only records
	// who is acting, for audit and a UI banner.
	ImpersonatedBy string
	// TokenEpoch is the user's revocation counter at mint time. A validator
	// comparing it against the current users.token_epoch can invalidate every
	// outstanding access token by bumping the counter (see TokenEpochRepository).
	// Tokens minted before the claim existed carry 0, matching the column default.
	TokenEpoch int64
}

func (c *Claims) HasPermission(perm string) bool {
	for _, p := range c.Permissions {
		if p == perm || p == "*" {
			return true
		}
	}
	return false
}

type TokenGenerator interface {
	GenerateAccessToken(claims Claims) (string, error)
	GenerateRefreshToken() (string, error)
	ValidateAccessToken(token string) (*Claims, error)
	// GeneratePINToken mints a short-lived proof that the caller entered the
	// given profile's parental PIN; ParsePINToken validates it and returns the
	// profile id. Used to gate switching out of a kids profile.
	GeneratePINToken(profileID string, ttl time.Duration) (string, error)
	ParsePINToken(raw string) (string, error)
	// DescribeRefreshToken names a refresh token for a log line WITHOUT
	// trusting it - its id and its issue/expiry times as the token itself
	// claims them - or returns "" when the string is not one of ours. A
	// refresh refused as "not found" used to be undiagnosable: a stale copy of
	// a token rotated 20 minutes earlier (swept from the store) and a token
	// forged elsewhere printed the same eight characters. Live 2026-08-31 a
	// phone lost its session mid-cast on exactly that line.
	DescribeRefreshToken(raw string) string
}

// TokenEpochRepository is the narrow slice of user storage backing access-token
// revocation. Kept separate from UserRepository so its many fakes stay
// untouched and so token_epoch can only be mutated through the atomic bump
// (never overwritten by a stale Update).
type TokenEpochRepository interface {
	GetTokenEpoch(ctx context.Context, id shared.UserID) (int64, error)
	// BumpTokenEpoch atomically increments the counter, invalidating every
	// access token minted with the previous value.
	BumpTokenEpoch(ctx context.Context, id shared.UserID) error
}
