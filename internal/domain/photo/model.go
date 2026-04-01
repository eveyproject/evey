package photo

import (
	"errors"
	"time"
)

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

func (p *Photo) Validate() error {
	if p.LibraryID == "" {
		return errors.New("photo requires a library ID")
	}
	if p.FilePath == "" {
		return errors.New("photo requires a file path")
	}
	return nil
}

type Album struct {
	Name         string
	LibraryID    string
	PhotoCount   int
	CoverPhotoID string
}
