package feed

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type FeedConfigChanged struct {
	FeedType   Type
	ProfileID  shared.ProfileID // empty for admin config
	OccurredAt time.Time
}

func (e FeedConfigChanged) EventName() string { return "feed.config.changed" }

type FeedConfigRemoved struct {
	FeedType   Type
	ProfileID  shared.ProfileID // empty for admin config
	OccurredAt time.Time
}

func (e FeedConfigRemoved) EventName() string { return "feed.config.removed" }
