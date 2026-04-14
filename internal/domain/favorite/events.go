package favorite

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type FavoriteAdded struct {
	UserID      shared.UserID
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e FavoriteAdded) EventName() string { return "favorite.added" }

type FavoriteRemoved struct {
	UserID      shared.UserID
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e FavoriteRemoved) EventName() string { return "favorite.removed" }
