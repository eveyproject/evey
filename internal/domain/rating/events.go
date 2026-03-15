package rating

import "time"

type MediaRated struct {
	UserID     string
	MediaID    string
	Rating     int
	OccurredAt time.Time
}

func (e MediaRated) EventName() string { return "rating.media.rated" }

type MediaRatingDeleted struct {
	UserID     string
	MediaID    string
	OccurredAt time.Time
}

func (e MediaRatingDeleted) EventName() string { return "rating.media.deleted" }

// RatingSet and RatingDeleted satisfy ARC-NEW-002.
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
