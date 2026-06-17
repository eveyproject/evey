package music

import (
	"context"
	"io"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type TrackRepository interface {
	// Upsert inserts or updates the track (ON CONFLICT by file path) and
	// returns the PERSISTED id. On conflict the existing row keeps its id, so
	// the returned id may differ from t.ID — callers that publish events must
	// use the returned id, not the locally-minted one, or they reference a
	// row that does not exist.
	Upsert(ctx context.Context, t Track) (string, error)
	GetByID(ctx context.Context, id string) (*Track, error)
	Delete(ctx context.Context, id string) error
	ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]Track, int, error)
	ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]Track, error)
	ListByAlbum(ctx context.Context, album, artist string) ([]Track, error)
	ListByLibraryAndAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]Track, error)
	ListByAlbumFolder(ctx context.Context, libraryID shared.LibraryID, albumFolder string) ([]Track, error)
	ListByArtist(ctx context.Context, artist string, limit, offset int) ([]Track, int, error)
	Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]Track, error)
	GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]Album, error)
	GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]Artist, error)
	UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error
	ListAlbumsMissingArt(ctx context.Context, libraryID shared.LibraryID) ([]AlbumKey, error)
	SetAlbumCoverArtURL(ctx context.Context, libraryID shared.LibraryID, albumFolder, url string) error
}

// PlayRepository records and reads per-profile music listening counts. It's the
// taste signal for the playlist generator (favorites/ratings can't apply to
// music tracks, which aren't media_items).
type PlayRepository interface {
	// IncrementPlay bumps the (profile, track) play count by one, stamping the
	// last-played time. Upserts the row on first play.
	IncrementPlay(ctx context.Context, profileID, trackID string) error
	// PlayCountsForProfile returns track id → play count for the profile.
	PlayCountsForProfile(ctx context.Context, profileID string) (map[string]int, error)
}

// AlbumKey identifies a distinct album within a library. AlbumFolder is
// the authoritative identity (a track's parent directory). Album and
// Artist are display-only strings — the most frequent values across the
// folder — and are used by external art fetchers that still key on
// (artist, album) tuples.
type AlbumKey struct {
	AlbumFolder string
	Artist      string
	Album       string
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
