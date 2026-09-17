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
	// ScanTitle is the title extracted from the filesystem at scan time -
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

	Trailers []Trailer
	// Translations is a carry-along of the item's per-locale titles/overviews,
	// NOT loaded by the default row scan (it lives in a separate table). The
	// search-indexing paths populate it before indexing so a user searching in
	// their own language finds a title only stored in a translation.
	Translations []MediaItemTranslation
	// CastNames is a carry-along of the people names attached to the item,
	// NOT loaded by the default row scan (it lives in media_people/people).
	// The search-indexing paths populate it before indexing so an actor-name
	// search keeps matching after a reindex/reconcile/drain rebuild.
	CastNames []string
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
// BitrateBps is the container's overall bitrate in bits per second: the stored
// value when the probe supplied one, otherwise derived from size and duration.
//
// The derivation is not an approximation of a different quantity - ffprobe's
// format-level bit_rate IS total bytes over total time, which is exactly what
// this computes. It exists because nothing ever populated MediaFile.Bitrate:
// the probers report per-TRACK bitrates (a Matroska BPS tag many files simply
// lack - 19% coverage on a real library), never a container-level one, so the
// column sat at 0 for every row.
func (f *MediaFile) BitrateBps() int64 {
	if f == nil {
		return 0
	}
	if f.Bitrate > 0 {
		return f.Bitrate
	}
	if secs := f.Duration.Seconds(); f.Size > 0 && secs > 0 {
		return int64(float64(f.Size) * 8 / secs)
	}
	return 0
}

type MediaFile struct {
	Path            string
	Size            int64
	Fingerprint     string
	ModTime         time.Time
	Duration        shared.Duration
	Bitrate         int64
	ContainerFormat string
	// StreamProbeAttempts counts consecutive ffmpeg stream-probe attempts that
	// stored no streams (probe error, timeout, or a genuinely streamless file).
	// 0 means the file was probed successfully or never attempted. The scanner
	// re-probes a streamless file while this is in (0, max) so a transient probe
	// failure self-heals, then stops so a durably-unprobeable file isn't
	// re-probed forever. A successful probe resets it to 0.
	StreamProbeAttempts int
	// InterleaveBackstepMs is how far a sampled cluster timestamp steps BACK
	// between two positions of the file (0 = tracks side by side). Above
	// InterleaveBackstepFlagMs the tracks sit in separate blocks of minutes
	// and no sequential player gets past the first one: playback is refused
	// until a re-mux. Measured at probe time and by the media-health scan.
	InterleaveBackstepMs int64
}

// InterleaveBackstepFlagMs is the floor above which a file NeedsRemux. Every
// muxer keeps cluster timestamps monotonic (494 real files read 0); the 20
// prod victims of the 2026-07-12 two-input remux read 17 s to 2259 s.
const InterleaveBackstepFlagMs = 10_000

// NeedsRemux reports a file whose tracks are stored in blocks of one track at
// a time - unplayable everywhere until re-interleaved.
func (f *MediaFile) NeedsRemux() bool {
	return f != nil && f.InterleaveBackstepMs > InterleaveBackstepFlagMs
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
	// StreamProbeAttempts mirrors media_files.stream_probe_attempts so the scan
	// fast path can re-process a streamless file that is owed a bounded re-probe.
	StreamProbeAttempts int
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
	ID            string
	MediaItemID   shared.MediaItemID
	StreamIndex   int
	StreamType    StreamType
	Codec         string
	CodecLongName string
	Profile       string
	// Level is the codec level (e.g. H.264 level 4.0 = 40), nil when unknown.
	// Needed with Profile to build the exact RFC-6381 CODECS string for a
	// video-copy rung; a nil Level gates copy off (no accurate codec advertise).
	Level     *int
	Language  string
	Title     string
	IsDefault bool
	IsForced  bool
	// Extended disposition flags, sourced from Matroska FlagCommentary/… and the
	// matching ffprobe disposition fields. All false when the container or the probe
	// did not report the flag. Commentary marks director/cast commentary audio tracks
	// and is used by the audio-track auto-selection to prefer a non-commentary default
	// when one exists.
	Commentary       bool
	HearingImpaired  bool
	VisualImpaired   bool
	TextDescriptions bool
	Original         bool
	Width            *int
	Height           *int
	Framerate        *float64
	BitDepth         *int
	// PixelFormat is ffprobe's pix_fmt (yuv420p, yuv420p10le…), nil when the
	// stream predates the probe that captures it. Display only - the copy and
	// HDR decisions read BitDepth/ColorTransfer, never this.
	PixelFormat *string
	ColorSpace  *string
	// ColorTransfer is the ffprobe color_transfer (smpte2084 = PQ, arib-std-b67
	// = HLG, bt709 = SDR…), nil when the probe didn't report it. The only field
	// that tells PQ from HLG, and - with DoViProfile - the HDR ground truth the
	// copy/direct-play decision needs. A nil value on an HEVC stream gates the
	// HEVC copy rung off until a re-probe backfills it (same pattern as Level).
	ColorTransfer *string
	HDRType       *string
	// DoViProfile / DoViBLCompat mirror the Dolby Vision configuration record
	// (ffprobe side_data dv_profile + dv_bl_signal_compatibility_id), nil when
	// the stream carries no DoVi RPU. Profile 5 must always transcode (IPTC2
	// base layer renders wrong on non-DoVi pipelines); profile 8 with a
	// compatible base layer (1=HDR10, 2=SDR, 4=HLG) may be served as its base.
	DoViProfile  *int
	DoViBLCompat *int
	// FieldOrder is ffprobe's field_order (progressive/tt/bb/tb/bt), nil when
	// unreported. A tt/bb/tb/bt value means frames are decoder-flagged
	// interlaced and the flag-gated bwdif already deinterlaces them.
	FieldOrder *string
	// IdetInterlaced is the idet CONTENT-detection verdict for sources whose
	// flags are absent or lie (repacked DVD rips), nil = never analyzed.
	// true switches the transcode to bwdif deint=all for this title.
	IdetInterlaced *bool
	// IdetTelecine: idet saw repeated fields on an interlaced verdict (3:2
	// pulldown). true switches the transcode to inverse telecine.
	IdetTelecine *bool
	// SampleAspectRatio is ffprobe's sample_aspect_ratio (e.g. "64:45" on an
	// anamorphic DVD source), nil when unreported. Anything other than 1:1
	// makes the builder compute the scale height from the DISPLAY aspect -
	// the historical storage-ratio scale+setsar=1 distorted such sources.
	SampleAspectRatio *string
	Channels          *int
	ChannelLayout     *string
	SampleRate        *int
	Bitrate           *int64
	IsExternal        bool
	ExternalPath      *string
	// Keyframes holds the video stream's keyframe presentation times in
	// milliseconds (sorted ascending), or nil when not yet indexed. These are the
	// only valid segment-start boundaries for a -c:v copy (remux) rung - a copied
	// segment cannot begin mid-GOP. Populated by a background backfill, not the
	// scan (the keyframe probe is too costly for the hot scan path). Video only.
	Keyframes []int64
	// FrameRateMode classifies a video stream as "cfr" (constant) or "vfr"
	// (variable), "" when unknown (never analyzed, or a non-EBML container -
	// MP4 does not get this classification). Populated by the same backfill
	// pass as Keyframes (mkvgo ops.Analyze, head-only). A vfr source gates
	// the copy/remux rung off (CopyRungEligible): the HLS copy segmenter
	// assumes a constant frame period when it cuts segments at keyframe
	// boundaries, and a variable rate drifts audio/video sync.
	FrameRateMode string
}

