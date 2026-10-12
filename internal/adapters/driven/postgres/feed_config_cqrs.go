package postgres

import (
	"context"
	"database/sql"

	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.UpsertAdminConfig")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.admin.Upsert(ctx, cfg)
}

func (r *FeedConfigCommandRepo) UpsertProfileConfig(ctx context.Context, cfg *feed.ProfileConfig) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.UpsertProfileConfig")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.profile.Upsert(ctx, cfg)
}

func (r *FeedConfigCommandRepo) BatchUpsertAdminConfig(ctx context.Context, cfgs []*feed.AdminConfig) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.BatchUpsertAdminConfig")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.admin.BatchUpsert(ctx, cfgs)
}

func (r *FeedConfigCommandRepo) DeleteProfileConfig(ctx context.Context, profileID string, feedType feed.Type) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.DeleteProfileConfig")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.profile.Delete(ctx, profileID, feedType)
}

func (r *FeedConfigCommandRepo) DeleteAdminConfigByType(ctx context.Context, feedType feed.Type) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.DeleteAdminConfigByType")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.GetAdminConfig")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.admin.GetAll(ctx)
}

func (r *FeedConfigQueryRepo) GetProfileConfig(ctx context.Context, profileID string) ([]*feed.ProfileConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feedconfig.GetProfileConfig")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.profile.GetByProfile(ctx, profileID)
}

var _ feed.ConfigCommandRepo = (*FeedConfigCommandRepo)(nil)
var _ feed.ConfigQueryRepo = (*FeedConfigQueryRepo)(nil)
