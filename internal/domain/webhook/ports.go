package webhook

import "context"

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
}
