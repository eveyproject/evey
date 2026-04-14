package favorite

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type Favorite struct {
	ID          string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
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
