package transcoding

import "time"

type TranscodeStarted struct {
	JobID       string
	MediaItemID string
	OutputCodec string
	OccurredAt  time.Time
}

func (e TranscodeStarted) EventName() string { return "transcode.started" }

type TranscodeCompleted struct {
	JobID       string
	MediaItemID string
	DurationMs  int64
	OccurredAt  time.Time
}

func (e TranscodeCompleted) EventName() string { return "transcode.completed" }

type TranscodeFailed struct {
	JobID        string
	MediaItemID  string
	ErrorMessage string
	OccurredAt   time.Time
}

func (e TranscodeFailed) EventName() string { return "transcode.failed" }

type TranscodeCancelled struct {
	JobID       string
	MediaItemID string
	OccurredAt  time.Time
}

func (e TranscodeCancelled) EventName() string { return "transcode.cancelled" }
