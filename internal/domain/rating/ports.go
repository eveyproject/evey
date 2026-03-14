package rating

import "context"

type Repository interface {
	Upsert(ctx context.Context, r Rating) error
	Get(ctx context.Context, mediaItemID, profileID string) (*Rating, error)
	Delete(ctx context.Context, mediaItemID, profileID string) error
	GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]Aggregate, error)
}
