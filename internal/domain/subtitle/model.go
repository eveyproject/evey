package subtitle

import (
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
