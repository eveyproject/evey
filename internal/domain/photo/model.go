package photo

import (
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/id"
)

type Photo struct {
	ID          string
	LibraryID   shared.LibraryID
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
	// Blurhash is a compact (~30-char) placeholder computed at scan time.
	// Empty when the file pre-dates migration 000096 or the encoder
	// couldn't decode it. Clients show a coloured preview while the full
	// image loads (Wolt blurhash spec).
	Blurhash  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewPhoto is the canonical constructor: mints the ID, stamps
// CreatedAt/UpdatedAt, copies the struct-args fields, and calls
// Validate before returning. Callers should prefer this over
// building a `Photo{...}` literal so invariants are enforced
// at construction rather than discovered at Upsert time.
func NewPhoto(libraryID shared.LibraryID, filePath string, size int64, modTime time.Time, opts ...Option) (*Photo, error) {
	now := time.Now()
	p := &Photo{
		ID:        id.New("photo-"),
		LibraryID: libraryID,
		FilePath:  filePath,
		FileSize:  size,
		ModTime:   modTime,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, opt := range opts {
		opt(p)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Option customises a Photo at construction. Non-required fields
// use options to keep the NewPhoto signature small as metadata
// (dimensions, EXIF, geoloc, …) evolves.
type Option func(*Photo)

func WithTitle(title string) Option       { return func(p *Photo) { p.Title = title } }
func WithHash(hash string) Option         { return func(p *Photo) { p.FileHash = hash } }
func WithMimeType(mime string) Option     { return func(p *Photo) { p.MimeType = mime } }
func WithDimensions(w, h int) Option      { return func(p *Photo) { p.Width, p.Height = w, h } }
func WithTakenAt(t *time.Time) Option     { return func(p *Photo) { p.TakenAt = t } }
func WithCameraModel(model string) Option { return func(p *Photo) { p.CameraModel = model } }
func WithAlbum(album string) Option       { return func(p *Photo) { p.Album = album } }
func WithBlurhash(hash string) Option     { return func(p *Photo) { p.Blurhash = hash } }

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
	LibraryID    shared.LibraryID
	PhotoCount   int
	CoverPhotoID string
}
