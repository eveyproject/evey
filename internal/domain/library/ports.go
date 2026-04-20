package library

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type LibraryRepository interface {
	Create(ctx context.Context, lib *Library) error
	GetByID(ctx context.Context, id shared.LibraryID) (*Library, error)
	List(ctx context.Context, filter ListLibrariesFilter) ([]*Library, error)
	Update(ctx context.Context, lib *Library) error
	Delete(ctx context.Context, id shared.LibraryID) error
}

// ListLibrariesFilter paginates LibraryRepository.List with offset semantics.
// Zero values mean "no pagination" — all rows returned, ordered by name.
// Limit is capped at MaxListLibrariesLimit (500); higher values are clamped.
type ListLibrariesFilter struct {
	Limit  int
	Offset int
}

// MaxListLibrariesLimit caps the page size for LibraryRepository.List.
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
	TotalDurationMs int64
	EpisodeCount    int
	SeasonCount     int
}

type MediaStreamRepository interface {
	CreateBatch(ctx context.Context, streams []*MediaStream) error
	ReplaceBatch(ctx context.Context, mediaItemID shared.MediaItemID, streams []*MediaStream) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*MediaStream, error)
	DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error
}

type ChapterRepository interface {
	ReplaceBatch(ctx context.Context, mediaItemID shared.MediaItemID, chapters []*Chapter) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*Chapter, error)
}

type PersonRepository interface {
	Create(ctx context.Context, person *Person) error
	Update(ctx context.Context, person *Person) error
	GetByID(ctx context.Context, id string) (*Person, error)
	GetByName(ctx context.Context, name string) (*Person, error)
	List(ctx context.Context) ([]*Person, error)
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
}

type ProbeResult struct {
	Streams    []*MediaStream
	DurationMs int64
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

type TrickplayRepository interface {
	Create(ctx context.Context, manifest *TrickplayManifest) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) (*TrickplayManifest, error)
	Delete(ctx context.Context, mediaItemID shared.MediaItemID) error
	ListMissingMediaItemIDs(ctx context.Context) ([]string, error)
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
}

type TrickplayGenerator interface {
	Generate(ctx context.Context, filePath string, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int) (*TrickplayResult, error)
}

type MediaItemTranslationRepository interface {
	Upsert(ctx context.Context, t *MediaItemTranslation) error
	UpsertBatch(ctx context.Context, translations []*MediaItemTranslation) error
	GetByMediaItemAndLocale(ctx context.Context, mediaItemID, locale string) (*MediaItemTranslation, error)
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*MediaItemTranslation, error)
	DeleteByMediaItem(ctx context.Context, mediaItemID string) error
}

type PersonTranslationRepository interface {
	Upsert(ctx context.Context, t *PersonTranslation) error
	GetByPersonAndLocale(ctx context.Context, personID, locale string) (*PersonTranslation, error)
	GetByPerson(ctx context.Context, personID string) ([]*PersonTranslation, error)
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
