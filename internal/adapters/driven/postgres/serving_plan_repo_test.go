package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServingPlanRepo_Upsert_GetByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	planRepo := postgres.NewServingPlanRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("splib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("spmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	plan := &library.ServingPlan{
		MediaItemID: item.ID,
		Target:      "mse-generic",
		Strategy:    library.ServingStrategyRemuxHLS,
		PlanJSON:    []byte(`{"strategy":"remux-hls","target":"mse-generic"}`),
	}
	require.NoError(t, planRepo.Upsert(ctx, plan))

	got, err := planRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.MediaItemID)
	assert.Equal(t, "mse-generic", got.Target)
	assert.Equal(t, library.ServingStrategyRemuxHLS, got.Strategy)
	assert.JSONEq(t, `{"strategy":"remux-hls","target":"mse-generic"}`, string(got.PlanJSON))
	assert.False(t, got.CreatedAt.IsZero())
}

func TestServingPlanRepo_Upsert_ReplacesExisting_KeepsCreatedAt(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	planRepo := postgres.NewServingPlanRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("splib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("spmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, planRepo.Upsert(ctx, &library.ServingPlan{
		MediaItemID: item.ID, Target: "mse-generic",
		Strategy: library.ServingStrategyRemuxHLS, PlanJSON: []byte(`{"needs_reindex":true}`),
	}))
	first, err := planRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)

	// Repaired + replanned: strategy flips to direct-play, target/plan_json
	// updated, created_at unchanged (the ingest time, not the repair time).
	require.NoError(t, planRepo.Upsert(ctx, &library.ServingPlan{
		MediaItemID: item.ID, Target: "mse-generic",
		Strategy: library.ServingStrategyDirectPlay, PlanJSON: []byte(`{"needs_reindex":false}`),
	}))

	got, err := planRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, library.ServingStrategyDirectPlay, got.Strategy)
	assert.JSONEq(t, `{"needs_reindex":false}`, string(got.PlanJSON))
	assert.Equal(t, first.CreatedAt.UTC(), got.CreatedAt.UTC())
}

func TestServingPlanRepo_GetByMediaItemID_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	planRepo := postgres.NewServingPlanRepository(db)
	_, err := planRepo.GetByMediaItemID(context.Background(), "does-not-exist")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestServingPlanRepo_DeleteByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	planRepo := postgres.NewServingPlanRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("splib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("spmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, planRepo.Upsert(ctx, &library.ServingPlan{
		MediaItemID: item.ID, Target: "mse-generic",
		Strategy: library.ServingStrategyDirectPlay, PlanJSON: []byte(`{}`),
	}))
	require.NoError(t, planRepo.DeleteByMediaItemID(ctx, item.ID))

	_, err := planRepo.GetByMediaItemID(ctx, item.ID)
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestServingPlanRepo_DeletedWithMediaItem_Cascades(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	planRepo := postgres.NewServingPlanRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("splib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("spmi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, planRepo.Upsert(ctx, &library.ServingPlan{
		MediaItemID: item.ID, Target: "mse-generic",
		Strategy: library.ServingStrategyDirectPlay, PlanJSON: []byte(`{}`),
	}))

	require.NoError(t, itemRepo.Delete(ctx, item.ID))

	_, err := planRepo.GetByMediaItemID(ctx, item.ID)
	require.ErrorIs(t, err, shared.ErrNotFound, "plan must be cascade-deleted with its media item")
}
