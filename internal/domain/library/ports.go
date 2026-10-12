package library

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// ListLibrariesFilter paginates LibraryQueryRepo.List with offset semantics.
// Zero values mean "no pagination" - all rows returned, ordered by name.
// Limit is capped at MaxListLibrariesLimit (500); higher values are clamped.
type ListLibrariesFilter struct {
	Limit  int
	Offset int
}

// MaxListLibrariesLimit caps the page size for LibraryQueryRepo.List.
const MaxListLibrariesLimit = 500

type ListMediaItemsFilter struct {
	LibraryID  *shared.LibraryID
	LibraryIDs []shared.LibraryID // filter to these library IDs (nil = no filter)
	ParentID   *string
	Type       *string
	Genres     []string
	Year       *int
	YearFrom   int     // inclusive lower bound on year (0 = no filter)
	YearTo     int     // inclusive upper bound on year (0 = no filter)
	MinRating  float64 // minimum community_rating (0 = no filter)
	SortField  string
	SortOrder  string
	Limit      int
	Offset     int
	// AfterID enables keyset pagination: when set, forces ORDER BY id ASC
	// and restricts to rows with id > AfterID. Prefer this over Offset for
	// full-table iteration (Meili reindex) - O(log N) per page vs O(n²)
	// cumulative with offset.
	AfterID string
	// KeysetByID orders by id from the FIRST page on. A full walk starts with
	// AfterID empty, and without this its first page came back in title order
	// while the following pages went on by id: every item with an id below
	// page one's last id and a title past page one was never visited - 34 of
	// 639 lost from the search rebuild, which then re-ran forever (audit
	// 2026-10-01).
	KeysetByID bool
	// Cursor enables sort-aware keyset pagination: WHERE (sort_value, id) >
	// (cursor.value, cursor.id) on the SortField. Mutually exclusive with
	// Offset (cursor wins). The cursor's SortField must match the request's
	// SortField - repos return shared.ErrCursorMismatch otherwise.
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
	// GetByMediaItemIDs batches GetByMediaItemID for one page of items into a
	// single query, keyed by media item id; an id with no streams is absent
	// from the map. Keyframes are NOT loaded - list consumers read codec/
	// resolution/language, and one film's keyframe index outweighs the rest
	// of its stream row a thousandfold.
	GetByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]*MediaStream, error)
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
	// GetByMediaItemIDs batches GetByMediaItemID for many items into one
	// query, keyed by media item id. An id with no segments is simply absent
	// from the returned map (not a nil-slice entry). Drives the intro
	// detection season sweep, which otherwise issued one query per episode
	// just to check for an existing covering segment.
	GetByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]*MediaSegment, error)
}

// IntroMetadataProvider is the port for an external source of intro/recap/
// end_credits metadata, queried per-episode by the intro detection pipeline.
//
// The pipeline calls this *before* MKV chapter pass and the heuristic
// passes - when the provider returns a non-empty slice, the rest of the
// pipeline is short-circuited. Implementations must:
//   - return nil, nil (or an empty slice) when no metadata is known for
//     the item - that's the common case, the pipeline must fall through;
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
// custom) is not yet made - see backlog "TMDB intro/outro metadata".
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
	// ListSeasonsPendingIntroDetection returns up to limit season parent_ids
	// that still have intro-detection work to do: at least one episode whose
	// stored intro does not satisfy IntroDetectionSettled AND that has not
	// burned maxAttempts at algoVersion. This IS the sweep's work list - it is
	// re-derived from the database on every batch, so it shrinks as detection
	// progresses and a process killed mid-sweep loses nothing.
	//
	// It replaced a "seasons with no media_segments row at all" predicate that
	// found almost nothing: ~99% of a real catalogue already carries an
	// end_credits row, which is not an intro.
	//
	// exclude is the caller's in-run skip set (seasons already walked this
	// cycle): without it a season that makes no progress is handed back by
	// every batch and the drain loop spins. Ordered by parent_id so the batches
	// are stable.
	ListSeasonsPendingIntroDetection(ctx context.Context, algoVersion, maxAttempts, limit int, exclude []string) ([]string, error)
	// RecordIntroDetectionAttempt counts a detection run that produced no usable
	// intro for an item, stamped with the algorithm version it ran under. When the
	// stored version differs from algoVersion (the algorithm improved) the count
	// resets to 1; otherwise it increments. Lets the backfill bound retries without
	// re-decoding the file on every restart.
	RecordIntroDetectionAttempt(ctx context.Context, mediaItemID string, algoVersion int) error
	// IntroDetectionExhaustedBatch reports, for each of mediaItemIDs, whether the
	// item has already been attempted maxAttempts times AT the current algoVersion
	// with no usable result - i.e. it should be skipped until the algorithm version
	// is bumped. An id at an older version (or with no row) is simply absent from
	// the returned map (not exhausted), so a detection improvement re-enables it.
	// Batched so a season sweep issues one query per page instead of one per
	// candidate episode.
	IntroDetectionExhaustedBatch(ctx context.Context, mediaItemIDs []string, algoVersion, maxAttempts int) (map[string]bool, error)
	// IntroDetectionAttemptVersions returns the algorithm version of each item's last
	// detection attempt; an item never attempted is absent. It tells a detector-made
	// intro from the current version apart from one an older version left behind.
	IntroDetectionAttemptVersions(ctx context.Context, mediaItemIDs []string) (map[string]int, error)
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
	// DeleteByMediaItemIDs is the batch form of DeleteByMediaItemID: one
	// statement for a whole level of items (e.g. all episodes of a season)
	// instead of one round-trip per item.
	DeleteByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*MediaPerson, error)
}

