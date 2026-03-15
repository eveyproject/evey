package music

import "time"

type Track struct {
	ID          string
	LibraryID   string
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
