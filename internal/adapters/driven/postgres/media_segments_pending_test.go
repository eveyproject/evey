package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// newTestEpisode builds an episode row under a season parent. media_items has a
// self FK on parent_id, so the season row must exist first - the in-memory fakes
// do not model that, the database does.
func newTestEpisode(t *testing.T, itemRepo *postgres.MediaItemRepository, ctx context.Context,
	id string, libraryID shared.LibraryID, parentID string,
) *library.MediaItem {
	t.Helper()
	season := newTestMediaItem(parentID, libraryID)
	season.Type = library.Season
	require.NoError(t, itemRepo.Create(ctx, season))
	it := newTestMediaItem(id, libraryID)
	it.Type = library.Episode
	it.ParentID = parentID
	require.NoError(t, itemRepo.Create(ctx, it))
	return it
}

// The boot sweep's work list must express "seasons whose INTRO detection is
// still to do", not "seasons with no media_segments row at all". 99% of the
// prod catalogue already carries an end_credits row, which made the old
// `ms.id IS NULL` predicate find almost nothing (9 seasons, done in 28s) and
// left the whole detection effort depending on a manual admin sweep.
func TestMediaSegmentRepo_ListSeasonsPendingIntroDetection(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("pendinglib")
	require.NoError(t, libRepo.Create(ctx, lib))

	// A season whose episode carries ONLY an end_credits row: its intro has
	// never been detected, yet the old predicate saw a segment and skipped it.
	creditsOnly := newTestEpisode(t, itemRepo, ctx, "pending-credits-only", lib.ID, "season-credits-only")
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{
		MediaItemID: creditsOnly.ID, Type: "end_credits",
		StartMs: 2_400_000, EndMs: 2_520_000, Source: "blackdetect", Confidence: 0.6,
	}))

	// A season with a real, file-derived intro: settled, must NOT come back.
	covered := newTestEpisode(t, itemRepo, ctx, "pending-covered", lib.ID, "season-covered")
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{
		MediaItemID: covered.ID, Type: "intro",
		StartMs: 30_000, EndMs: 90_000, Source: "audio_cluster", Confidence: 0.9,
	}))

	// A season whose intro comes from a community DB: NOT settled - those
	// timestamps describe someone else's copy of the file.
	tidb := newTestEpisode(t, itemRepo, ctx, "pending-tidb", lib.ID, "season-tidb")
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{
		MediaItemID: tidb.ID, Type: "intro",
		StartMs: 30_000, EndMs: 90_000, Source: "tidb", Confidence: 0.8,
	}))

	// A season whose "intro" is a degenerate 2s stub: not coverage.
	stub := newTestEpisode(t, itemRepo, ctx, "pending-stub", lib.ID, "season-stub")
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{
		MediaItemID: stub.ID, Type: "intro",
		StartMs: 0, EndMs: 2_000, Source: "phash", Confidence: 0.7,
	}))

	// A season locked by the operator: settled whatever the span.
	manual := newTestEpisode(t, itemRepo, ctx, "pending-manual", lib.ID, "season-manual")
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{
		MediaItemID: manual.ID, Type: "intro",
		StartMs: 0, EndMs: 1_000, Source: "manual", Confidence: 1.0,
	}))

	// A season whose only episode burned its retry budget at the CURRENT algo
	// version: parked, the cursor must not hand it back forever.
	exhausted := newTestEpisode(t, itemRepo, ctx, "pending-exhausted", lib.ID, "season-exhausted")
	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, string(exhausted.ID), 5))
	require.NoError(t, segRepo.RecordIntroDetectionAttempt(ctx, string(exhausted.ID), 5))

	// A season with nothing at all: obviously pending.
	newTestEpisode(t, itemRepo, ctx, "pending-bare", lib.ID, "season-bare")

	got, err := segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 100, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"season-credits-only", "season-tidb", "season-stub", "season-bare"},
		got,
		"the work list is 'intro still to do', not 'no segment at all'")
	// One episode per season here: the per-episode count is the same list.
	cov, err := segRepo.IntroCoverage(ctx, 5, 2)
	require.NoError(t, err)
	assertCoveragePending(t, cov, len(got))
	assert.Equal(t, int64(7), cov.TotalItems)

	// A bump of the algo version re-enables the parked season, exactly like
	// IntroDetectionExhaustedBatch: one supported way to force a re-detection.
	got, err = segRepo.ListSeasonsPendingIntroDetection(ctx, 6, 2, 100, nil)
	require.NoError(t, err)
	assert.Contains(t, got, "season-exhausted", "an algo bump re-enables a parked season")
	cov, err = segRepo.IntroCoverage(ctx, 6, 2)
	require.NoError(t, err)
	assertCoveragePending(t, cov, len(got))

	// exclude is the in-run skip set: a season already swept this cycle must
	// not be handed back by the next batch, or the drain loop spins on it.
	got, err = segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 100,
		[]string{"season-credits-only", "season-tidb", "season-stub"})
	require.NoError(t, err)
	assert.Equal(t, []string{"season-bare"}, got)

	// limit bounds the batch: this list is NOT paginated through List, so
	// nothing else clamps it.
	got, err = segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 2, nil)
	require.NoError(t, err)
	assert.Len(t, got, 2, "the batch must honour its own limit")
}
