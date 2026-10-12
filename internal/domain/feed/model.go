package feed

import (
	"errors"
	"maps"
	"strings"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// Sentinel errors returned by the feed config constructors.
var (
	ErrInvalidFeedType  = errors.New("invalid feed type")
	ErrInvalidProfileID = errors.New("invalid profile ID")
	ErrNegativeMaxItems = errors.New("feed max items must not be negative")
)

type Type string

const (
	TypeContinueWatching Type = "continue_watching" // legacy, split into resume+next
	TypeRecentlyAdded    Type = "recently_added"
	TypeRecommended      Type = "recommended"
	TypeUpNext           Type = "up_next" // legacy, replaced by next_episode
	TypeMostPopular      Type = "most_popular"
	TypeCollections      Type = "collections"
	TypeResumePlayback   Type = "resume_playback"
	TypeNextEpisode      Type = "next_episode"
)

// knownTypes is the set of built-in feed types. Dynamic feeds (library:<id>,
// plugin:<id>) are validated by their prefix in IsValid, not this set.
var knownTypes = map[Type]bool{
	TypeContinueWatching: true,
	TypeRecentlyAdded:    true,
	TypeRecommended:      true,
	TypeUpNext:           true,
	TypeMostPopular:      true,
	TypeCollections:      true,
	TypeResumePlayback:   true,
	TypeNextEpisode:      true,
}

// IsValid reports whether t is a known built-in feed type or a well-formed
// dynamic one (library:<id> / plugin:<id>). A bare non-empty string is no
// longer accepted: a typo'd type ("reccently_added") must fail construction
// rather than silently persist a feed that renders nothing.
func (t Type) IsValid() bool {
	if knownTypes[t] {
		return true
	}
	return (t.IsLibrary() && t.LibraryID() != "") || (t.IsPlugin() && t.PluginID() != "")
}

func (t Type) IsLibrary() bool {
	return strings.HasPrefix(string(t), "library:")
}

func (t Type) LibraryID() string {
	if t.IsLibrary() {
		return strings.TrimPrefix(string(t), "library:")
	}
	return ""
}

func LibraryType(libraryID string) Type {
	return Type("library:" + libraryID)
}

// IsPlugin returns true for feed types that point at a frontend plugin
// rendering on the home page (slot "home.section"). Encoded as
// "plugin:<plugin_id>" so the customise-home menu can list them next to
// library and built-in feed types.
func (t Type) IsPlugin() bool {
	return strings.HasPrefix(string(t), "plugin:")
}

// PluginID returns the plugin id portion of a "plugin:<id>" feed type,
// or "" when the type is not a plugin feed.
func (t Type) PluginID() string {
	if t.IsPlugin() {
		return strings.TrimPrefix(string(t), "plugin:")
	}
	return ""
}

// PluginType builds the canonical feed type for a frontend plugin.
func PluginType(pluginID string) Type {
	return Type("plugin:" + pluginID)
}

// AdminConfig is the server-wide preset for a feed.
type AdminConfig struct {
	FeedType     Type
	Enabled      bool
	DisplayOrder int
	MaxItems     int
	Params       map[string]any
}

func (c *AdminConfig) Validate() error {
	if !c.FeedType.IsValid() {
		return ErrInvalidFeedType
	}
	if c.MaxItems < 0 {
		return ErrNegativeMaxItems
	}
	return nil
}

// AdminConfigOption tweaks an AdminConfig before validation.
type AdminConfigOption func(*AdminConfig)

// WithAdminEnabled toggles the server-wide enabled flag.
func WithAdminEnabled(enabled bool) AdminConfigOption {
	return func(c *AdminConfig) { c.Enabled = enabled }
}

// WithAdminDisplayOrder sets the display order index.
func WithAdminDisplayOrder(order int) AdminConfigOption {
	return func(c *AdminConfig) { c.DisplayOrder = order }
}

// WithAdminMaxItems caps the number of items returned by the feed.
func WithAdminMaxItems(maxItems int) AdminConfigOption {
	return func(c *AdminConfig) { c.MaxItems = maxItems }
}

// WithAdminParams attaches feed-specific parameters (cloned defensively).
func WithAdminParams(params map[string]any) AdminConfigOption {
	return func(c *AdminConfig) { c.Params = maps.Clone(params) }
}

// NewFeedAdminConfig is the canonical constructor for the server-wide feed
// preset. It enforces FeedType validity (sentinel ErrInvalidFeedType) and
// rejects negative MaxItems (ErrNegativeMaxItems) before returning the value.
func NewFeedAdminConfig(feedType Type, opts ...AdminConfigOption) (AdminConfig, error) {
	c := AdminConfig{FeedType: feedType}
	for _, opt := range opts {
		opt(&c)
	}
	if err := c.Validate(); err != nil {
		return AdminConfig{}, err
	}
	return c, nil
}

// ProfileConfig is an optional per-profile override.
// nil fields mean "use admin default".
type ProfileConfig struct {
	ProfileID    shared.ProfileID
	FeedType     Type
	Enabled      *bool
	DisplayOrder *int
	MaxItems     *int
	Params       map[string]any
}

func (c *ProfileConfig) Validate() error {
	if c.ProfileID == "" {
		return ErrInvalidProfileID
	}
	if !c.FeedType.IsValid() {
		return ErrInvalidFeedType
	}
	// Mirror AdminConfig.Validate: a negative per-profile override is as
	// invalid as a negative server-wide preset.
	if c.MaxItems != nil && *c.MaxItems < 0 {
		return ErrNegativeMaxItems
	}
	return nil
}

// ProfileConfigOption tweaks a ProfileConfig before validation.
// nil overrides keep "use admin default" semantics.
type ProfileConfigOption func(*ProfileConfig)

// WithProfileEnabled overrides the admin enabled flag.
func WithProfileEnabled(enabled bool) ProfileConfigOption {
	return func(c *ProfileConfig) { c.Enabled = &enabled }
}

// WithProfileDisplayOrder overrides the display order.
func WithProfileDisplayOrder(order int) ProfileConfigOption {
	return func(c *ProfileConfig) { c.DisplayOrder = &order }
}

// WithProfileMaxItems overrides the max items cap.
func WithProfileMaxItems(maxItems int) ProfileConfigOption {
	return func(c *ProfileConfig) { c.MaxItems = &maxItems }
}

// WithProfileParams attaches profile-specific params (cloned defensively).
func WithProfileParams(params map[string]any) ProfileConfigOption {
	return func(c *ProfileConfig) { c.Params = maps.Clone(params) }
}

// NewFeedProfileConfig is the canonical constructor for a per-profile feed
// override. It enforces a non-empty ProfileID (ErrInvalidProfileID) and a
// valid FeedType (ErrInvalidFeedType) before returning the value.
func NewFeedProfileConfig(feedType Type, profileID shared.ProfileID, opts ...ProfileConfigOption) (ProfileConfig, error) {
	c := ProfileConfig{
		ProfileID: profileID,
		FeedType:  feedType,
	}
	for _, opt := range opts {
		opt(&c)
	}
	if err := c.Validate(); err != nil {
		return ProfileConfig{}, err
	}
	return c, nil
}

// MergedConfig is the resolved config for a given profile+feed.
type MergedConfig struct {
	FeedType     Type
	Enabled      bool
	DisplayOrder int
	MaxItems     int
	Params       map[string]any
}

// FeedItem is a generic item returned by any feed.
type FeedItem struct {
	MediaItemID   shared.MediaItemID
	ParentID      string // for episodes: series ID
	Title         string
	Type          string
	Position      shared.Duration
	Duration      shared.Duration
	AddedAt       string
	Score         int
	SeasonNumber  int // episodes have both set; zero for other types
	EpisodeNumber int
	// PrimaryBlurhash is the encoded placeholder for the item's primary
	// artwork. Populated by the postgres adapter after the main feed query;
	// nil when no primary artwork exists or it pre-dates the blurhash field.
	PrimaryBlurhash *string
}
