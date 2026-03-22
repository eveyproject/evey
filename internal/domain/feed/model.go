package feed

import (
	"errors"
	"strings"
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

func (t Type) IsValid() bool { return t != "" }

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
