package subtitle

import (
	"errors"
	"time"
)

type SubtitleDownload struct {
	ID           string
	MediaItemID  string
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
