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

// Why a session ended, carried by PlaybackStopped.
const (
	// StopReasonClient: the client said stop.
	StopReasonClient = "client"
	// StopReasonStale: no heartbeat for long enough, the server ended it.
	StopReasonStale = "stale"
)

type PlaybackStopped struct {
	SessionID   string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	// Reason is one of the StopReason constants; empty for an event raised
	// before the field existed.
	Reason     string
	OccurredAt time.Time
}

func (e PlaybackStopped) EventName() string { return "playback.stopped" }

type MediaMarkedWatched struct {
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	// MediaItemIDs are the playable items marked along with MediaItemID when
	// it is a series or a season (its episodes); empty for a playable item.
	MediaItemIDs []shared.MediaItemID
	OccurredAt   time.Time
}

func (e MediaMarkedWatched) EventName() string { return "media.marked.watched" }
