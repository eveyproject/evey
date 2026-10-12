package music

import (
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type Track struct {
	ID          string
	LibraryID   shared.LibraryID
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Year        int
	TrackNumber int
	DiskNumber  int
	Duration    float64
	FilePath    string
	FileSize    int64
	FileHash    string
	ModTime     time.Time
	MimeType    string
	Bitrate     int
	SampleRate  int
	Channels    int
	Genre       string
	CoverArtURL string
	// AlbumFolder is the directory containing the file; it is the
	// authoritative album identity. ID3 tags remain the source of the
	// display metadata but can be inconsistent across a folder
	// (featurings, compilations) so grouping by folder avoids splitting
	// a single on-disk album into multiple phantom rows.
	AlbumFolder string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (t *Track) Validate() error {
	if t.ID == "" {
		return errors.New("track requires an ID")
	}
	if t.LibraryID == "" {
		return errors.New("track requires a library ID")
	}
	if t.Title == "" {
		return errors.New("track requires a title")
	}
	if t.FilePath == "" {
		return errors.New("track requires a file path")
	}
	return nil
}

type Album struct {
	ID          string
	Title       string
	Artist      string
	Year        int
	CoverArtURL string
	TrackCount  int
	Duration    float64
}

type Artist struct {
	ID          string
	Name        string
	TrackCount  int
	AlbumCount  int
	CoverArtURL string
}

type TrackOption func(*Track)

func WithTrackArtist(artist, albumArtist string) TrackOption {
	return func(t *Track) { t.Artist, t.AlbumArtist = artist, albumArtist }
}

func WithTrackAlbum(album string) TrackOption {
	return func(t *Track) { t.Album = album }
}

func WithTrackYear(year int) TrackOption {
	return func(t *Track) { t.Year = year }
}

func WithTrackNumbers(trackNum, diskNum int) TrackOption {
	return func(t *Track) { t.TrackNumber, t.DiskNumber = trackNum, diskNum }
}

func WithTrackFile(size int64, mimeType string) TrackOption {
	return func(t *Track) { t.FileSize, t.MimeType = size, mimeType }
}

func WithTrackGenre(genre string) TrackOption {
	return func(t *Track) { t.Genre = genre }
}

func WithTrackCoverArt(url string) TrackOption {
	return func(t *Track) { t.CoverArtURL = url }
}

// NewTrack is the canonical constructor. Validates. Required identity
// fields (id, library, title, filePath) are surfaced as params so
// construction never succeeds with them missing. FileHash / ModTime /
// probe-derived fields (Bitrate/SampleRate/Channels/Duration) are
// stamped by the scanner after construction.
func NewTrack(id string, libraryID shared.LibraryID, title, filePath string, opts ...TrackOption) (*Track, error) {
	t := &Track{
		ID:        id,
		LibraryID: libraryID,
		Title:     title,
		FilePath:  filePath,
	}
	for _, opt := range opts {
		opt(t)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}
