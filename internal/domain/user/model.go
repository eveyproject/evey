package user

import (
	"errors"
	"time"
)

type QuickConnectStatus string

const (
	Pending    QuickConnectStatus = "pending"
	Authorized QuickConnectStatus = "authorized"
	Expired    QuickConnectStatus = "expired"
)

type User struct {
	ID               string
	Username         string
	Email            string
	PasswordHash     string
	IsAdmin          bool
	Enabled          bool
	OIDCSubject      *string
	OIDCIssuer       *string
	MaxStreams        *int
	MaxBitrateKbps   int
	PushConsent      bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ParentalSettings struct {
	MaxRating   string
	BlockedTags []string
	PINHash     string
	Enabled     bool
}

type Profile struct {
	ID                string
	UserID            string
	Name              string
	AvatarPath        *string
	Preferences       map[string]string
	MaxRating         *string
	IsDefault         bool
	AudioLanguages    []string // ordered fallback list, e.g. ["fr","en"]
	SubtitleLanguages []string // ordered fallback list
	MaxBitrateKbps    *int
	Parental          *ParentalSettings
	Locale            string // UI locale (en, fr, es, zh, hi)
	CreatedAt         time.Time
}

type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string
	DeviceName string
	ClientIP   *string
	Used       bool
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

type APIKeyModel struct {
	ID          string
	UserID      string
	KeyHash     string
	Name        string
	Permissions []string
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
	CreatedAt   time.Time
}

type InviteLink struct {
	ID        string
	CreatedBy string
	Token     string
	Label     *string
	MaxUses   int
	UsedCount int
	IsAdmin   bool
	ExpiresAt *time.Time
	CreatedAt time.Time
}

type QuickConnectRequest struct {
	ID         string
	Code       string
	Secret     string
	UserID     *string
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
	UserID    string
	LibraryID string
	GrantedAt time.Time
}

func (u *User) Validate() error {
	if u.Username == "" {
		return errors.New("username is required")
	}
	return nil
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

func (u *User) IsOIDCUser() bool {
	return u.OIDCSubject != nil && u.OIDCIssuer != nil
}

func (u *User) HasStreamsLimit() bool {
	return u.MaxStreams != nil && *u.MaxStreams > 0
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
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

func (t *PasswordResetToken) IsValid() bool {
	return !t.Used && time.Now().Before(t.ExpiresAt)
}
