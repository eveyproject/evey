package photo

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type Repository interface {
	Upsert(ctx context.Context, p Photo) error
	GetByID(ctx context.Context, id string) (*Photo, error)
	Delete(ctx context.Context, id string) error
	ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]Photo, int, error)
	// CountByLibrary returns just the number of photos in a library. Callers that
	// only need the count must NOT reach for ListByLibrary with limit=1: that
	// query carries a COUNT(*) OVER() window, so it reads and sorts every row of
	// the library to hand back one.
	CountByLibrary(ctx context.Context, libraryID shared.LibraryID) (int, error)
	ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]Photo, error)
	ListByAlbum(ctx context.Context, libraryID shared.LibraryID, album string, limit, offset int) ([]Photo, int, error)
	GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]Album, error)
	Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]Photo, error)
	UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error
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
