package user

import (
	"context"
	"time"
)

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByOIDCSubject(ctx context.Context, issuer, subject string) (*User, error)
	List(ctx context.Context) ([]*User, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
	CountAdmins(ctx context.Context) (int, error)
}

type ProfileRepository interface {
	Create(ctx context.Context, p *Profile) error
	GetByID(ctx context.Context, id string) (*Profile, error)
	GetByUserID(ctx context.Context, userID string) ([]*Profile, error)
	GetAll(ctx context.Context) ([]*Profile, error)
	Update(ctx context.Context, p *Profile) error
	Delete(ctx context.Context, id string) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, t *RefreshToken) error
	GetByTokenHash(ctx context.Context, hash string) (*RefreshToken, error)
	MarkUsed(ctx context.Context, id string) error
	DeleteByID(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) error
}

type APIKeyRepository interface {
	Create(ctx context.Context, key *APIKeyModel) error
	GetByKeyHash(ctx context.Context, hash string) (*APIKeyModel, error)
	ListByUserID(ctx context.Context, userID string) ([]*APIKeyModel, error)
	Delete(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string, t time.Time) error
}

type InviteLinkRepository interface {
	Create(ctx context.Context, link *InviteLink) error
	GetByToken(ctx context.Context, token string) (*InviteLink, error)
	ListByCreator(ctx context.Context, creatorID string) ([]*InviteLink, error)
	IncrementUsedCount(ctx context.Context, id string) error
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
	AssignRole(ctx context.Context, userID, roleID string) error
	RemoveRole(ctx context.Context, userID, roleID string) error
	GetRolesByUserID(ctx context.Context, userID string) ([]*Role, error)
}

type LibraryAccessRepository interface {
	Grant(ctx context.Context, userID, libraryID string) error
	Revoke(ctx context.Context, userID, libraryID string) error
	GetByUserID(ctx context.Context, userID string) ([]string, error)
	HasAccess(ctx context.Context, userID, libraryID string) (bool, error)
	ListUsersWithRestrictions(ctx context.Context) (map[string][]string, error)
}

type PasswordResetTokenRepository interface {
	Create(ctx context.Context, t *PasswordResetToken) error
	GetByTokenHash(ctx context.Context, hash string) (*PasswordResetToken, error)
	MarkUsed(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
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
	Permissions []string
}

type TokenGenerator interface {
	GenerateAccessToken(claims Claims) (string, error)
	GenerateRefreshToken() (string, error)
	ValidateAccessToken(token string) (*Claims, error)
}
