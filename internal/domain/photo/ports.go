package photo

import (
	"context"
	"time"
)

type Repository interface {
	Upsert(ctx context.Context, p Photo) error
	GetByID(ctx context.Context, id string) (*Photo, error)
	Delete(ctx context.Context, id string) error
	ListByLibrary(ctx context.Context, libraryID string, limit, offset int) ([]Photo, int, error)
	ListAllByLibrary(ctx context.Context, libraryID string) ([]Photo, error)
	ListByAlbum(ctx context.Context, libraryID, album string, limit, offset int) ([]Photo, int, error)
	GetAlbums(ctx context.Context, libraryID string) ([]Album, error)
	Search(ctx context.Context, libraryID, query string, limit int) ([]Photo, error)
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
