package favorite

import "time"

type Favorite struct {
	ID          string
	ProfileID   string
	MediaItemID string
	CreatedAt   time.Time
}
