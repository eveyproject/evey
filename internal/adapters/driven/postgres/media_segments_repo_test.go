package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpsertUnlessManual must atomically skip the write when the existing
// (media_item_id, type) row is an operator mark, so a re-run of auto-detection
// can't clobber a manual intro/credits the operator set.
func TestMediaSegmentRepo_UpsertUnlessManual(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("seglib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("segmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	// 1) First auto upsert into an empty (item, type) slot → applied.
	auto := &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 0, EndMs: 60_000, Confidence: 0.85, Source: "phash"}
	applied, err := segRepo.UpsertUnlessManual(ctx, auto)
	require.NoError(t, err)
	assert.True(t, applied, "first auto upsert into an empty slot must apply")

	// 2) The operator sets a manual mark (unconditional Upsert always wins).
	manual := &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 1_000, EndMs: 30_000, Confidence: 1.0, Source: "manual"}
	require.NoError(t, segRepo.Upsert(ctx, manual))

	// 3) A later/concurrent auto upsert must NOT overwrite the manual mark.
	auto2 := &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 5_000, EndMs: 90_000, Confidence: 0.7, Source: "phash"}
	applied, err = segRepo.UpsertUnlessManual(ctx, auto2)
	require.NoError(t, err)
	assert.False(t, applied, "auto upsert must be skipped when a manual lock exists")

	got, err := segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "manual", got[0].Source, "the manual mark must survive the auto upsert")
	assert.Equal(t, int64(1_000), got[0].StartMs)
	assert.Equal(t, int64(30_000), got[0].EndMs)

	// 4) A manual_propagated mark is also protected.
	prop := &library.MediaSegment{MediaItemID: item.ID, Type: "end_credits", StartMs: 200, EndMs: 400, Confidence: 0.85, Source: "manual_propagated"}
	require.NoError(t, segRepo.Upsert(ctx, prop))
	applied, err = segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "end_credits", StartMs: 1, EndMs: 2, Confidence: 0.7, Source: "phash"})
	require.NoError(t, err)
	assert.False(t, applied, "auto upsert must be skipped when a manual_propagated lock exists")

	// 5) A non-manual existing row of a fresh type is updated normally.
	applied, err = segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "recap", StartMs: 0, EndMs: 10_000, Confidence: 0.6, Source: "phash"})
	require.NoError(t, err)
	assert.True(t, applied)
	applied, err = segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "recap", StartMs: 0, EndMs: 20_000, Confidence: 0.6, Source: "blackdetect"})
	require.NoError(t, err)
	assert.True(t, applied, "a non-manual existing row must be overwritable by auto-detection")
}
