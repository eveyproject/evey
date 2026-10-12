package library

import (
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type Favorite struct {
	ID          string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	CreatedAt   time.Time
}

func (f *Favorite) Validate() error {
	if f.ID == "" {
		return errors.New("favorite requires an ID")
	}
	if f.ProfileID == "" {
		return errors.New("favorite requires a profile ID")
	}
	if f.MediaItemID == "" {
		return errors.New("favorite requires a media item ID")
	}
	return nil
}

// NewFavorite is the canonical constructor. Stamps CreatedAt and
// validates. Required identity fields (id, profile, media) are
// surfaced as params so construction never succeeds with them missing.
func NewFavorite(id string, profileID shared.ProfileID, mediaItemID shared.MediaItemID) (*Favorite, error) {
	f := &Favorite{
		ID:          id,
		ProfileID:   profileID,
		MediaItemID: mediaItemID,
		CreatedAt:   time.Now(),
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}
