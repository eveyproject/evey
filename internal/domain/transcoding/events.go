package transcoding

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type TranscodeStarted struct {
	JobID       string
	MediaItemID shared.MediaItemID
	OutputCodec string
	OccurredAt  time.Time
}

func (e TranscodeStarted) EventName() string { return "transcode.started" }

type TranscodeCompleted struct {
	JobID       string
	MediaItemID shared.MediaItemID
	Duration    shared.Duration
	OccurredAt  time.Time
}

func (e TranscodeCompleted) EventName() string { return "transcode.completed" }

type TranscodeFailed struct {
	JobID        string
	MediaItemID  shared.MediaItemID
	ErrorMessage string
	OccurredAt   time.Time
}

func (e TranscodeFailed) EventName() string { return "transcode.failed" }

type TranscodeCancelled struct {
	JobID       string
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e TranscodeCancelled) EventName() string { return "transcode.cancelled" }
