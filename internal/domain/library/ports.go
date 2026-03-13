package library

import (
	"context"
	"time"
)

type LibraryRepository interface {
	Create(ctx context.Context, lib *Library) error
	GetByID(ctx context.Context, id string) (*Library, error)
	List(ctx context.Context) ([]*Library, error)
	Update(ctx context.Context, lib *Library) error
	Delete(ctx context.Context, id string) error
}

type ListMediaItemsFilter struct {
	LibraryID      *string
	LibraryIDs     []string // filter to these library IDs (nil = no filter)
	ParentID       *string
	Type           *string
	Genres         []string
	Year           *int
	SortField      string
	SortOrder      string
	Limit          int
	Offset         int
	AllowedRatings []string // parental control: if set, only items with official_rating in this list (or empty rating) are returned
}

type MediaItemRepository interface {
	Create(ctx context.Context, item *MediaItem) error
	GetByID(ctx context.Context, id string) (*MediaItem, error)
	List(ctx context.Context, filter ListMediaItemsFilter) ([]*MediaItem, error)
	Update(ctx context.Context, item *MediaItem) error
	Delete(ctx context.Context, id string) error
	GetByFilePath(ctx context.Context, path string) (*MediaItem, error)
	GetByFileFingerprint(ctx context.Context, fingerprint string) (*MediaItem, error)
	GetByLibraryTypeTitle(ctx context.Context, libraryID, itemType, title, parentID string) (*MediaItem, error)
	Count(ctx context.Context, filter ListMediaItemsFilter) (int, error)
}

type MediaStreamRepository interface {
	CreateBatch(ctx context.Context, streams []*MediaStream) error
	GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*MediaStream, error)
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
}

type ChapterRepository interface {
	ReplaceBatch(ctx context.Context, mediaItemID string, chapters []*Chapter) error
	GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*Chapter, error)
}

type PersonRepository interface {
	Create(ctx context.Context, person *Person) error
	GetByID(ctx context.Context, id string) (*Person, error)
	GetByName(ctx context.Context, name string) (*Person, error)
	List(ctx context.Context) ([]*Person, error)
}

type MediaPersonRepository interface {
	Create(ctx context.Context, mp *MediaPerson) error
	DeleteByMediaItemID(ctx context.Context, mediaItemID string) error
	GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*MediaPerson, error)
}

type CollectionCommandRepo interface {
	Create(ctx context.Context, col *Collection) error
	Update(ctx context.Context, col *Collection) error
	Delete(ctx context.Context, id string) error
	AddItem(ctx context.Context, collectionID, mediaItemID string) error
	RemoveItem(ctx context.Context, collectionID, mediaItemID string) error
}

type CollectionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*Collection, error)
	List(ctx context.Context) ([]*Collection, error)
	GetSmartItems(ctx context.Context, col *Collection, limit, offset int) ([]*MediaItem, error)
	CountSmartItems(ctx context.Context, col *Collection) (int, error)
	GetManualItems(ctx context.Context, collectionID string, limit, offset int) ([]*MediaItem, error)
	CountManualItems(ctx context.Context, collectionID string) (int, error)
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
	ListEnabled(ctx context.Context) ([]*ScanSchedule, error)
	UpdateAfterRun(ctx context.Context, libraryID string, lastRun, nextRun time.Time) error
}

type SearchHit struct {
	ID               string
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
	RemoveMediaItem(ctx context.Context, id string) error
	Search(ctx context.Context, query string, filters map[string]string, limit int64) ([]SearchHit, int64, error)
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
	GetByMediaItemID(ctx context.Context, mediaItemID string) (*TrickplayManifest, error)
	Delete(ctx context.Context, mediaItemID string) error
	ListMissingMediaItemIDs(ctx context.Context) ([]string, error)
}

type TrickplayResult struct {
	ImageDir   string
	ImageCount int
}

type TrickplayGenerator interface {
	Generate(ctx context.Context, filePath string, outputDir string, interval, width, tileWidth, tileHeight int) (*TrickplayResult, error)
}
