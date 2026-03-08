package feed

type Type string

const (
	TypeContinueWatching Type = "continue_watching"
	TypeRecentlyAdded    Type = "recently_added"
	TypeRecommended      Type = "recommended"
)

// AdminConfig is the server-wide preset for a feed.
type AdminConfig struct {
	FeedType     Type
	Enabled      bool
	DisplayOrder int
	MaxItems     int
	Params       map[string]interface{}
}

// ProfileConfig is an optional per-profile override.
// nil fields mean "use admin default".
type ProfileConfig struct {
	ProfileID    string
	FeedType     Type
	Enabled      *bool
	DisplayOrder *int
	MaxItems     *int
	Params       map[string]interface{}
}

// MergedConfig is the resolved config for a given profile+feed.
type MergedConfig struct {
	FeedType     Type
	Enabled      bool
	DisplayOrder int
	MaxItems     int
	Params       map[string]interface{}
}

// FeedItem is a generic item returned by any feed.
type FeedItem struct {
	MediaItemID string
	ParentID    string  // for episodes: series ID
	Title       string
	Type        string
	PositionMs  int64
	AddedAt     string
	Score       int
}
