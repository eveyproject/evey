package photo

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// CommandRepo is the write side of the photo store (ADR-0009). Reads the
// command flows need (scan reconciliation) live in QueryRepo - the concrete
// repository satisfies both, the split only narrows what each call site can
// reach.
type CommandRepo interface {
	Upsert(ctx context.Context, p Photo) error
	Delete(ctx context.Context, id string) error
	UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error
}

// QueryRepo is the read side of the photo store.
type QueryRepo interface {
	GetByID(ctx context.Context, id string) (*Photo, error)
	ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]Photo, int, error)
	// CountByLibrary returns just the number of photos in a library. Callers that
	// only need the count must NOT reach for ListByLibrary with limit=1: that
	// query carries a COUNT(*) OVER() window, so it reads and sorts every row of
	// the library to hand back one.
	CountByLibrary(ctx context.Context, libraryID shared.LibraryID) (int, error)
	ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]Photo, error)
	ListByAlbum(ctx context.Context, libraryID shared.LibraryID, album string, limit, offset int) ([]Photo, int, error)
	// ListByLibraryCursor keysets ListByLibrary's page instead of OFFSET-ing into
	// it. It does not carry a COUNT(*) OVER() window (that window forces Postgres
	// to compute the total before returning even page 1, which dominates the
	// query cost on a large library) - callers that need a total call
	// CountByLibrary separately. nil cursor = first page.
	ListByLibraryCursor(ctx context.Context, libraryID shared.LibraryID, cursor *PhotoCursor, limit int) ([]Photo, error)
	// ListByAlbumCursor is the keyset form of ListByAlbum. See ListByLibraryCursor.
	ListByAlbumCursor(ctx context.Context, libraryID shared.LibraryID, album string, cursor *PhotoCursor, limit int) ([]Photo, error)
	GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]Album, error)
	Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]Photo, error)
}

// Repository is the composite the concrete adapters implement; services hold
// the narrowed halves, never this.
type Repository interface {
	CommandRepo
	QueryRepo
}

// PhotoCursor keysets photo listings by the same tuple the list queries sort
// by: taken_at DESC NULLS LAST, created_at DESC, id DESC. TakenAt mirrors the
// column's nullability (many photos carry no EXIF date). ID is the mandatory
// tiebreaker: taken_at and created_at are not unique on their own (bulk
// imports, same-day photos), so without it a page boundary can silently skip
// or repeat a row.
type PhotoCursor struct {
	TakenAt   *time.Time
	CreatedAt time.Time
	ID        string
}

// ScannedImageFile represents a discovered image file during directory walking.
type ScannedImageFile struct {
	Path     string
	Size     int64
	ModTime  time.Time
	MimeType string
}

// PhotoScanner abstracts filesystem operations needed by photo scanning.
type PhotoScanner interface {
	WalkImageFiles(ctx context.Context, dir string, fn func(ScannedImageFile) error) error
	ReadFile(path string) ([]byte, error)
	ImageDimensions(path string) (width, height int)
	ComputeHash(path string) (string, error)
}
