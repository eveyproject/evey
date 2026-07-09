package postgres

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// ServingPlanRepository persists the precomputed mkvgo ops.Ingest decision
// (see internal/domain/library.ServingPlanRepository) one row per media item.
type ServingPlanRepository struct {
	db *sql.DB
}

func NewServingPlanRepository(db *sql.DB) *ServingPlanRepository {
	return &ServingPlanRepository{db: db}
}

// Upsert inserts or replaces the plan. created_at is stamped by the database
// on first insert (DEFAULT now()) and left untouched on a later update - a
// repair-then-replan cycle refreshes the decision, not the original ingest
// time.
func (r *ServingPlanRepository) Upsert(ctx context.Context, plan *library.ServingPlan) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.serving_plan.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(plan.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_serving_plans (media_item_id, target, strategy, plan_json)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   target    = EXCLUDED.target,
		   strategy  = EXCLUDED.strategy,
		   plan_json = EXCLUDED.plan_json`,
		string(plan.MediaItemID), plan.Target, string(plan.Strategy), plan.PlanJSON,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ServingPlanRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) (*library.ServingPlan, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.serving_plan.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	plan := &library.ServingPlan{MediaItemID: mediaItemID}
	var strategy string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT target, strategy, plan_json, created_at
		 FROM media_serving_plans WHERE media_item_id = $1`, string(mediaItemID),
	).Scan(&plan.Target, &strategy, &plan.PlanJSON, &plan.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	plan.Strategy = library.ServingStrategy(strategy)
	return plan, nil
}

func (r *ServingPlanRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.serving_plan.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM media_serving_plans WHERE media_item_id = $1`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ library.ServingPlanRepository = (*ServingPlanRepository)(nil)
