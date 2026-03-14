package user

import "time"

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
	Status     string
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
