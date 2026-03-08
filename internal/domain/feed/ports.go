package feed

import "context"

type AdminConfigRepository interface {
	GetAll(ctx context.Context) ([]*AdminConfig, error)
	Upsert(ctx context.Context, cfg *AdminConfig) error
}

type ProfileConfigRepository interface {
	GetByProfile(ctx context.Context, profileID string) ([]*ProfileConfig, error)
	Upsert(ctx context.Context, cfg *ProfileConfig) error
	Delete(ctx context.Context, profileID string, feedType Type) error
}

type FeedQuerier interface {
	ContinueWatching(ctx context.Context, profileID string, limit int) ([]*FeedItem, error)
	RecentlyAdded(ctx context.Context, daysWindow, limit int) ([]*FeedItem, error)
	Recommended(ctx context.Context, profileID string, genres []string, limit int) ([]*FeedItem, error)
	SimilarItems(ctx context.Context, mediaItemID, profileID string, limit int) ([]*FeedItem, error)
}
