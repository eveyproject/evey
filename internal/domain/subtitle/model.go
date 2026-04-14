package subtitle

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
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