func (s *MediaStream) GetStreamType() string     { return string(s.StreamType) }
func (s *MediaStream) GetBitDepth() *int         { return s.BitDepth }
func (s *MediaStream) GetColorSpace() *string    { return s.ColorSpace }
func (s *MediaStream) GetColorTransfer() *string { return s.ColorTransfer }
func (s *MediaStream) GetHDRType() *string       { return s.HDRType }
func (s *MediaStream) GetProfile() string        { return s.Profile }
func (s *MediaStream) GetLevel() *int            { return s.Level }
func (s *MediaStream) GetDoViProfile() *int      { return s.DoViProfile }
func (s *MediaStream) GetDoViBLCompat() *int     { return s.DoViBLCompat }

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
	// BioCheckedAt is the last time the metadata provider was asked for this
	// person's biography and answered WITHOUT one (a negative cache stamp).
	// nil means never asked, or never answered.
	BioCheckedAt *time.Time
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

// Validate enforces the ScanSchedule invariants: a schedule must target a
// library and carry a cron expression (its syntactic validity is checked by
// the scheduler before persistence).
func (s *ScanSchedule) Validate() error {
	if s.LibraryID == "" {
		return errors.New("scan schedule library ID is required")
	}
	if s.CronExpression == "" {
		return errors.New("scan schedule cron expression is required")
	}
	return nil
}

// NewScanSchedule is the canonical constructor. nextRun may be nil (computed by
// the caller from the cron). Enforces Validate so a malformed schedule can't be
// built from a bare literal.
func NewScanSchedule(libraryID shared.LibraryID, cronExpr string, enabled bool, nextRun *time.Time) (*ScanSchedule, error) {
	s := &ScanSchedule{
		LibraryID:      libraryID,
		CronExpression: cronExpr,
		Enabled:        enabled,
		NextRunAt:      nextRun,
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
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

// MaxCollectionItems bounds how many members of a collection are considered at
// once - by the access check that decides whether a caller may download it, and
// by the bundling that builds the archive. It lives here because those two live
// in different packages: as a literal repeated on both sides, raising one and
// not the other would show a user 1500 items and hand them a ZIP of 1000, with
// no error anywhere.
const MaxCollectionItems = 1000

// Validate enforces the Collection invariants: an id and a name are required.
func (c *Collection) Validate() error {
	if c.ID == "" {
		return errors.New("collection ID is required")
	}
	if c.Name == "" {
		return errors.New("collection name is required")
	}
	return nil
}

// NewCollection is the canonical constructor for a user-created collection
// (manual or smart). It stamps timestamps and enforces Validate so a nameless
// collection can't be built from a bare literal. The TMDB-sourced upsert path
// keeps its own shape (TmdbID/artwork) and is not routed through this.
func NewCollection(id, name string, rules []CollectionRule, sortBy string) (*Collection, error) {
	now := time.Now()
	c := &Collection{
		ID:        id,
		Name:      name,
		Rules:     rules,
		SortBy:    sortBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
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

// ErrMediaItemGone is returned when a trickplay manifest cannot be written
// because the media item it references has been deleted - a foreign-key
// violation. The generation job runs for minutes and races user deletes, so a
// vanished subject is a skip, not a failure (prod, 2026-09-02).
var ErrMediaItemGone = errors.New("media item deleted before its trickplay manifest could be written")

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
	// AlgoVersion stamps the generation parameters the manifest was built
	// with; 0 = legacy rows from before versioning. The missing-sweep
	// re-lists anything below the generator's current version.
	AlgoVersion int
	CreatedAt   time.Time
}
