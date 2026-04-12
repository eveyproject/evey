package subtitle

import "time"

type SubtitleDownloaded struct {
	MediaItemID string
	Language    string
	Format      string
	Source      string
	OccurredAt  time.Time
}

func (e SubtitleDownloaded) EventName() string { return "subtitle.downloaded" }

type SubtitleDeleted struct {
	MediaItemID string
	Language    string
	OccurredAt  time.Time
}

func (e SubtitleDeleted) EventName() string { return "subtitle.deleted" }
