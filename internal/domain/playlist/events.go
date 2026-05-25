package playlist

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type PlaylistCreated struct {
	ID         string
	ProfileID  shared.ProfileID
	Name       string
	OccurredAt time.Time
}

func (e PlaylistCreated) EventName() string { return "playlist.created" }

type PlaylistUpdated struct {
	ID         string
	OccurredAt time.Time
}

func (e PlaylistUpdated) EventName() string { return "playlist.updated" }

type ItemAdded struct {
	PlaylistID  string
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e ItemAdded) EventName() string { return "playlist.item.added" }

type ItemRemoved struct {
	PlaylistID  string
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e ItemRemoved) EventName() string { return "playlist.item.removed" }