type CollectionCommandRepo interface {
	Create(ctx context.Context, col *Collection) error
	Upsert(ctx context.Context, col *Collection) error
	Update(ctx context.Context, col *Collection) error
	Delete(ctx context.Context, id string) error
	LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
	// UnlinkItem removes one member. Absent membership is not an error: the
	// caller asked for a state, and that state already holds.
	UnlinkItem(ctx context.Context, collectionID, mediaItemID string) error
	// SetItemOrder stamps the hand-set position of the supplied members;
	// position in the slice IS the new order. Members left out keep theirs.
	// Returns ErrCollectionReorderDrift when a supplied id is not a member at
	// write time, and the whole reorder is rolled back.
	SetItemOrder(ctx context.Context, collectionID string, mediaItemIDs []string) error
}

// ErrCollectionReorderDrift reports that a reorder matched fewer rows than the
// distinct ids supplied: an item left the collection between the handler's
// membership pre-check (which runs outside any transaction) and the UPDATE.
// The write is rolled back rather than half-applied. Mirrors
// playlist.ErrReorderMembershipDrift.
var ErrCollectionReorderDrift = errors.New("collection reorder: membership changed during reorder")

type CollectionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*Collection, error)
	List(ctx context.Context) ([]*Collection, error)
	GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*MediaItem, error)
	CountItems(ctx context.Context, collectionID string) (int, error)
	// GetItemsByCollectionIDs fetches items for multiple collections in a
	// single query. Returns a map keyed by collection ID. Empty input returns
	// an empty map without hitting the database.
	GetItemsByCollectionIDs(ctx context.Context, collectionIDs []string) (map[string][]*MediaItem, error)
	// ListMoviesWithoutCollection is the saga-repair work list: movies that
	// carry a provider id and belong to no collection. Membership is only
	// written while a title is being enriched, so every movie enriched before
	// that path existed stayed out of its saga - on the production server the
	// sequels were sitting right there while the saga listed one title
	// (measured 2026-08-04). Returns at most limit rows, in id order, strictly
	// after afterID - a cursor, because a standalone film is never removed from
	// this list, so a caller without one would re-read the same head of the
	// catalogue for ever and never reach what sits behind it.
	ListMoviesWithoutCollection(ctx context.Context, afterID string, limit int) ([]UnlinkedMovie, error)
	// GetByMediaItemID returns the collections holding this title, so a media
	// page can say which saga it belongs to and link to the rest of it.
	GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*Collection, error)
}

// UnlinkedMovie is one entry of the saga-repair work list: the movie, and the
// provider id its collection can be resolved from.
type UnlinkedMovie struct {
	MediaItemID shared.MediaItemID
	ProviderID  string
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
	// current parser when an old polluted scan needs cleaning up - the
	// regular Scan path only emits this for fresh discoveries, but the
	// heal pass needs it for items already in DB whose files haven't moved
	// on disk (so detectChanges short-circuits before ever calling Scan
	// downstream of them).
	ParseTitle(path string) string
}

