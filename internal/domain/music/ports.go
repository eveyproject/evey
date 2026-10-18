package music

import (
	"context"
	"io"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// TrackReader is the read-only surface of the track store.
type TrackReader interface {
	GetByID(ctx context.Context, id string) (*Track, error)
	ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]Track, int, error)
	// CountByLibrary returns just the number of tracks in a library. See the
	// note on photo.Repository.CountByLibrary: the list query pays a
	// COUNT(*) OVER() over the whole library, so it is the wrong counter.
	CountByLibrary(ctx context.Context, libraryID shared.LibraryID) (int, error)
	// ListByLibraryCursor keysets ListByLibrary's page instead of OFFSET-ing
	// into it, and does not carry a COUNT(*) OVER() window - see
	// photo.Repository.ListByLibraryCursor for why that matters. Callers that
	// need a total call CountByLibrary separately. nil cursor = first page.
	ListByLibraryCursor(ctx context.Context, libraryID shared.LibraryID, search string, cursor *TrackCursor, limit int) ([]Track, error)
	ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]Track, error)
	ListByAlbum(ctx context.Context, album, artist string) ([]Track, error)
	ListByLibraryAndAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]Track, error)
	ListByAlbumFolder(ctx context.Context, libraryID shared.LibraryID, albumFolder string) ([]Track, error)
	// ListAlbumFolders returns the distinct album_folder values in a library,
	// used to resolve a short opaque album id (AlbumID) back to its folder.
	ListAlbumFolders(ctx context.Context, libraryID shared.LibraryID) ([]string, error)
	ListByArtist(ctx context.Context, artist string, limit, offset int) ([]Track, int, error)
	// ListByArtistCursor is the keyset form of ListByArtist. See ListByLibraryCursor.
	ListByArtistCursor(ctx context.Context, artist string, cursor *ArtistTrackCursor, limit int) ([]Track, error)
	Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]Track, error)
	GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]Album, error)
	GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]Artist, error)
	// ListAllAlbums returns one AlbumKey per album folder (folders we can
	// represent as a fetchable artist+album). The caller decides which need
	// art by checking the on-disk file, so an album whose cover_art_url is set
	// in the DB but whose cached image was wiped gets re-fetched rather than
	// staying silently broken.
	ListAllAlbums(ctx context.Context, libraryID shared.LibraryID) ([]AlbumKey, error)
	// SampleByLibrary returns up to limit tracks of the library drawn at
	// random, a fresh draw on every call (the "shuffle" of the whole library,
	// not of the tracks a client happened to have loaded).
	SampleByLibrary(ctx context.Context, libraryID shared.LibraryID, limit int) ([]Track, error)
}

// TrackWriter is the write surface of the track store.
type TrackWriter interface {
	// Upsert inserts or updates the track (ON CONFLICT by file path) and
	// returns the PERSISTED id. On conflict the existing row keeps its id, so
	// the returned id may differ from t.ID - callers that publish events must
	// use the returned id, not the locally-minted one, or they reference a
	// row that does not exist.
	Upsert(ctx context.Context, t Track) (string, error)
	Delete(ctx context.Context, id string) error
	// DeleteByIDs removes the rows in one statement - the scan prune's path, so
	// a library that lost thousands of files is not thousands of round trips.
	// Ids that match no row are ignored.
	DeleteByIDs(ctx context.Context, ids []string) error
	UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error
	SetAlbumCoverArtURL(ctx context.Context, libraryID shared.LibraryID, albumFolder, url string) error

	// SetTrackCoverArtURL fixe la pochette d'UNE piste. Sert à restaurer une
	// pochette embarquée qu'un fetch externe avait écrasée : un scan ne
	// ré-upsert pas un fichier inchangé, donc sans ce chemin la pochette du
	// fichier ne revient jamais d'elle-même.
	SetTrackCoverArtURL(ctx context.Context, trackID, url string) error
}

// TrackRepository is the full track store surface (reads + writes).
type TrackRepository interface {
	TrackReader
	TrackWriter
}

// TrackCursor keysets ListByLibrary by the same tuple its ORDER BY sorts on:
// album_artist, album, disk_number, track_number, id (all ascending). ID is
// the mandatory tiebreaker - disk/track numbers repeat across albums, and
// even within one album for multi-artist compilations, so without it a page
// boundary can silently skip or repeat a row.
type TrackCursor struct {
	AlbumArtist string
	Album       string
	DiskNumber  int
	TrackNumber int
	ID          string
}

// ArtistTrackCursor keysets ListByArtist by the tuple its ORDER BY sorts on:
// album, disk_number, track_number, id (all ascending). Same tiebreaker
// rationale as TrackCursor.
type ArtistTrackCursor struct {
	Album       string
	DiskNumber  int
	TrackNumber int
	ID          string
}

// MusicSearchIndex is the full-text search index for music tracks (Meilisearch
// in prod, a SQL fallback when it's down). Documents carry the full track so a
// hit is directly playable without a second DB round-trip. An empty libraryID
// in Search means "every library" (the global search bar); the caller is
// responsible for filtering hits down to the libraries the user may access.
type MusicSearchIndex interface {
	IndexTrack(ctx context.Context, t Track) error
	IndexTracksBatch(ctx context.Context, tracks []Track) error
	RemoveTrack(ctx context.Context, id string) error
	Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]Track, error)
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
// Artist are display-only strings - the most frequent values across the
// folder - and are used by external art fetchers that still key on
// (artist, album) tuples.
type AlbumKey struct {
	AlbumFolder string
	Artist      string
	Album       string
	// HasEmbeddedArt vaut true dès qu'une piste du dossier porte une pochette
	// lue dans le fichier audio lui-même. Un fetcher externe doit alors
	// s'abstenir : le fichier est la source autoritaire, et une devinette
	// externe ne peut que la contredire.
	HasEmbeddedArt bool
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
	// Exists reports whether the file is really on disk. The scan prune path
	// double-checks with it before deleting a row, so a transient per-file
	// walk error can never read as "the file was removed".
	Exists(path string) bool
	OpenFile(path string) (ReadSeekCloser, error)
	ComputeHash(path string) (string, error)
}
