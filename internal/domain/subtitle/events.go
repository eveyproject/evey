package subtitle

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type SubtitleDownloaded struct {
	MediaItemID shared.MediaItemID
	Language    string
	Format      string
	Source      string
	OccurredAt  time.Time
}

func (e SubtitleDownloaded) EventName() string { return "subtitle.downloaded" }

type SubtitleDeleted struct {
	MediaItemID shared.MediaItemID
	Language    string
	OccurredAt  time.Time
}

func (e SubtitleDeleted) EventName() string { return "subtitle.deleted" }
