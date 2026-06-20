package library

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// ListLibrariesFilter paginates LibraryQueryRepo.List with offset semantics.
// Zero values mean "no pagination" — all rows returned, ordered by name.
// Limit is capped at MaxListLibrariesLimit (500); higher values are clamped.
type ListLibrariesFilter struct {
	Limit  int
	Offset int
}

// MaxListLibrariesLimit caps the page size for LibraryQueryRepo.List.
const MaxListLibrariesLimit = 500

type ListMediaItemsFilter struct {
	LibraryID      *shared.LibraryID
	LibraryIDs     []shared.LibraryID // filter to these library IDs (nil = no filter)
	ParentID       *string
	Type           *string
	Genres         []string
	Year           *int
	YearFrom       int     // inclusive lower bound on year (0 = no filter)
	YearTo         int     // inclusive upper bound on year (0 = no filter)
	MinRating      float64 // minimum community_rating (0 = no filter)
	SortField      string
	SortOrder      string
	Limit          int
	Offset         int
	// AfterID enables keyset pagination: when set, forces ORDER BY id ASC
	// and restricts to rows with id > AfterID. Prefer this over Offset for
	// full-table iteration (Meili reindex) — O(log N) per page vs O(n²)
	// cumulative with offset.
	AfterID string
	// Cursor enables sort-aware keyset pagination: WHERE (sort_value, id) >
	// (cursor.value, cursor.id) on the SortField. Mutually exclusive with
	// Offset (cursor wins). The cursor's SortField must match the request's
	// SortField — repos return shared.ErrCursorMismatch otherwise.
	Cursor         shared.Cursor
	AllowedRatings []string // parental control: if set, only items with official_rating in this list (or empty rating) are returned
	PersonID       *string  // filter to items featuring this person
	// MissingProviderIDs filters by whether the item's Catalog.ProviderIDs is
	// effectively empty (NULL row in media_catalog or provider_ids = '{}').
	// nil = no filter; true = only orphans; false = only matched items.
	// Used by the admin "Match orphans" action to target enrichment at items
	// that never matched a provider during initial scan.
	MissingProviderIDs *bool
}

type MediaItemRepository interface {
	Create(ctx context.Context, item *MediaItem) error
	GetByID(ctx context.Context, id shared.MediaItemID) (*MediaItem, error)
	List(ctx context.Context, filter ListMediaItemsFilter) ([]*MediaItem, error)
	Update(ctx context.Context, item *MediaItem) error
	Delete(ctx context.Context, id shared.MediaItemID) error
	GetByFilePath(ctx context.Context, path string) (*MediaItem, error)
	GetByFileFingerprint(ctx context.Context, fingerprint string) (*MediaItem, error)
	GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*MediaItem, error)
	Count(ctx context.Context, filter ListMediaItemsFilter) (int, error)
	// SeriesAggregate returns the total runtime + episode/season counts for a
	// series by recursively descending the parent→child hierarchy. Only counts
	// media items that are actually present in the library.
	SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (SeriesAggregate, error)
}

// SeriesAggregate is the denormalized runtime view of a series computed on the
// fly. Season count is the distinct parent_ids of the counted episodes.
type SeriesAggregate struct {
	TotalDuration shared.Duration
	EpisodeCount  int
	SeasonCount   int
}

// MediaStreamQueryRepo is the read-only subset of MediaStreamRepository for
// query services. The full repository embeds this, so postgres adapters and
// other concrete repos satisfy both with a single implementation.
type MediaStreamQueryRepo interface {
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*MediaStream, error)
}

type MediaStreamRepository interface {
	MediaStreamQueryRepo
	CreateBatch(ctx context.Context, streams []*MediaStream) error
	ReplaceBatch(ctx context.Context, mediaItemID shared.MediaItemID, streams []*MediaStream) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error
}

// ChapterQueryRepo is the read-only subset of ChapterRepository for query
// services. The full repository embeds this.
type ChapterQueryRepo interface {
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*Chapter, error)
}

type ChapterRepository interface {
	ChapterQueryRepo
	ReplaceBatch(ctx context.Context, mediaItemID shared.MediaItemID, chapters []*Chapter) error
}

// MediaSegmentQueryRepo is the read-only subset used by query services.
type MediaSegmentQueryRepo interface {
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*MediaSegment, error)
}

