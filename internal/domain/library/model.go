package library

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

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
	ID                  shared.LibraryID
	Name                string
	Type                LibraryType
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
	ID              shared.MediaItemID
	LibraryID       shared.LibraryID
	ParentID        string
	Type            MediaItemType
	Title           string
	SortTitle       string
	OriginalTitle   string
	Overview        string
	Tagline         string
	FilePath        string
	FileSize        int64
	FileFingerprint string
	ModTime         time.Time
	DurationMs      int64
	Bitrate         int64
	ContainerFormat string
	IndexNumber     *int
	CommunityRating *float64
	OfficialRating  string
	// CertificationsByCountry holds per-country content ratings (e.g. {"US":"PG-13","FR":"-12"}).
	// OfficialRating mirrors the US entry when available for parental-gate backcompat.
	CertificationsByCountry map[string]string
	PremiereDate            *time.Time
	Year            *int
	ProviderIDs     map[string]string
	Ratings         map[string]float64
	Genres          []string
	Studios         []string
	Tags            []string
	Trailers        []Trailer
	AddedAt         time.Time
	UpdatedAt       time.Time
}

type Trailer struct {
	Site             string
	Key              string
	Language         string
	Official         bool
	Type             string
	UseSubtitles     bool
	SubtitleLanguage string
}

// MediaItemScanRow is a lightweight projection for scan comparisons (avoids loading JSONB columns).
type MediaItemScanRow struct {
	ID              string
	FilePath        string
	FileFingerprint string
	FileSize        int64
	ModTime         time.Time
}

// MediaItemContainerRow is a lightweight projection for pruning empty containers.
type MediaItemContainerRow struct {
	ID       string
	ParentID string
	Type     string
}

func (m *MediaItem) Validate() error {
	if m.LibraryID == "" {
		return errors.New("media item requires a library ID")
	}
	if m.Type == "" {
		return errors.New("media item requires a type")
	}
	if m.Title == "" {
		return errors.New("media item requires a title")
	}
	return nil
}

func (m *MediaItem) HasFile() bool {
	return m.FilePath != ""
}

func (m *MediaItem) IsPlayable() bool {
	return m.HasFile() && (m.Type == MovieItem || m.Type == Episode || m.Type == Track)
}

type MediaStream struct {
	ID             string
	MediaItemID    shared.MediaItemID
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

func (s *MediaStream) GetStreamType() string  { return s.StreamType }
func (s *MediaStream) GetBitDepth() *int       { return s.BitDepth }
func (s *MediaStream) GetColorSpace() *string  { return s.ColorSpace }
func (s *MediaStream) GetHDRType() *string     { return s.HDRType }

type Person struct {
	ID           string
	Name         string
	ProviderIDs  map[string]string
	ThumbPath    *string
	Biography    string
	Birthday     string
	Deathday     string
	PlaceOfBirth string
	KnownFor     string
}

type MediaPerson struct {
	MediaItemID   shared.MediaItemID
	PersonID      string
	RoleType      string
	CharacterName *string
	SortOrder     int
}

type Chapter struct {
	ID          string
	MediaItemID shared.MediaItemID
	Title       string
	StartTimeMs int64
	EndTimeMs   int64
	SortOrder   int
}

type ScanSchedule struct {
	LibraryID      shared.LibraryID
	CronExpression string
	Enabled        bool
	LastRunAt      *time.Time
	NextRunAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CollectionRule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type Collection struct {
	ID          string
	TmdbID      *int
	Name        string
	Overview    string
	PosterURL   string
	BackdropURL string
	Rules       []CollectionRule
	SortBy      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MediaItemTranslation struct {
	MediaItemID shared.MediaItemID
	Locale      string
	Title       string
	Overview    string
	Tagline     string
}

type PersonTranslation struct {
	PersonID  string
	Locale    string
	Biography string
}

type TrickplayManifest struct {
	ID          string
	MediaItemID shared.MediaItemID
	Interval    int // seconds between frames
	Width       int
	Height      int
	TileWidth   int
	TileHeight  int
	ImageCount  int
	ImagePath   string // directory containing sprite images
	CreatedAt   time.Time
}
