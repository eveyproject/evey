package shared

import (
	"context"
	"time"
)

type ServerConfigEntry struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

type ServerConfigRepository interface {
	Get(ctx context.Context, key string) (*ServerConfigEntry, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]*ServerConfigEntry, error)
}
