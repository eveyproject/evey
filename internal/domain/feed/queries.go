package feed

import "context"

type ConfigQueryRepo interface {
	GetAdminConfig(ctx context.Context) ([]*AdminConfig, error)
	GetProfileConfig(ctx context.Context, profileID string) ([]*ProfileConfig, error)
}
