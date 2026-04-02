package feed

import "time"

type FeedConfigChanged struct {
	FeedType   Type
	ProfileID  string // empty for admin config
	OccurredAt time.Time
}

func (e FeedConfigChanged) EventName() string { return "feed.config.changed" }

type FeedConfigRemoved struct {
	FeedType   Type
	ProfileID  string // empty for admin config
	OccurredAt time.Time
}

func (e FeedConfigRemoved) EventName() string { return "feed.config.removed" }