type ProbeResult struct {
	Streams  []*MediaStream
	Duration shared.Duration
	// Container is the source container/format (ffprobe vocabulary, e.g.
	// "matroska", "mp4", "webm"), stored on MediaFile.ContainerFormat. Empty when
	// the prober couldn't determine it. The playback decision matches it against
	// the client's supported containers - an empty value reads as "incompatible"
	// and forces a needless remux, so probers MUST populate it.
	Container string
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
	// IndexMediaItem upserts one item's search document. Cast names travel on
	// item.CastNames (a carry-along, like Translations) so every indexing path
	// feeds the same document shape.
	IndexMediaItem(ctx context.Context, item *MediaItem) error
	IndexMediaItemsBatch(ctx context.Context, items []*MediaItem) error
	RemoveMediaItem(ctx context.Context, id string) error
	// Clear removes every document from the index. The index/upsert path can only
	// add or overwrite, so it can never SHRINK an index that has drifted larger
	// than the DB (a delete dropped during an outage). Clear lets the reconcile
	// wipe-then-reindex to evict those stale docs.
	Clear(ctx context.Context) error
	Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]SearchHit, int64, error)
}

// Search-index write ops recorded in the durable outbox.
const (
	SearchOpUpsert = "upsert"
	SearchOpDelete = "delete"
)

// PendingSearchWrite is one queued search-index op: the media item to re-index
// or remove once the engine recovers. Only the ID + op are stored - the drain
// worker re-reads the item from the DB (the source of truth), so a value that
// changed between enqueue and drain is indexed at its current state.
type PendingSearchWrite struct {
	MediaItemID string
	Op          string // SearchOpUpsert | SearchOpDelete
}

// PendingSearchWriteQueue is the durable outbox the resilient search wrapper
// appends to when the engine rejects a live index write, and the drain worker
// replays. Enqueue is last-op-wins per item (an upsert then a delete during one
// outage → the delete wins).
type PendingSearchWriteQueue interface {
	Enqueue(ctx context.Context, mediaItemID, op string) error
	List(ctx context.Context, limit int) ([]PendingSearchWrite, error)
	Delete(ctx context.Context, mediaItemID string) error
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
	// GetByMediaItemIDs batches GetByMediaItemID for many items into one
	// query, keyed by media item id. An id with no manifest is simply absent
	// from the returned map (not a nil-pointer entry). Drives the intro
	// propagation sibling sweeps, which otherwise issued one query per
	// sibling just to read its trickplay manifest.
	GetByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID]*TrickplayManifest, error)
	// ListMissingMediaItemIDs returns the items with no manifest at all PLUS
	// the items whose stored manifest predates minAlgoVersion (its generation
	// parameters changed) - without the version clause, a params change never
	// reached pre-existing manifests.
	ListMissingMediaItemIDs(ctx context.Context, minAlgoVersion int) ([]string, error)
}

