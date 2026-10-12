package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
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

// UpsertUnlessOperatorManual is the propagation-path guard: it must atomically
// skip when the existing row is a hard operator 'manual' mark, but - unlike
// UpsertUnlessManual - it MUST overwrite a 'manual_propagated' row, so a
// re-run of propagation can refresh its own earlier work.
func TestMediaSegmentRepo_UpsertUnlessOperatorManual(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("opseglib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("opsegmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	// 1) Operator 'manual' mark is protected against propagation.
	manual := &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 1_000, EndMs: 30_000, Confidence: 1.0, Source: "manual"}
	require.NoError(t, segRepo.Upsert(ctx, manual))
	applied, err := segRepo.UpsertUnlessOperatorManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 5_000, EndMs: 90_000, Confidence: 0.85, Source: "manual_propagated"})
	require.NoError(t, err)
	assert.False(t, applied, "propagation must NOT clobber an operator manual mark")
	got, err := segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "manual", got[0].Source, "the operator manual mark must survive")

	// 2) A 'manual_propagated' row IS overwritable (the key difference from
	//    UpsertUnlessManual): a re-run refreshes its own propagation.
	prop := &library.MediaSegment{MediaItemID: item.ID, Type: "end_credits", StartMs: 200, EndMs: 400, Confidence: 0.85, Source: "manual_propagated"}
	require.NoError(t, segRepo.Upsert(ctx, prop))
	applied, err = segRepo.UpsertUnlessOperatorManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "end_credits", StartMs: 250, EndMs: 500, Confidence: 0.85, Source: "manual_propagated"})
	require.NoError(t, err)
	assert.True(t, applied, "a manual_propagated row must be refreshable by a re-run of propagation")
	got, err = segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	for _, g := range got {
		if g.Type == "end_credits" {
			assert.Equal(t, int64(250), g.StartMs, "the re-propagation must have updated the window")
		}
	}
}

// The bounded, version-gated intro-detection attempt counter: increments at the
// same algo version, resets on a bump, and exhausts only at the matching version.
// Exercised through the batch API - ids absent from the returned map are the
// "not exhausted" case (never attempted, under the cap, or an older version).
func TestMediaSegmentRepo_IntroDetectionAttempts(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("introlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("introep", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	id := string(item.ID)

	// Never attempted → not exhausted.
	ex, err := segRepo.IntroDetectionExhaustedBatch(ctx, []string{id}, 1, 2)
	require.NoError(t, err)
	assert.False(t, ex[id], "an item never attempted must be eligible")

	// One attempt at v1 → under the cap.
	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, id, 1))
	ex, err = segRepo.IntroDetectionExhaustedBatch(ctx, []string{id}, 1, 2)
	require.NoError(t, err)
	assert.False(t, ex[id], "1 attempt < cap 2")

	// Second attempt at v1 → hits the cap.
	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, id, 1))
	ex, err = segRepo.IntroDetectionExhaustedBatch(ctx, []string{id}, 1, 2)
	require.NoError(t, err)
	assert.True(t, ex[id], "2 >= cap 2 at the current version → exhausted")

	// A newer algorithm version re-enables detection (a quality improvement).
	ex, err = segRepo.IntroDetectionExhaustedBatch(ctx, []string{id}, 2, 2)
	require.NoError(t, err)
	assert.False(t, ex[id], "exhaustion is version-scoped; a bump re-enables the item")

	// Recording at the new version resets the count to 1 (fresh retry budget).
	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, id, 2))
	ex, err = segRepo.IntroDetectionExhaustedBatch(ctx, []string{id}, 2, 2)
	require.NoError(t, err)
	assert.False(t, ex[id], "a version bump resets attempts to 1")
}

// TestMediaSegmentRepo_IntroDetectionExhaustedBatch_MultipleIDs proves the
// batch query covers several items in a single call, correctly distinguishing
// an exhausted item from an untouched sibling in the same page (the ANY($1)
// query this batching relies on).
func TestMediaSegmentRepo_IntroDetectionExhaustedBatch_MultipleIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("introbatchlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	exhaustedItem := newTestMediaItem("introbatch-exhausted", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, exhaustedItem))
	freshItem := newTestMediaItem("introbatch-fresh", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, freshItem))
	exhaustedID, freshID := string(exhaustedItem.ID), string(freshItem.ID)

	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, exhaustedID, 1))
	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, exhaustedID, 1))

	ex, err := segRepo.IntroDetectionExhaustedBatch(ctx, []string{exhaustedID, freshID}, 1, 2)
	require.NoError(t, err)
	assert.True(t, ex[exhaustedID], "the item attempted twice at the cap must be exhausted")
	assert.False(t, ex[freshID], "an id absent from the underlying table must be absent from the map")
	_, freshPresent := ex[freshID]
	assert.False(t, freshPresent, "not-exhausted ids must be absent, not false-valued, per the port contract")
}

// The season sweep reads a whole page of episodes in one query, keyed by
// item; deletes are scoped to one item, or one item and one segment type.
func TestMediaSegmentRepo_BatchReadAndScopedDeletes(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("segbatchlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	ep1, ep2, ep3 := newTestMediaItem("segb1", lib.ID), newTestMediaItem("segb2", lib.ID), newTestMediaItem("segb3", lib.ID)
	for _, it := range []*library.MediaItem{ep1, ep2, ep3} {
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	segRepo := postgres.NewMediaSegmentRepository(db)
	for _, s := range []*library.MediaSegment{
		{MediaItemID: ep1.ID, Type: "end_credits", StartMs: 2_500_000, EndMs: 2_600_000, Confidence: 0.9, Source: "phash"},
		{MediaItemID: ep1.ID, Type: "intro", StartMs: 0, EndMs: 60_000, Confidence: 0.9, Source: "phash"},
		{MediaItemID: ep2.ID, Type: "intro", StartMs: 1_000, EndMs: 61_000, Confidence: 0.8, Source: "audio"},
	} {
		require.NoError(t, segRepo.Upsert(ctx, s))
	}

	got, err := segRepo.GetByMediaItemIDs(ctx, []shared.MediaItemID{ep1.ID, ep2.ID, ep3.ID, "ghost"})
	require.NoError(t, err)
	require.Len(t, got[ep1.ID], 2)
	assert.Equal(t, "intro", got[ep1.ID][0].Type, "an item's segments come back by start time")
	assert.Equal(t, "end_credits", got[ep1.ID][1].Type)
	require.Len(t, got[ep2.ID], 1)
	assert.NotContains(t, got, ep3.ID, "an item without rows is absent, not an empty slice")
	assert.NotContains(t, got, shared.MediaItemID("ghost"))
	empty, err := segRepo.GetByMediaItemIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	require.NoError(t, segRepo.DeleteByMediaItemIDAndType(ctx, ep1.ID, "intro"))
	rows, err := segRepo.GetByMediaItemID(ctx, ep1.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "end_credits", rows[0].Type, "only the named type goes")
	rows, _ = segRepo.GetByMediaItemID(ctx, ep2.ID)
	assert.Len(t, rows, 1, "another item's intro is untouched")

	require.NoError(t, segRepo.DeleteByMediaItemID(ctx, ep1.ID))
	rows, err = segRepo.GetByMediaItemID(ctx, ep1.ID)
	require.NoError(t, err)
	assert.Empty(t, rows)
	rows, _ = segRepo.GetByMediaItemID(ctx, ep2.ID)
	assert.Len(t, rows, 1)
	require.NoError(t, segRepo.DeleteByMediaItemID(ctx, ep1.ID), "deleting nothing is a no-op")
}