// IntroMetadataProvider is the port for an external source of intro/recap/
// end_credits metadata, queried per-episode by the intro detection pipeline.
//
// The pipeline calls this *before* MKV chapter pass and the heuristic
// passes — when the provider returns a non-empty slice, the rest of the
// pipeline is short-circuited. Implementations must:
//   - return nil, nil (or an empty slice) when no metadata is known for
//     the item — that's the common case, the pipeline must fall through;
//   - return MediaSegment values with Confidence ≥ 0.95 and Source set
//     to a stable identifier (e.g. "tmdb", "anidb", "simkl") so the
//     admin UI / backfill tooling can audit which source produced each
//     segment;
//   - be safe to call from a worker goroutine; the pipeline does not
//     wrap the call with its own timeout, so the implementation owns
//     its HTTP/IO timeouts.
//
// No concrete adapter is wired today; intro metadata is not part of
// TMDB's public schema and the choice of community DB (AniDB / simkl /
// custom) is not yet made — see backlog "TMDB intro/outro metadata".
type IntroMetadataProvider interface {
	GetSegments(ctx context.Context, item *MediaItem) ([]MediaSegment, error)
}

// SegmentContributor is the port for reporting high-confidence locally-detected
// segments back to a community database (e.g. TheIntroDB). Implementations
// should skip silently when the submission is not applicable (no API key,
// unsupported type) so callers can invoke Contribute unconditionally.
//
// Contribute is expected to be called fire-and-forget in a goroutine;
// implementations own their HTTP timeouts and should not surface non-fatal
// errors to callers. The item is provided for any ancestor-lookup the
// implementation needs (e.g. resolving tmdb_id from an episode).
type SegmentContributor interface {
	Contribute(ctx context.Context, item *MediaItem, seg MediaSegment) error
}

// MediaSegmentRepository is the full read-write interface for media segments.
type MediaSegmentRepository interface {
	MediaSegmentQueryRepo
	Upsert(ctx context.Context, seg *MediaSegment) error
	// UpsertUnlessManual inserts the segment, or updates the existing
	// (media_item_id, type) row ONLY when its source is not an operator mark
	// ('manual'/'manual_propagated'). The manual-lock check and the write happen
	// atomically (single statement, row-locked) so a concurrent auto-detection
	// can't read "no lock" then clobber a manual mark set in the race window.
	// Returns applied=false when an existing manual mark caused the skip.
	UpsertUnlessManual(ctx context.Context, seg *MediaSegment) (applied bool, err error)
	// UpsertUnlessOperatorManual is UpsertUnlessManual that locks ONLY against the
	// operator's hard 'manual' mark, NOT 'manual_propagated'. The intro propagation
	// path uses it: it writes 'manual_propagated' rows and must be able to overwrite
	// its OWN previous propagation on a re-run, while still never clobbering an
	// operator 'manual' mark that landed in the race window between its read-guard
	// and this write. Returns applied=false when an existing 'manual' mark skipped.
	UpsertUnlessOperatorManual(ctx context.Context, seg *MediaSegment) (applied bool, err error)
	DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error
	DeleteByMediaItemIDAndType(ctx context.Context, mediaItemID shared.MediaItemID, segType string) error
	// ListSeasonsMissingSegments returns the parent_ids of seasons that have
	// at least one episode without any media_segments rows. Used by the intro
	// detection backfill sweep at startup.
	ListSeasonsMissingSegments(ctx context.Context) ([]string, error)
	// RecordIntroDetectionAttempt counts a detection run that produced no usable
	// intro for an item, stamped with the algorithm version it ran under. When the
	// stored version differs from algoVersion (the algorithm improved) the count
	// resets to 1; otherwise it increments. Lets the backfill bound retries without
	// re-decoding the file on every restart.
	RecordIntroDetectionAttempt(ctx context.Context, mediaItemID string, algoVersion int) error
	// IntroDetectionExhausted reports whether an item has already been attempted
	// maxAttempts times AT the current algoVersion with no usable result — i.e. it
	// should be skipped until the algorithm version is bumped. A row at an older
	// version (or no row) returns false, so a detection improvement re-enables it.
	IntroDetectionExhausted(ctx context.Context, mediaItemID string, algoVersion, maxAttempts int) (bool, error)
}

// PersonQueryRepo is the read-only subset of PersonRepository for query
// services. The full repository embeds this.
type PersonQueryRepo interface {
	GetByID(ctx context.Context, id string) (*Person, error)
	GetByName(ctx context.Context, name string) (*Person, error)
	// GetByNamesBatch returns a name→Person map for the given set of
	// names. Missing names are absent from the returned map (not nil
	// entries). Drives the enricher preload that kills the N+1 on
	// large casts.
	GetByNamesBatch(ctx context.Context, names []string) (map[string]*Person, error)
	List(ctx context.Context) ([]*Person, error)
}