type TrickplayRepository interface {
	TrickplayQueryRepo
	// Create stores the manifest and clears any recorded generation failure
	// for the item - a success settles the question the failure recorded.
	Create(ctx context.Context, manifest *TrickplayManifest) error
	Delete(ctx context.Context, mediaItemID shared.MediaItemID) error
	// RecordFailure remembers that generation failed against this exact file
	// (identified by size + mod time) so ListMissingMediaItemIDs stops
	// re-serving the item at every boot. A replaced file no longer matches the
	// recorded pair and re-enters the candidates naturally.
	RecordFailure(ctx context.Context, mediaItemID shared.MediaItemID, fileSize int64, modTime time.Time, reason string) error
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

// FavoriteRepository stores a profile's favorites - like UserRating, a
// per-profile mark on a media item.
type FavoriteRepository interface {
	Add(ctx context.Context, fav *Favorite) error
	Remove(ctx context.Context, profileID, mediaItemID string) error
	List(ctx context.Context, profileID string, limit, offset int) ([]*Favorite, error)
	IsFavorite(ctx context.Context, profileID, mediaItemID string) (bool, error)
	// IsFavoriteMany answers for several media items in one round trip. The
	// returned set holds only the favorited ids - an absent id is not favorited.
	IsFavoriteMany(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]bool, error)
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
	// knownDurationMs is the duration the scan already established for this
	// file; 0 means unknown. A positive value lets the generator skip its
	// readability pre-flight - the scan producing a duration IS the proof that
	// the file opens - which is one less ffprobe fork per generated trickplay.
	Generate(ctx context.Context, filePath string, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int, knownDurationMs int64) (*TrickplayResult, error)
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
// item. Audio bands are intentionally not cached in v1 - they are
// candidate-window-specific (different padSec/searchSec per call) and the
// cost of regenerating them per detection run is small compared to the
// trickplay sprite decode + tile hash cycle that this cache short-circuits.
type IntroFingerprintCacheEntry struct {
	MediaItemID     string
	FileFingerprint string
	PhashTiles      []uint64 // one hash per kept tile, in tile order
	PhashTimestamps []int64  // matching source-video timestamp (ms) for every PhashTiles entry
	AlgoVersion     int16    // bumped whenever the hash function changes - invalidates the row
	ExtractedAt     time.Time
}

// IntroFingerprintCacheRepository persists and retrieves cached pHash
// fingerprints keyed by media item id. Invalidation is up to the caller:
// look up by media id, compare FileFingerprint and AlgoVersion against the
// current values, recompute and Upsert if either differs. The repository
// itself does no validation - keep it dumb so changes to the invalidation
// policy don't require touching the adapter.
type IntroFingerprintCacheRepository interface {
	Get(ctx context.Context, mediaItemID string) (*IntroFingerprintCacheEntry, error)
	Upsert(ctx context.Context, entry *IntroFingerprintCacheEntry) error
	Delete(ctx context.Context, mediaItemID string) error
	DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error)
}

// IntroAudioFingerprintCacheEntry is the cached Chromaprint fingerprint for one
// media item over [0, WindowSec]. The cache is keyed by (MediaItemID,
// FileFingerprint, AlgoVersion, WindowSec) - any mismatch triggers recompute.
type IntroAudioFingerprintCacheEntry struct {
	MediaItemID     string
	FileFingerprint string
	WindowSec       int      // length of the analysed window fed to fpcalc
	Fingerprint     []uint32 // raw Chromaprint []uint32 over [0, WindowSec]
	AlgoVersion     int16    // bumped when windowSec or the Chromaprint method changes
	ExtractedAt     time.Time
}

// IntroAudioFingerprintCacheRepository persists and retrieves cached Chromaprint
// fingerprints keyed by media item id. Same invalidation contract as
// IntroFingerprintCacheRepository - the caller owns invalidation logic, the
// repository stays dumb.
type IntroAudioFingerprintCacheRepository interface {
	Get(ctx context.Context, mediaItemID string) (*IntroAudioFingerprintCacheEntry, error)
	// GetBatch fetches many entries in one query, keyed by media item id; ids
	// with no entry are simply absent. Drives the season-cluster pass, which
	// otherwise issued one Get per episode (40 round trips per season).
	GetBatch(ctx context.Context, mediaItemIDs []string) (map[string]*IntroAudioFingerprintCacheEntry, error)
	Upsert(ctx context.Context, entry *IntroAudioFingerprintCacheEntry) error
	Delete(ctx context.Context, mediaItemID string) error
	DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error)
}

// ProbeFilePrefix marks the watcher's own inotify liveness-probe files.
//
// It lives in the domain because BOTH sides of the watcher port need it: the
// app layer writes these files and waits for their events, and the filesystem
// adapter must forward them past its video-extension filter - a probe file
// deliberately has no media extension, so without an explicit exemption the
// adapter drops the event and every probe times out, declaring healthy watches
// dead. That failure was real and was caught only by a test against the actual
// kernel; keep the two sides sharing this one constant.
const ProbeFilePrefix = ".evey-watch-probe-"

// IsProbeFile reports whether a path is a watcher liveness-probe file.
func IsProbeFile(path string) bool {
	return strings.HasPrefix(filepath.Base(path), ProbeFilePrefix)
}
