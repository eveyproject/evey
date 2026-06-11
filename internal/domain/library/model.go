package library

import (
	"errors"
	"fmt"
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

func (t LibraryType) IsValid() bool {
	switch t {
	case Movie, TVShow, Music, Photo:
		return true
	}
	return false
}

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

func (t MediaItemType) IsValid() bool {
	switch t {
	case MovieItem, Series, Season, Episode, Track, Album, PhotoItem:
		return true
	}
	return false
}

type StreamType string

const (
	Video    StreamType = "video"
	Audio    StreamType = "audio"
	Subtitle StreamType = "subtitle"
)

func (t StreamType) IsValid() bool {
	switch t {
	case Video, Audio, Subtitle:
		return true
	}
	return false
}

type PersonRole string

const (
	Actor    PersonRole = "actor"
	Director PersonRole = "director"
	Writer   PersonRole = "writer"
	Producer PersonRole = "producer"
)

func (r PersonRole) IsValid() bool {
	switch r {
	case Actor, Director, Writer, Producer:
		return true
	}
	return false
}

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

// LibraryOption customises a Library at construction. Timestamps + ID
// are owned by NewLibrary; options cover the non-required fields.
type LibraryOption func(*Library)

func WithLibraryPaths(paths []string) LibraryOption {
	return func(l *Library) { l.Paths = paths }
}

func WithLibraryScanInterval(minutes int) LibraryOption {
	return func(l *Library) { l.ScanIntervalMinutes = minutes }
}

func WithLibraryScanOnStartup(b bool) LibraryOption {
	return func(l *Library) { l.ScanOnStartup = b }
}

func WithLibraryRealtimeMonitoring(b bool) LibraryOption {
	return func(l *Library) { l.RealtimeMonitoring = b }
}

// NewLibrary is the canonical constructor: stamps CreatedAt/UpdatedAt,
// enables the library by default, applies options, then validates.
// Callers should prefer this over &Library{...} literals so invariants
// are enforced at construction rather than discovered at Create time.
func NewLibrary(id shared.LibraryID, name string, libType LibraryType, opts ...LibraryOption) (*Library, error) {
	now := time.Now()
	l := &Library{
		ID:        id,
		Name:      name,
		Type:      libType,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, opt := range opts {
		opt(l)
	}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Library) Validate() error {
	if l.ID == "" {
		return errors.New("library requires an ID")
	}
	if l.Name == "" {
		return errors.New("library requires a name")
	}
	if !l.Type.IsValid() {
		return fmt.Errorf("library: invalid type %q", l.Type)
	}
	return nil
}

type MediaItem struct {
	ID        shared.MediaItemID
	LibraryID shared.LibraryID
	ParentID  string
	Type      MediaItemType
	Title     string
	// ScanTitle is the title extracted from the filesystem at scan time —
	// the stable source of truth for metadata lookups. Set once at
	// creation and never overwritten by enrichment, so a wrong TMDB match
	// doesn't self-reinforce on re-enrichment.
	ScanTitle     string
	SortTitle     string
	OriginalTitle string
	Overview      string
	Tagline       string
	IndexNumber   *int

	// File carries the on-disk artefact (path, size, container, fingerprint,
	// runtime). nil for container types (series, season, album) that own
	// no file. Populated by the postgres repo via LEFT JOIN on media_files.
	File *MediaFile

	// Classification carries audience/editorial rating data (community
	// score, official certification, per-country certifications, per-
	// provider critic scores). nil when no provider has scored the item
	// and the admin hasn't set a manual rating.
	Classification *MediaClassification

	// Catalog carries editorial/discovery metadata (release date, year,
	// per-provider IDs, taxonomy: genres/studios/tags). nil for items
	// the scan created but enrichment never touched.
	Catalog *MediaCatalog

	Trailers  []Trailer
	AddedAt   time.Time
	UpdatedAt time.Time
}

// MediaCatalog carries discovery metadata sourced from providers: the
// release/premiere date and year, the per-provider ID map (tmdb,imdb,...)
// used to look the item back up, and the taxonomy (genres, studios,
// tags) used by filters and recommendations.
type MediaCatalog struct {
	PremiereDate *time.Time
	Year         *int
	ProviderIDs  map[string]string
	Genres       []string
	Studios      []string
	Tags         []string
}

// MediaClassification holds rating-style data: the audience score
// (community), the editorial certification (official + per-country) and
// the per-provider critic scores. Distinct from per-user ratings, which
// live in the media_ratings table.
type MediaClassification struct {
	CommunityRating         *float64
	OfficialRating          string
	CertificationsByCountry map[string]string
	// ProviderRatings keyed by provider name (tmdb, imdb, rotten, ...) → score on each provider's own scale.
	ProviderRatings map[string]float64
}

// MediaFile carries everything the scan and the player need to know
// about the on-disk artefact backing a MediaItem. Modelled as a
// sub-aggregate so non-file callers (catalog metadata, ratings) don't
// have to thread file fields they never touch.
type MediaFile struct {
	Path            string
	Size            int64
	Fingerprint     string
	ModTime         time.Time
	Duration        shared.Duration
	Bitrate         int64
	ContainerFormat string
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
	if !m.Type.IsValid() {
		return fmt.Errorf("media item: invalid type %q", m.Type)
	}
	if m.Title == "" {
		return errors.New("media item requires a title")
	}
	return nil
}

func (m *MediaItem) HasFile() bool {
	return m.File != nil && m.File.Path != ""
}

func (m *MediaItem) IsPlayable() bool {
	return m.HasFile() && (m.Type == MovieItem || m.Type == Episode || m.Type == Track)
}

type MediaStream struct {
	ID             string
	MediaItemID    shared.MediaItemID
	StreamIndex    int
	StreamType     StreamType
	Codec          string
	CodecLongName  string
	Profile        string
	// Level is the codec level (e.g. H.264 level 4.0 = 40), nil when unknown.
	// Needed with Profile to build the exact RFC-6381 CODECS string for a
	// video-copy rung; a nil Level gates copy off (no accurate codec advertise).
	Level          *int
	Language       string
	Title          string
	IsDefault      bool
	IsForced       bool
	Width          *int
	Height         *int
	Framerate      *float64
	BitDepth       *int
	ColorSpace     *string
	// ColorTransfer is the ffprobe color_transfer (smpte2084 = PQ, arib-std-b67
	// = HLG, bt709 = SDR…), nil when the probe didn't report it. The only field
	// that tells PQ from HLG, and — with DoViProfile — the HDR ground truth the
	// copy/direct-play decision needs. A nil value on an HEVC stream gates the
	// HEVC copy rung off until a re-probe backfills it (same pattern as Level).
	ColorTransfer  *string
	HDRType        *string
	// DoViProfile / DoViBLCompat mirror the Dolby Vision configuration record
	// (ffprobe side_data dv_profile + dv_bl_signal_compatibility_id), nil when
	// the stream carries no DoVi RPU. Profile 5 must always transcode (IPTC2
	// base layer renders wrong on non-DoVi pipelines); profile 8 with a
	// compatible base layer (1=HDR10, 2=SDR, 4=HLG) may be served as its base.
	DoViProfile    *int
	DoViBLCompat   *int
	Channels       *int
	ChannelLayout  *string
	SampleRate     *int
	Bitrate        *int64
	IsExternal     bool
	ExternalPath   *string
	// Keyframes holds the video stream's keyframe presentation times in
	// milliseconds (sorted ascending), or nil when not yet indexed. These are the
	// only valid segment-start boundaries for a -c:v copy (remux) rung — a copied
	// segment cannot begin mid-GOP. Populated by a background backfill, not the
	// scan (the keyframe probe is too costly for the hot scan path). Video only.
	Keyframes      []int64
}

func (s *MediaStream) GetStreamType() string      { return string(s.StreamType) }
func (s *MediaStream) GetBitDepth() *int          { return s.BitDepth }
func (s *MediaStream) GetColorSpace() *string     { return s.ColorSpace }
func (s *MediaStream) GetColorTransfer() *string  { return s.ColorTransfer }
func (s *MediaStream) GetHDRType() *string        { return s.HDRType }
func (s *MediaStream) GetProfile() string         { return s.Profile }
func (s *MediaStream) GetLevel() *int             { return s.Level }
func (s *MediaStream) GetDoViProfile() *int       { return s.DoViProfile }
func (s *MediaStream) GetDoViBLCompat() *int      { return s.DoViBLCompat }

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

// MediaSegment represents a detected time range within a media item
// (e.g. intro, outro, recap, preview). Type is one of "intro", "outro",
// "recap", "preview". Source identifies the detection method.
type MediaSegment struct {
	ID          string
	MediaItemID shared.MediaItemID
	Type        string
	StartMs     int64
	EndMs       int64
	Confidence  float64
	Source      string
	CreatedAt   time.Time
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
