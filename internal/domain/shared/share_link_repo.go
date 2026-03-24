package shared

import "context"

type ShareLinkRepository interface {
	Create(ctx context.Context, link *ShareLink) error
	GetByToken(ctx context.Context, token string) (*ShareLink, error)
	ListByUser(ctx context.Context, userID string) ([]*ShareLink, error)
	ListAll(ctx context.Context) ([]*ShareLink, error)
	IncrementViewCount(ctx context.Context, token string) error
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) (int, error)
}
