package rating

import "time"

type Rating struct {
	MediaItemID string
	ProfileID   string
	Value       RatingValue
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Aggregate struct {
	Average float64
	Count   int
}
