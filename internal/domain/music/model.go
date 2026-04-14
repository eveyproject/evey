package music

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (t *Track) Validate() error {
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
	ID         string
	Name       string
	TrackCount int
	AlbumCount int
}
