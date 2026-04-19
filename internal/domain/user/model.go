package user

import (
	"errors"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
)

type QuickConnectStatus string

const (
	Pending    QuickConnectStatus = "pending"
	Authorized QuickConnectStatus = "authorized"
	Expired    QuickConnectStatus = "expired"
)

func (s QuickConnectStatus) IsValid() bool {
	switch s {
	case Pending, Authorized, Expired:
		return true
	}
	return false
}

type User struct {
	ID               shared.UserID
	Username         string
	Email            string
	PasswordHash     string
	IsAdmin          bool
	Enabled          bool
	OIDCSubject      *string
	OIDCIssuer       *string
	MaxStreams        *int
	// MaxProfiles bounds the number of profiles this user can have.
	// nil / 0 means unlimited. Enforced at CreateProfile.
	MaxProfiles      *int
	// MaxDevicesPerProfile bounds concurrent devices per profile.
	// nil / 0 means unlimited. Enforced at playback start.
	MaxDevicesPerProfile *int
	MaxBitrateKbps   int
	PushConsent      bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ParentalSettings struct {
	MaxAge      *int // nil = no age filter. 0 means "only content rated for all ages".
	BlockedTags []string
	PINHash     string
	Enabled     bool
}

type Profile struct {
	ID                shared.ProfileID
	UserID            shared.UserID
	Name              string
	AvatarPath        *string
	Preferences       map[string]string
	MaxRating         *string
	IsDefault         bool
	// IsKids marks a restricted profile intended for minors. When at least
	// one kids profile exists on the user, all non-kids profiles MUST have a
	// PIN set so switching to them from kids requires a challenge.
	IsKids            bool
	AudioLanguages    []string // ordered fallback list, e.g. ["fr","en"]
	SubtitleLanguages []string // ordered fallback list
	MaxBitrateKbps    *int
	Parental          *ParentalSettings
	Locale            string  // UI locale (en, fr, es, zh, hi)
	Region            *string // ISO 3166-1 alpha-2 (FR, BE, CH, SN, ...) — nil = resolve via server/locale fallback
	CreatedAt         time.Time
}

// HasPIN returns true when the profile has a PIN set (used by kids-profile
// invariant: every non-kids profile needs one when a kids profile exists).
func (p *Profile) HasPIN() bool {
	return p != nil && p.Parental != nil && p.Parental.PINHash != ""
}

type RefreshToken struct {
	ID         string
	UserID     shared.UserID
	TokenHash  string
	DeviceName string
	ClientIP   *string
	Used       bool
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

type APIKeyModel struct {
	ID          string
	UserID      shared.UserID
	KeyHash     string
	Name        string
	Permissions []string
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
	CreatedAt   time.Time
}

type InviteLink struct {
	ID         string
	CreatedBy  shared.UserID
	Token      string
	Label      *string
	MaxUses    int
	UsedCount  int
	IsAdmin    bool
	LibraryIDs []shared.LibraryID
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

type QuickConnectRequest struct {
	ID         string
	Code       string
	Secret     string
	UserID     *shared.UserID
	DeviceName *string
	Status     QuickConnectStatus
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

type Role struct {
	ID          string
	Name        string
	Description string
	Permissions []string
	IsSystem    bool
}

type LibraryAccess struct {
	UserID    shared.UserID
	LibraryID shared.LibraryID
	GrantedAt time.Time
}

func (u *User) Validate() error {
	if u.Username == "" {
		return errors.New("username is required")
	}
	if u.Email != "" && !strings.Contains(u.Email, "@") {
		return errors.New("email is invalid")
	}
	return nil
}

// NewUser constructs a User with invariants enforced. Defaults: Enabled=true, IsAdmin=false.
// Caller may mutate IsAdmin/Enabled afterwards if needed.
func NewUser(username, email, passwordHash string) (*User, error) {
	now := time.Now()
	u := &User{
		ID:           shared.UserID(id.New("usr-")),
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.Validate(); err != nil {
		return nil, err
	}
	return u, nil
}

func (p *Profile) Validate() error {
	if p.UserID == "" {
		return errors.New("profile user ID is required")
	}
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	return nil
}

func (inv *InviteLink) Validate() error {
	if inv.CreatedBy == "" {
		return errors.New("invite link created_by is required")
	}
	if inv.Token == "" {
		return errors.New("invite link token is required")
	}
	if inv.MaxUses < 0 {
		return errors.New("invite link max uses must not be negative")
	}
	return nil
}

func (k *APIKeyModel) Validate() error {
	if k.ID == "" {
		return errors.New("API key ID is required")
	}
	if k.UserID == "" {
		return errors.New("API key user ID is required")
	}
	if k.Name == "" {
		return errors.New("API key name is required")
	}
	if k.KeyHash == "" {
		return errors.New("API key hash is required")
	}
	return nil
}

// APIKeyOption customises an APIKeyModel at construction.
type APIKeyOption func(*APIKeyModel)

func WithAPIKeyPermissions(perms []string) APIKeyOption {
	return func(k *APIKeyModel) { k.Permissions = perms }
}

func WithAPIKeyExpiresAt(t *time.Time) APIKeyOption {
	return func(k *APIKeyModel) { k.ExpiresAt = t }
}

// NewAPIKey is the canonical constructor. Callers mint the id
// (`ak-*`) and hash the raw key outside the domain (hashing depends
// on the crypto policy owned by the handler). Stamps CreatedAt and
// validates before return.
func NewAPIKey(id string, userID shared.UserID, name, keyHash string, opts ...APIKeyOption) (*APIKeyModel, error) {
	k := &APIKeyModel{
		ID:        id,
		UserID:    userID,
		Name:      name,
		KeyHash:   keyHash,
		CreatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(k)
	}
	if err := k.Validate(); err != nil {
		return nil, err
	}
	return k, nil
}

func (u *User) IsOIDCUser() bool {
	return u.OIDCSubject != nil && u.OIDCIssuer != nil
}

func (u *User) HasStreamsLimit() bool {
	return u.MaxStreams != nil && *u.MaxStreams > 0
}

// StreamQuotaExceeded reports whether starting another stream would exceed the
// user's concurrent-stream quota. Users without a configured limit are never
// over quota.
func (u *User) StreamQuotaExceeded(activeCount int) bool {
	return u.HasStreamsLimit() && activeCount >= *u.MaxStreams
}

func (u *User) CanLogin() bool {
	return u.Enabled
}

func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}

func (rt *RefreshToken) IsValid() bool {
	return !rt.Used && !rt.IsExpired()
}

func (inv *InviteLink) IsUsable() bool {
	if inv.ExpiresAt != nil && time.Now().After(*inv.ExpiresAt) {
		return false
	}
	if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
		return false
	}
	return true
}

func (k *APIKeyModel) IsExpired() bool {
	return k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt)
}

func (k *APIKeyModel) HasPermission(perm string) bool {
	for _, p := range k.Permissions {
		if p == perm || p == "*" {
			return true
		}
	}
	return false
}

type PasswordResetToken struct {
	ID        string
	UserID    shared.UserID
	TokenHash string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

func (t *PasswordResetToken) IsValid() bool {
	return !t.Used && time.Now().Before(t.ExpiresAt)
}
