package photo

import "time"

type Photo struct {
	ID          string
	LibraryID   string
	Title       string
	FilePath    string
	FileSize    int64
	FileHash    string
	ModTime     time.Time
	MimeType    string
	Width       int
	Height      int
	TakenAt     *time.Time
	Latitude    *float64
	Longitude   *float64
	CameraModel string
	Album       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Album struct {
	Name         string
	LibraryID    string
	PhotoCount   int
	CoverPhotoID string
}