type PersonRepository interface {
	PersonQueryRepo
	Create(ctx context.Context, person *Person) error
	Update(ctx context.Context, person *Person) error
}

type MediaPersonRepository interface {
	Create(ctx context.Context, mp *MediaPerson) error
	CreateBatch(ctx context.Context, mps []*MediaPerson) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*MediaPerson, error)
}

type CollectionCommandRepo interface {
	Create(ctx context.Context, col *Collection) error
	Upsert(ctx context.Context, col *Collection) error
	Update(ctx context.Context, col *Collection) error
	Delete(ctx context.Context, id string) error
	LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

type CollectionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*Collection, error)
	List(ctx context.Context) ([]*Collection, error)
	GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*MediaItem, error)
	CountItems(ctx context.Context, collectionID string) (int, error)
	// GetItemsByCollectionIDs fetches items for multiple collections in a
	// single query. Returns a map keyed by collection ID. Empty input returns
	// an empty map without hitting the database.
	GetItemsByCollectionIDs(ctx context.Context, collectionIDs []string) (map[string][]*MediaItem, error)
}

type ScannedFile struct {
	Path        string
	Size        int64
	ModTime     time.Time
	Fingerprint string
	Title       string
	Type        MediaItemType
	Year        *int
	Season      *int
	Episode     *int
}

type FileScanner interface {
	Scan(ctx context.Context, path string) ([]ScannedFile, error)
	// FileExists reports whether a file is still present on disk. Used to tell a
	// real move (old path gone) from a size+mtime fingerprint collision (both
	// files still present) before repointing an existing item to a new path.
	FileExists(path string) bool
	// ParseTitle re-runs the filename parser on a known file path and
	// returns the title token (without quality/edition/year noise). Used
	// by the orphan title-heal pass to rebuild media_items.title with the
	// current parser when an old polluted scan needs cleaning up — the
	// regular Scan path only emits this for fresh discoveries, but the
	// heal pass needs it for items already in DB whose files haven't moved
	// on disk (so detectChanges short-circuits before ever calling Scan
	// downstream of them).
	ParseTitle(path string) string
}

type ProbeResult struct {
	Streams  []*MediaStream
	Duration shared.Duration
}

type StreamProber interface {
	ProbeStreams(ctx context.Context, filePath string) (*ProbeResult, error)
}

type ExternalSubtitle struct {
	Path     string
	Language string
	IsForced bool
	Format   string
}

type ExternalSubtitleScanner interface {
	ScanForSubtitles(videoPath string) ([]ExternalSubtitle, error)
}

type ScanScheduleRepository interface {
	Upsert(ctx context.Context, s *ScanSchedule) error
	GetByLibraryID(ctx context.Context, libraryID string) (*ScanSchedule, error)
	Delete(ctx context.Context, libraryID string) error
	List(ctx context.Context) ([]*ScanSchedule, error)
	ListEnabled(ctx context.Context) ([]*ScanSchedule, error)
	UpdateAfterRun(ctx context.Context, libraryID string, lastRun, nextRun time.Time) error
}

type SearchHit struct {
	ID               string
	LibraryID        shared.LibraryID
	Title            string
	Year             *int
	Type             string
	Genres           []string
	Tags             []string
	OfficialRating   string
	HighlightedTitle string
}

type SearchResult struct {
	Hits      []SearchHit
	TotalHits int64
}

type SearchIndex interface {
	IndexMediaItem(ctx context.Context, item *MediaItem, castNames []string) error
	IndexMediaItemsBatch(ctx context.Context, items []*MediaItem) error
	RemoveMediaItem(ctx context.Context, id string) error
	Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]SearchHit, int64, error)
}

type FileEvent struct {
	Path string
	Op   FileOp
}

type FileOp int

const (
	FileCreated FileOp = iota
	FileModified
	FileRemoved
	FileRenamed
)

type FileWatcher interface {
	Watch(path string) error
	Unwatch(path string) error
	Events() <-chan FileEvent
	Errors() <-chan error
	Close() error
}

// TrickplayQueryRepo is the read-only subset of TrickplayRepository for query
// services. The full repository embeds this.
type TrickplayQueryRepo interface {
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) (*TrickplayManifest, error)
	ListMissingMediaItemIDs(ctx context.Context) ([]string, error)
}

type TrickplayRepository interface {
	TrickplayQueryRepo
	Create(ctx context.Context, manifest *TrickplayManifest) error
	Delete(ctx context.Context, mediaItemID shared.MediaItemID) error
}

