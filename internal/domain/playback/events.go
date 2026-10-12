package playback

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type PlaybackStarted struct {
	SessionID   string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	PlayMethod  string
	OccurredAt  time.Time
}

func (e PlaybackStarted) EventName() string { return "playback.started" }

type PlaybackStopped struct {
	SessionID   string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e PlaybackStopped) EventName() string { return "playback.stopped" }

type MediaMarkedWatched struct {
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e MediaMarkedWatched) EventName() string { return "media.marked.watched" }
