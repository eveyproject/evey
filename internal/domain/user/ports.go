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

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id shared.UserID) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByOIDCSubject(ctx context.Context, issuer, subject string) (*User, error)
	List(ctx context.Context) ([]*User, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id shared.UserID) error
	CountAdmins(ctx context.Context) (int, error)
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
	Delete(ctx context.Context, id shared.ProfileID) error
	// SetDefault atomically clears is_default on every other profile owned
	// by userID and sets it on profileID. Returns ErrNotFound if profileID
	// does not exist or does not belong to userID.
	SetDefault(ctx context.Context, userID shared.UserID, profileID shared.ProfileID) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, t *RefreshToken) error
	GetByTokenHash(ctx context.Context, hash string) (*RefreshToken, error)
	// MarkUsedIfNotUsed atomically marks the token used only if it was unused.
	// Returns true if the token was claimed, false if it was already used.
	MarkUsedIfNotUsed(ctx context.Context, id string) (bool, error)
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
}
