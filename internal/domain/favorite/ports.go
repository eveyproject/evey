package favorite

import "context"

type Repository interface {
	Add(ctx context.Context, fav *Favorite) error
	Remove(ctx context.Context, profileID, mediaItemID string) error
	List(ctx context.Context, profileID string, limit, offset int) ([]*Favorite, error)
	IsFavorite(ctx context.Context, profileID, mediaItemID string) (bool, error)
	// IsFavoriteMany answers for several media items in one round trip. The
	// returned set holds only the favorited ids - an absent id is not favorited.
	IsFavoriteMany(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]bool, error)
}
