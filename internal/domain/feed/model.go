package feed

import "errors"

type Type string

const (
	TypeContinueWatching Type = "continue_watching"
	TypeRecentlyAdded    Type = "recently_added"
	TypeRecommended      Type = "recommended"
)

func (t Type) IsValid() bool {
	return t == TypeContinueWatching || t == TypeRecentlyAdded || t == TypeRecommended
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
		return errors.New("invalid feed type")
	}
	if c.MaxItems < 0 {
		return errors.New("feed max items must not be negative")
	}
	return nil
}

// ProfileConfig is an optional per-profile override.
// nil fields mean "use admin default".
type ProfileConfig struct {
	ProfileID    string
	FeedType     Type
	Enabled      *bool
	DisplayOrder *int
	MaxItems     *int
	Params       map[string]any
}

func (c *ProfileConfig) Validate() error {
	if c.ProfileID == "" {
		return errors.New("feed profile config requires a profile ID")
	}
	if !c.FeedType.IsValid() {
		return errors.New("invalid feed type")
	}
	return nil
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
	MediaItemID string
	ParentID    string // for episodes: series ID
	Title       string
	Type        string
	PositionMs  int64
	AddedAt     string
	Score       int
}
