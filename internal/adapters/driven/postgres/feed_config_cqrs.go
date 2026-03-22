package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/feed"
)

// FeedConfigCommandRepo implements feed.ConfigCommandRepo (write side).
type FeedConfigCommandRepo struct {
	admin   *FeedAdminConfigRepository
	profile *FeedProfileConfigRepository
}

func NewFeedConfigCommandRepo(db *sql.DB) *FeedConfigCommandRepo {
	return &FeedConfigCommandRepo{
		admin:   NewFeedAdminConfigRepository(db),
		profile: NewFeedProfileConfigRepository(db),
	}
}

func (r *FeedConfigCommandRepo) UpsertAdminConfig(ctx context.Context, cfg *feed.AdminConfig) error {
	return r.admin.Upsert(ctx, cfg)
}

func (r *FeedConfigCommandRepo) UpsertProfileConfig(ctx context.Context, cfg *feed.ProfileConfig) error {
	return r.profile.Upsert(ctx, cfg)
}

func (r *FeedConfigCommandRepo) BatchUpsertAdminConfig(ctx context.Context, cfgs []*feed.AdminConfig) error {
	return r.admin.BatchUpsert(ctx, cfgs)
}

func (r *FeedConfigCommandRepo) DeleteProfileConfig(ctx context.Context, profileID string, feedType feed.Type) error {
	return r.profile.Delete(ctx, profileID, feedType)
}

func (r *FeedConfigCommandRepo) DeleteAdminConfigByType(ctx context.Context, feedType feed.Type) error {
	if err := r.profile.DeleteByType(ctx, feedType); err != nil {
		return err
	}
	return r.admin.DeleteByType(ctx, feedType)
}

// FeedConfigQueryRepo implements feed.ConfigQueryRepo (read side).
type FeedConfigQueryRepo struct {
	admin   *FeedAdminConfigRepository
	profile *FeedProfileConfigRepository
}

func NewFeedConfigQueryRepo(db *sql.DB) *FeedConfigQueryRepo {
	return &FeedConfigQueryRepo{
		admin:   NewFeedAdminConfigRepository(db),
		profile: NewFeedProfileConfigRepository(db),
	}
}

func (r *FeedConfigQueryRepo) GetAdminConfig(ctx context.Context) ([]*feed.AdminConfig, error) {
	return r.admin.GetAll(ctx)
}

func (r *FeedConfigQueryRepo) GetProfileConfig(ctx context.Context, profileID string) ([]*feed.ProfileConfig, error) {
	return r.profile.GetByProfile(ctx, profileID)
}
