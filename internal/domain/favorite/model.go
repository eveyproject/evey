package favorite

import (
	"errors"
	"time"
)

type Favorite struct {
	ID          string
	ProfileID   string
	MediaItemID string
	CreatedAt   time.Time
}

func (f *Favorite) Validate() error {
	if f.ProfileID == "" {
		return errors.New("favorite requires a profile ID")
	}
	if f.MediaItemID == "" {
		return errors.New("favorite requires a media item ID")
	}
	return nil
}
