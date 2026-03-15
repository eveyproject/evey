package playback

import "time"

type PlaySessionFilter struct {
	ProfileID *string
	StartFrom *time.Time
	StartTo   *time.Time
	Limit     int
	Offset    int
}

// PlaySessionRepository composes command+query for adapters that implement both.
type PlaySessionRepository interface {
	SessionCommandRepo
	SessionQueryRepo
}
