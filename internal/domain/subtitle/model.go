package subtitle

import (
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type SubtitleDownload struct {
	ID           string
	MediaItemID  shared.MediaItemID
	Language     string
	Format       string
	FilePath     string
	Source       string
	DownloadedAt time.Time
}

type SearchResult struct {
	FileID   int
	FileName string
	Language string
	Format   string
	Rating   float64
	Hash     string
}

func (d *SubtitleDownload) Validate() error {
	if d.ID == "" {
		return errors.New("subtitle download requires an ID")
	}
	if d.MediaItemID == "" {
		return errors.New("subtitle download requires a media item ID")
	}
	if d.Language == "" {
		return errors.New("subtitle download requires a language")
	}
	if d.FilePath == "" {
		return errors.New("subtitle download requires a file path")
	}
	return nil
}

func (r *SearchResult) Validate() error {
	if r.FileName == "" {
		return errors.New("search result requires a file name")
	}
	if r.Language == "" {
		return errors.New("search result requires a language")
	}
	return nil
}

type MediaItemInfo struct {
	ID       string
	FilePath string
	Title    string
}

type SubtitleDownloadOption func(*SubtitleDownload)

func WithSubtitleFormat(format string) SubtitleDownloadOption {
	return func(d *SubtitleDownload) { d.Format = format }
}

func WithSubtitleSource(source string) SubtitleDownloadOption {
	return func(d *SubtitleDownload) { d.Source = source }
}

// NewSubtitleDownload is the canonical constructor. Stamps DownloadedAt
// and validates. Required identity fields (id, mediaItem, language,
// filePath) are surfaced as params so construction never succeeds with
// them missing.
func NewSubtitleDownload(id string, mediaItemID shared.MediaItemID, language, filePath string, opts ...SubtitleDownloadOption) (*SubtitleDownload, error) {
	d := &SubtitleDownload{
		ID:           id,
		MediaItemID:  mediaItemID,
		Language:     language,
		FilePath:     filePath,
		DownloadedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(d)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return d, nil
}
