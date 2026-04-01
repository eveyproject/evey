package favorite

import "time"

type FavoriteAdded struct {
	UserID      string
	MediaItemID string
	OccurredAt  time.Time
}

func (e FavoriteAdded) EventName() string { return "favorite.added" }

type FavoriteRemoved struct {
	UserID      string
	MediaItemID string
	OccurredAt  time.Time
}

func (e FavoriteRemoved) EventName() string { return "favorite.removed" }
