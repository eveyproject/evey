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
	LibraryID   *string
	LibraryIDs  []string // filter to these library IDs (nil = no filter)
	ParentID    *string
	Type        *string
	Genres      []string
	Year        *int
	SortField   string
	SortOrder   string
	Limit       int
	Offset      int
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

type CollectionRepository interface {
	Create(ctx context.Context, col *Collection) error
	GetByID(ctx context.Context, id string) (*Collection, error)
	List(ctx context.Context) ([]*Collection, error)
	Update(ctx context.Context, col *Collection) error
	Delete(ctx context.Context, id string) error
	AddItem(ctx context.Context, collectionID, mediaItemID string) error
	RemoveItem(ctx context.Context, collectionID, mediaItemID string) error
	GetItems(ctx context.Context, collectionID string) ([]*MediaItem, error)
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

type SearchIndex interface {
	IndexMediaItem(ctx context.Context, item *MediaItem) error
	RemoveMediaItem(ctx context.Context, id string) error
	Search(ctx context.Context, query string, filters map[string]string) ([]string, error)
}
