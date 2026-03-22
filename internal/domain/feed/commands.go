package feed

import "context"

type ConfigCommandRepo interface {
	UpsertAdminConfig(ctx context.Context, cfg *AdminConfig) error
	BatchUpsertAdminConfig(ctx context.Context, cfgs []*AdminConfig) error
	UpsertProfileConfig(ctx context.Context, cfg *ProfileConfig) error
	DeleteProfileConfig(ctx context.Context, profileID string, feedType Type) error
	DeleteAdminConfigByType(ctx context.Context, feedType Type) error
}
