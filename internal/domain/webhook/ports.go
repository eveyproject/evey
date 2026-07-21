package webhook

import (
	"context"
	"time"
)

type Repository interface {
	Create(ctx context.Context, cfg *Config) error
	GetByID(ctx context.Context, id string) (*Config, error)
	List(ctx context.Context) ([]*Config, error)
	Update(ctx context.Context, cfg *Config) error
	Delete(ctx context.Context, id string) error
}

type DeliveryRepository interface {
	Create(ctx context.Context, d *Delivery) error
	ListByWebhook(ctx context.Context, webhookID string, limit int) ([]*Delivery, error)

	// Purge deletes deliveries older than before, returning how many went.
	//
	// The table is append-only otherwise - one row per delivery attempt, for
	// the lifetime of the deployment - and it exists purely as an operator
	// diagnostic. Nothing needs a delivery from six months ago, but without
	// this sweep every one of them was kept.
	Purge(ctx context.Context, before time.Time) (int64, error)
}

// Sender delivers webhook HTTP requests.
type Sender interface {
	Send(ctx context.Context, url string, headers map[string]string, body []byte) (statusCode int, err error)
}
