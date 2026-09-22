package user

import (
	"errors"
	"net/mail"
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
	ID           shared.UserID
	Username     string
	Email        string
	PasswordHash string
	IsAdmin      bool
	Enabled      bool
	OIDCSubject  *string
	OIDCIssuer   *string
	MaxStreams   *int
	// MaxProfiles bounds the number of profiles this user can have.
	// nil / 0 means unlimited. Enforced at CreateProfile.
	MaxProfiles *int
	// MaxDevicesPerProfile bounds concurrent devices per profile.
	// nil / 0 means unlimited. Enforced at playback start.
	MaxDevicesPerProfile *int
	MaxBitrateKbps       int
	PushConsent          bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	// LastLogin is the timestamp of the most recent successful login (local or
	// OIDC). nil until the user has logged in at least once since the feature.
	LastLogin *time.Time
}

type ParentalSettings struct {
	MaxAge      *int // nil = no age filter. 0 means "only content rated for all ages".
	BlockedTags []string
	PINHash     string
	Enabled     bool
}

type Profile struct {
	ID          shared.ProfileID
	UserID      shared.UserID
	Name        string
	AvatarPath  *string
	Preferences map[string]string
	MaxRating   *string
	IsDefault   bool
	// IsKids marks a restricted profile intended for minors. When at least
	// one kids profile exists on the user, all non-kids profiles MUST have a
	// PIN set so switching to them from kids requires a challenge.
	IsKids            bool
	AudioLanguages    []string // ordered fallback list, e.g. ["fr","en"]
	SubtitleLanguages []string // ordered fallback list
	MaxBitrateKbps    *int
	Parental          *ParentalSettings
	Locale            string  // UI locale (en, fr, es, zh, hi)
	Region            *string // ISO 3166-1 alpha-2 (FR, BE, CH, SN, ...) - nil = resolve via server/locale fallback
	CreatedAt         time.Time
	// LastActiveAt is when an access token was last minted for this profile
	// (login or refresh). nil = never. Impersonation does not stamp it.
	LastActiveAt *time.Time
}

// HasPIN returns true when the profile has a PIN set (used by kids-profile
// invariant: every non-kids profile needs one when a kids profile exists).
// IsParentalRestricted reports whether the profile is subject to a parental
// content restriction: a kids profile always, and a non-kids profile whose
// owner explicitly enabled parental controls AND set a PIN. Leaving such a
// profile for an unrestricted one is PIN-gated (see the RefreshToken switch
// gate), otherwise the server-side content filter is bypassable by a plain
// profile switch.
func (p *Profile) IsParentalRestricted() bool {
	if p == nil {
		return false
	}
	if p.IsKids {
		return true
	}
	return p.EffectiveParental() != nil && p.HasPIN()
}

func (p *Profile) HasPIN() bool {
	return p != nil && p.Parental != nil && p.Parental.PINHash != ""
}

// DefaultKidsMaxAge is the implicit content-age cap applied to a profile
// flagged as IsKids when no explicit MaxAge is configured. Conservative
// fallback so the "kids" flag is never decorative - the alternative would
// be that a profile literally labelled as for children sees R-rated
// content as long as the admin forgot to flip the parental switch.
const DefaultKidsMaxAge = 12

