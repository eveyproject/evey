package postgres_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// The pHash cache packs every hash and timestamp as 8 little-endian bytes:
// the round trip must be exact at the extremes (all-ones hashes, negative
// timestamps), a mismatched pair is refused before it hits the table, an
// algorithm bump sweeps the stale rows, and a deleted item takes its row.
func TestIntroFingerprintCacheRepo_RoundTripAndInvalidation(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("fpclib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	ep1, ep2 := newTestMediaItem("fpc1", lib.ID), newTestMediaItem("fpc2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, ep1))
	require.NoError(t, itemRepo.Create(ctx, ep2))
	repo := postgres.NewIntroFingerprintCacheRepository(db)

	_, err := repo.Get(ctx, string(ep1.ID))
	require.ErrorIs(t, err, shared.ErrNotFound)

	entry := &library.IntroFingerprintCacheEntry{
		MediaItemID: string(ep1.ID), FileFingerprint: "fp-a", AlgoVersion: 3,
		PhashTiles:      []uint64{0, 1, math.MaxUint64, 0x0123456789abcdef},
		PhashTimestamps: []int64{0, 1000, -1, math.MaxInt64},
	}
	require.NoError(t, repo.Upsert(ctx, entry))
	got, err := repo.Get(ctx, string(ep1.ID))
	require.NoError(t, err)
	assert.Equal(t, entry.PhashTiles, got.PhashTiles)
	assert.Equal(t, entry.PhashTimestamps, got.PhashTimestamps)
	assert.Equal(t, "fp-a", got.FileFingerprint)
	assert.EqualValues(t, 3, got.AlgoVersion)
	assert.False(t, got.ExtractedAt.IsZero())

	require.Error(t, repo.Upsert(ctx, &library.IntroFingerprintCacheEntry{
		MediaItemID: string(ep1.ID), FileFingerprint: "fp-b", AlgoVersion: 3,
		PhashTiles: []uint64{1, 2}, PhashTimestamps: []int64{1},
	}), "tiles and timestamps must pair up")
	got, _ = repo.Get(ctx, string(ep1.ID))
	assert.Equal(t, "fp-a", got.FileFingerprint, "a refused upsert leaves the row alone")

	// Re-extraction overwrites in place; an empty fingerprint is a valid row.
	require.NoError(t, repo.Upsert(ctx, &library.IntroFingerprintCacheEntry{MediaItemID: string(ep1.ID), FileFingerprint: "fp-c", AlgoVersion: 4}))
	got, err = repo.Get(ctx, string(ep1.ID))
	require.NoError(t, err)
	assert.Equal(t, "fp-c", got.FileFingerprint)
	assert.Empty(t, got.PhashTiles)

	require.NoError(t, repo.Upsert(ctx, &library.IntroFingerprintCacheEntry{MediaItemID: string(ep2.ID), FileFingerprint: "fp-old", AlgoVersion: 2, PhashTiles: []uint64{9}, PhashTimestamps: []int64{9}}))
	n, err := repo.DeleteByAlgoVersionBelow(ctx, 4)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only the row from the older algorithm goes")
	_, err = repo.Get(ctx, string(ep2.ID))
	require.ErrorIs(t, err, shared.ErrNotFound)
	_, err = repo.Get(ctx, string(ep1.ID))
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, string(ep1.ID)))
	_, err = repo.Get(ctx, string(ep1.ID))
	require.ErrorIs(t, err, shared.ErrNotFound)
	require.NoError(t, repo.Delete(ctx, string(ep1.ID)), "deleting a missing row is a no-op")

	require.NoError(t, repo.Upsert(ctx, &library.IntroFingerprintCacheEntry{MediaItemID: string(ep2.ID), FileFingerprint: "fp", AlgoVersion: 4}))
	require.NoError(t, itemRepo.Delete(ctx, ep2.ID))
	_, err = repo.Get(ctx, string(ep2.ID))
	require.ErrorIs(t, err, shared.ErrNotFound, "the cache row follows its media item (FK cascade)")
}
