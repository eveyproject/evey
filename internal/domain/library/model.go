package library

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type LibraryType string

const (
	Movie  LibraryType = "movie"
	TVShow LibraryType = "tvshow"
	Music  LibraryType = "music"
	Photo  LibraryType = "photo"
)

type MediaItemType string

const (
	MovieItem MediaItemType = "movie"
	Series    MediaItemType = "series"
	Season    MediaItemType = "season"
	Episode   MediaItemType = "episode"
	Track     MediaItemType = "track"
	Album     MediaItemType = "album"
	PhotoItem MediaItemType = "photo"
)

type StreamType string

const (
	Video    StreamType = "video"
	Audio    StreamType = "audio"
	Subtitle StreamType = "subtitle"
)

type PersonRole string

const (
	Actor    PersonRole = "actor"
	Director PersonRole = "director"
	Writer   PersonRole = "writer"
	Producer PersonRole = "producer"
)

type Library struct {
	ID                  string
	Name                string
	Type                string
	Paths               []string
	ScanIntervalMinutes int
	Enabled             bool
	ScanOnStartup       bool
	RealtimeMonitoring  bool
	LastScannedAt       *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type MediaItem struct {
	ID              string
	LibraryID       string
	ParentID        string
	Type            string
	Title           string
	SortTitle       string
	OriginalTitle   string
	Overview        string
	Tagline         string
	FilePath        string
	FileSize        int64
	FileFingerprint string
	DurationMs      int64
	Bitrate         int64
	ContainerFormat string
	IndexNumber     *int
	CommunityRating *float64
	OfficialRating  string
	PremiereDate    *time.Time
	Year            *int
	ProviderIDs     map[string]string
	Genres          []string
	Studios         []string
	Tags            []string
	AddedAt         time.Time
	UpdatedAt       time.Time
}

type MediaStream struct {
	ID             string
	MediaItemID    string
	StreamIndex    int
	StreamType     string
	Codec          string
	CodecLongName  string
	Profile        string
	Language       string
	Title          string
	IsDefault      bool
	IsForced       bool
	Width          *int
	Height         *int
	Framerate      *float64
	BitDepth       *int
	ColorSpace     *string
	HDRType        *string
	Channels       *int
	ChannelLayout  *string
	SampleRate     *int
	Bitrate        *int64
	IsExternal     bool
	ExternalPath   *string
}

type Person struct {
	ID          string
	Name        string
	ProviderIDs map[string]string
	ThumbPath   *string
}

type MediaPerson struct {
	MediaItemID   string
	PersonID      string
	RoleType      string
	CharacterName *string
	SortOrder     int
}

type CollectionType string

const (
	CollectionManual CollectionType = "manual"
	CollectionSmart  CollectionType = "smart"
)

type RuleOperator string

const (
	OpEq       RuleOperator = "eq"
	OpNeq      RuleOperator = "neq"
	OpGt       RuleOperator = "gt"
	OpLt       RuleOperator = "lt"
	OpContains RuleOperator = "contains"
	OpIn       RuleOperator = "in"
)

type CollectionRule struct {
	Field    string       `json:"field"`
	Operator RuleOperator `json:"operator"`
	Value    string       `json:"value"`
}

type Collection struct {
	ID        string
	Name      string
	Overview  string
	Type      CollectionType
	Rules     []CollectionRule
	SortBy    string
	SortOrder string
	CreatedAt time.Time
	UpdatedAt time.Time
}
