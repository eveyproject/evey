package rating

import "time"

type RatingSet struct {
	MediaID    string
	UserID     string
	Score      int
	OccurredAt time.Time
}

func (e RatingSet) EventName() string { return "rating.set" }

type RatingDeleted struct {
	MediaID    string
	UserID     string
	OccurredAt time.Time
}

func (e RatingDeleted) EventName() string { return "rating.deleted" }
