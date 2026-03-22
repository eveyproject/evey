package library

import "context"

type LibraryQueryRepo interface {
	List(ctx context.Context) ([]*Library, error)
	GetByID(ctx context.Context, id string) (*Library, error)
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
	GetByID(ctx context.Context, id string) (*MediaItem, error)
	GetByIDs(ctx context.Context, ids []string) ([]*MediaItem, error)
	List(ctx context.Context, filter ListMediaItemsFilter) ([]*MediaItem, error)
	Count(ctx context.Context, filter ListMediaItemsFilter) (int, error)
	GetCastByMediaItem(ctx context.Context, mediaItemID string) ([]CastMember, error)
	// GetCastForItems fetches cast for multiple media items in one query (batch to avoid N+1).
	GetCastForItems(ctx context.Context, mediaItemIDs []string) (map[string][]CastMember, error)
}
