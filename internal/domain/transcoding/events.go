package transcoding

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type TranscodeStarted struct {
	JobID       string
	MediaItemID shared.MediaItemID
	OutputCodec string
	OccurredAt  time.Time
}

func (e TranscodeStarted) EventName() string { return "transcode.started" }

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

// DerivedCacheFull is published when a store of material derived from the
// library - the extracted subtitles under /data - refuses a write because the
// disk is at the operator's free-space floor. Served bytes are unaffected; what
// is lost is the cache, and with it the next viewer's wait. Rate-limited by
// the publisher: one per store per hour.
type DerivedCacheFull struct {
	Store      string // which store, e.g. "subtitles"
	Dir        string
	FreeBytes  int64
	FloorBytes int64
	OccurredAt time.Time
}

func (e DerivedCacheFull) EventName() string { return "transcoding.derived_cache_full" }