// UserRating values: -1=dislike, 0=neutral/removed, 1=like
type UserRating struct {
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	Value       int // -1, 0, 1
}

type UserRatingRepository interface {
	Upsert(ctx context.Context, r *UserRating) error
	Get(ctx context.Context, profileID, mediaItemID string) (*UserRating, error)
	// GetForItems returns ratings for multiple media items for a given profile.
	GetForItems(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]*UserRating, error)
}

type TrickplayResult struct {
	ImageDir   string
	ImageCount int
	// FrameTimestampsMs is the actual timestamp (in source video, ms) of every
	// extracted thumbnail, in tile order. Length = ImageCount * tileWidth *
	// tileHeight (minus any unfilled tiles in the last sprite). Captured from
	// ffmpeg's `showinfo` filter so the player and intro detector get true
	// frame positions instead of an interval-based approximation that breaks
	// when keyframes are sparse.
	FrameTimestampsMs []int64
}

type TrickplayGenerator interface {
	Generate(ctx context.Context, filePath string, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int) (*TrickplayResult, error)
}

// MediaItemTranslationQueryRepo is the read-only subset of
// MediaItemTranslationRepository for query services. The full repository
// embeds this.
type MediaItemTranslationQueryRepo interface {
	GetByMediaItemAndLocale(ctx context.Context, mediaItemID, locale string) (*MediaItemTranslation, error)
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*MediaItemTranslation, error)
	// GetByMediaItemsAndLocale batch-fetches one locale's translations for many
	// items at once, keyed by media item ID. Items without a translation in
	// that locale are simply absent from the map (no error).
	GetByMediaItemsAndLocale(ctx context.Context, mediaItemIDs []string, locale string) (map[string]*MediaItemTranslation, error)
}

type MediaItemTranslationRepository interface {
	MediaItemTranslationQueryRepo
	Upsert(ctx context.Context, t *MediaItemTranslation) error
	UpsertBatch(ctx context.Context, translations []*MediaItemTranslation) error
	DeleteByMediaItem(ctx context.Context, mediaItemID string) error
}

// PersonTranslationQueryRepo is the read-only subset of
// PersonTranslationRepository for query services. The full repository embeds
// this.
type PersonTranslationQueryRepo interface {
	GetByPersonAndLocale(ctx context.Context, personID, locale string) (*PersonTranslation, error)
	GetByPerson(ctx context.Context, personID string) ([]*PersonTranslation, error)
}

type PersonTranslationRepository interface {
	PersonTranslationQueryRepo
	Upsert(ctx context.Context, t *PersonTranslation) error
	DeleteByPerson(ctx context.Context, personID string) error
}

type PersonDetailsResult struct {
	Biography    string
	Birthday     string
	Deathday     string
	PlaceOfBirth string
	KnownFor     string
	ThumbURL     string
}

type PersonDetailsFetcher interface {
	GetPersonDetails(ctx context.Context, name string) (*PersonDetailsResult, error)
}

type PersonTranslationFetcher interface {
	FetchPersonTranslations(ctx context.Context, name string, locales []string) (map[string]string, error)
}

// IntroFingerprintCacheEntry is the cached pHash fingerprint for one media
// item. Audio bands are intentionally not cached in v1 — they are
// candidate-window-specific (different padSec/searchSec per call) and the
// cost of regenerating them per detection run is small compared to the
// trickplay sprite decode + tile hash cycle that this cache short-circuits.
type IntroFingerprintCacheEntry struct {
	MediaItemID      string
	FileFingerprint  string
	PhashTiles       []uint64 // one hash per kept tile, in tile order
	PhashTimestamps  []int64  // matching source-video timestamp (ms) for every PhashTiles entry
	AlgoVersion      int16    // bumped whenever the hash function changes — invalidates the row
	ExtractedAt      time.Time
}

// IntroFingerprintCacheRepository persists and retrieves cached pHash
// fingerprints keyed by media item id. Invalidation is up to the caller:
// look up by media id, compare FileFingerprint and AlgoVersion against the
// current values, recompute and Upsert if either differs. The repository
// itself does no validation — keep it dumb so changes to the invalidation
// policy don't require touching the adapter.
type IntroFingerprintCacheRepository interface {
	Get(ctx context.Context, mediaItemID string) (*IntroFingerprintCacheEntry, error)
	Upsert(ctx context.Context, entry *IntroFingerprintCacheEntry) error
	Delete(ctx context.Context, mediaItemID string) error
	DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error)
}
