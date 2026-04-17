package music

import (
	"context"
	"io"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type TrackRepository interface {
	Upsert(ctx context.Context, t Track) error
	GetByID(ctx context.Context, id string) (*Track, error)
	Delete(ctx context.Context, id string) error
	ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]Track, int, error)
	ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]Track, error)
	ListByAlbum(ctx context.Context, album, artist string) ([]Track, error)
	ListByLibraryAndAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]Track, error)
	ListByArtist(ctx context.Context, artist string, limit, offset int) ([]Track, int, error)
	Search(ctx context.Context, query string, limit int) ([]Track, error)
	GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]Album, error)
	GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]Artist, error)
	UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error
	ListAlbumsMissingArt(ctx context.Context, libraryID shared.LibraryID) ([]AlbumKey, error)
	SetAlbumCoverArtURL(ctx context.Context, libraryID shared.LibraryID, album, albumArtist, url string) error
}

// AlbumKey identifies a distinct (artist, album) pair within a library.
type AlbumKey struct {
	Artist string
	Album  string
}

// ScannedAudioFile represents a discovered audio file during directory walking.
type ScannedAudioFile struct {
	Path     string
	Size     int64
	ModTime  time.Time
	MimeType string
}

// ReadSeekCloser combines io.ReadSeeker with io.Closer.
type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// MusicScanner abstracts filesystem operations needed by music scanning.
type MusicScanner interface {
	WalkAudioFiles(ctx context.Context, dir string, fn func(ScannedAudioFile) error) error
	OpenFile(path string) (ReadSeekCloser, error)
	ComputeHash(path string) (string, error)
}
