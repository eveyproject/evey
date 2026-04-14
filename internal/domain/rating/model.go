package rating

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type Rating struct {
	MediaItemID shared.MediaItemID
	ProfileID   shared.ProfileID
	Value       RatingValue
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Rating) Validate() error {
	if r.MediaItemID == "" {
		return errors.New("rating requires a media item ID")
	}
	if r.ProfileID == "" {
		return errors.New("rating requires a profile ID")
	}
	return nil
}

type Aggregate struct {
	Average float64
	Count   int
}