// EffectiveParental returns the parental enforcement that should actually
// be applied to this profile at runtime. The returned struct is meant for
// read-only filtering decisions (rating cap, blocked tags); it does NOT
// represent persisted state. Returns nil when no enforcement applies.
//
// Rule: kids profiles always enforce, falling back to DefaultKidsMaxAge
// when no explicit cap is set. A non-kids profile enforces ONLY what an
// admin explicitly enabled and configured (a shared family profile with a
// PIN and an age cap) - without the explicit opt-in it stays unrestricted,
// and it never inherits the kids default cap. Ignoring the opt-in entirely
// was an audit HIGH: the PIN became pure UX with the server enforcing
// nothing for such profiles.
func (p *Profile) EffectiveParental() *ParentalSettings {
	if p == nil {
		return nil
	}
	if !p.IsKids {
		if p.Parental == nil || !p.Parental.Enabled {
			return nil
		}
		if p.Parental.MaxAge == nil && len(p.Parental.BlockedTags) == 0 {
			return nil // enabled but nothing configured: nothing to enforce
		}
		return &ParentalSettings{
			Enabled:     true,
			MaxAge:      p.Parental.MaxAge,
			BlockedTags: p.Parental.BlockedTags,
			PINHash:     p.Parental.PINHash,
		}
	}
	eff := ParentalSettings{Enabled: true}
	if p.Parental != nil {
		eff.BlockedTags = p.Parental.BlockedTags
		eff.PINHash = p.Parental.PINHash
		if p.Parental.MaxAge != nil {
			eff.MaxAge = p.Parental.MaxAge
		}
	}
	if eff.MaxAge == nil {
		def := DefaultKidsMaxAge
		eff.MaxAge = &def
	}
	return &eff
}

type RefreshToken struct {
	ID         string
	UserID     shared.UserID
	TokenHash  string
	DeviceName string
	ClientIP   *string
	// ProfileID is the profile that was active when this token was issued. nil
	// for legacy tokens (pre-binding), which fall back to the default profile.
	ProfileID *shared.ProfileID
	Used      bool
	// UsedAt is when the token was rotated (claimed). It bounds the rotation
	// grace window: a just-rotated token presented again within the grace is a
	// multi-tab race loser and gets a sibling session; past it, a replay. nil
	// on unused rows and on legacy rows claimed before the column existed.
	UsedAt *time.Time
	// SuccessorID is the row this token was rotated INTO. It is the breadcrumb
	// that tells an interrupted rotation apart from a replay: a client that
	// never received its new token comes back with this one, and the successor
	// is still unused. A thief replaying a token whose successor HAS been used
	// finds no such alibi. nil on unused rows and on rows rotated before the
	// column existed.
	SuccessorID *string
	ExpiresAt   time.Time
	CreatedAt   time.Time
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
	// Persistent records that the device being paired asked for a long-lived
	// session, at the moment it started the pairing. It is read when the
	// tokens are minted, because the call that collects them cannot always
	// say it: a browser collects over an event stream, and an EventSource
	// carries no headers.
	Persistent bool
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
	if strings.TrimSpace(u.Username) == "" {
		return errors.New("username is required")
	}
	// Surrounding whitespace is tolerated: rows written before this check
	// tightened may carry it, and refusing them would block the account's
	// next update until the operator notices a space.
	if e := strings.TrimSpace(u.Email); e != "" && !IsPlainAddress(e) {
		return errors.New("email is invalid")
	}
	return nil
}

