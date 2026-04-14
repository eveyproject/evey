package library

import (
	"context"

	"github.com/evey-project/evey/internal/domain/shared"
)

type LibraryQueryRepo interface {
	List(ctx context.Context) ([]*Library, error)
	GetByID(ctx context.Context, id shared.LibraryID) (*Library, error)
}

type CastMember struct {
	PersonID      string
	PersonName    string
	ThumbPath     *string
	RoleType      string
	CharacterName *string
	SortOrder     int
}

type MediaItemQueryRepo interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*MediaItem, error)
	GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*MediaItem, error)
	List(ctx context.Context, filter ListMediaItemsFilter) ([]*MediaItem, error)
	Count(ctx context.Context, filter ListMediaItemsFilter) (int, error)
	GetCastByMediaItem(ctx context.Context, mediaItemID shared.MediaItemID) ([]CastMember, error)
	// GetCastForItems fetches cast for multiple media items in one query (batch to avoid N+1).
	GetCastForItems(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]CastMember, error)
}
