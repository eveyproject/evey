package feed

import "context"

type ConfigCommandRepo interface {
	UpsertAdminConfig(ctx context.Context, cfg *AdminConfig) error
	UpsertProfileConfig(ctx context.Context, cfg *ProfileConfig) error
	DeleteProfileConfig(ctx context.Context, profileID string, feedType Type) error
}
