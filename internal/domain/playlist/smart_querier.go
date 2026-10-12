package playlist

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// SmartPlaylistQuerier resolves a smart playlist Spec to a sorted, limited
// list of media item IDs. Item hydration is the app layer's responsibility,
// which keeps this port free of library.MediaItem and makes the playlist
// domain self-contained.
type SmartPlaylistQuerier interface {
	Query(ctx context.Context, spec Spec, sortBy, sortOrder string, limit int) ([]shared.MediaItemID, error)
}