// IsPlainAddress accepts one bare RFC 5322 address (no display name, no list,
// no folding): what the SMTP sender puts in a To: header verbatim.
func IsPlainAddress(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

// NewUser constructs a User with invariants enforced. Defaults: Enabled=true, IsAdmin=false.
// Caller may mutate IsAdmin/Enabled afterwards if needed.
// Surrounding whitespace is dropped: a phone keyboard appends a space after
// autocomplete, and Login trims, so an account stored with the space could
// never sign in.
func NewUser(username, email, passwordHash string) (*User, error) {
	now := time.Now()
	u := &User{
		ID:           shared.UserID(id.New("usr-")),
		Username:     strings.TrimSpace(username),
		Email:        strings.TrimSpace(email),
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
	if p.ID == "" {
		return errors.New("profile ID is required")
	}
	if p.UserID == "" {
		return errors.New("profile user ID is required")
	}
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	return nil
}

type ProfileOption func(*Profile)

func WithProfileAvatar(path string) ProfileOption {
	return func(p *Profile) { p.AvatarPath = &path }
}

func WithProfilePreferences(prefs map[string]string) ProfileOption {
	return func(p *Profile) { p.Preferences = prefs }
}

func WithProfileMaxRating(r string) ProfileOption {
	return func(p *Profile) { p.MaxRating = &r }
}

func WithProfileIsKids(kids bool) ProfileOption {
	return func(p *Profile) { p.IsKids = kids }
}

func WithProfileLocale(locale string) ProfileOption {
	return func(p *Profile) { p.Locale = locale }
}

func WithProfileRegion(region string) ProfileOption {
	return func(p *Profile) { p.Region = &region }
}

func WithProfileDefault(isDefault bool) ProfileOption {
	return func(p *Profile) { p.IsDefault = isDefault }
}

func WithProfileMaxBitrate(kbps int) ProfileOption {
	return func(p *Profile) { p.MaxBitrateKbps = &kbps }
}

func WithProfileParental(settings *ParentalSettings) ProfileOption {
	return func(p *Profile) { p.Parental = settings }
}

// NewProfile is the canonical constructor. ID is minted upstream
// (the gRPC handler owns the "prf-" prefix). Stamps CreatedAt and
// validates.
func NewProfile(id shared.ProfileID, userID shared.UserID, name string, opts ...ProfileOption) (*Profile, error) {
	p := &Profile{
		ID:        id,
		UserID:    userID,
		Name:      name,
		CreatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(p)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (inv *InviteLink) Validate() error {
	if inv.ID == "" {
		return errors.New("invite link ID is required")
	}
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

type InviteLinkOption func(*InviteLink)

func WithInviteLabel(label string) InviteLinkOption {
	return func(l *InviteLink) { l.Label = &label }
}

func WithInviteMaxUses(n int) InviteLinkOption {
	return func(l *InviteLink) { l.MaxUses = n }
}

func WithInviteAdmin(isAdmin bool) InviteLinkOption {
	return func(l *InviteLink) { l.IsAdmin = isAdmin }
}

func WithInviteLibraries(ids []shared.LibraryID) InviteLinkOption {
	return func(l *InviteLink) { l.LibraryIDs = ids }
}

func WithInviteExpiresAt(t *time.Time) InviteLinkOption {
	return func(l *InviteLink) { l.ExpiresAt = t }
}

// NewInviteLink is the canonical constructor. Identity (ID, token)
// is minted by the caller since token entropy + format are adapter
// concerns. Stamps CreatedAt and validates before return.
func NewInviteLink(id string, createdBy shared.UserID, token string, opts ...InviteLinkOption) (*InviteLink, error) {
	l := &InviteLink{
		ID:        id,
		CreatedBy: createdBy,
		Token:     token,
		CreatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(l)
	}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return l, nil
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
	// An empty scope list used to mean "allow-all" at the interceptor: a key
	// minted with no scopes was a fully unrestricted credential. Require at least
	// one scope so a new key is always bounded (defense in depth alongside the
	// interceptor's deny-by-default on admin methods).
	if len(k.Permissions) == 0 {
		return errors.New("API key requires at least one scope (an unscoped key would be unrestricted)")
	}
	return nil
}

// APIKeyOption customises an APIKeyModel at construction.
type APIKeyOption func(*APIKeyModel)

func WithAPIKeyPermissions(perms []string) APIKeyOption {
	return func(k *APIKeyModel) { k.Permissions = perms }
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

func (u *User) HasDeviceLimit() bool {
	return u.MaxDevicesPerProfile != nil && *u.MaxDevicesPerProfile > 0
}

// DeviceQuotaExceeded reports whether starting another session for a profile
// would exceed the user's per-profile concurrent-device quota. Each active
// PlaySession counts as one device.
func (u *User) DeviceQuotaExceeded(activeCount int) bool {
	return u.HasDeviceLimit() && activeCount >= *u.MaxDevicesPerProfile
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
