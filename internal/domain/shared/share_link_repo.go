package shared

import "context"

type ShareLinkRepository interface {
	Create(ctx context.Context, link *ShareLink) error
	GetByID(ctx context.Context, id string) (*ShareLink, error)
	GetByToken(ctx context.Context, token string) (*ShareLink, error)
	ListByUser(ctx context.Context, userID string) ([]*ShareLink, error)
	ListAll(ctx context.Context) ([]*ShareLink, error)
	// ConsumeView spends one view atomically: it succeeds only while the link
	// is unexpired and under its limit, and returns the row after the spend.
	// ErrNotFound means nothing was spent (unknown, expired or exhausted).
	ConsumeView(ctx context.Context, token string) (*ShareLink, error)
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) (int, error)
}
