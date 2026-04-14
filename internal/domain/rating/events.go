package rating

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type RatingSet struct {
	MediaID    shared.MediaItemID
	UserID     shared.UserID
	Score      int
	OccurredAt time.Time
}

func (e RatingSet) EventName() string { return "rating.set" }

type RatingDeleted struct {
	MediaID    shared.MediaItemID
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e RatingDeleted) EventName() string { return "rating.deleted" }

type RatingRemoved struct {
	MediaID    shared.MediaItemID
	UserID     shared.UserID
	OccurredAt time.Time
}

func (e RatingRemoved) EventName() string { return "rating.removed" }
