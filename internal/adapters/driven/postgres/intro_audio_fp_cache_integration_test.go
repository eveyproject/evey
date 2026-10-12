package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
)

// Run on the HOST (the container skips silently, no docker socket):
// CGO_ENABLED=0 go test ./internal/adapters/driven/postgres/ -run TestIntroAudioFPCache

// GetBatch must return every cached entry of the requested ids in one query,
// with fingerprints surviving the BYTEA round-trip, and simply omit ids that
// have no entry.
func TestIntroAudioFPCache_GetBatch(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	require.NoError(t, libRepo.Create(ctx, newTestLibrary("lib-fp")))
	for _, id := range []string{"fp-ep1", "fp-ep2", "fp-ep3"} {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, "lib-fp")))
	}

	cache := postgres.NewIntroAudioFingerprintCacheRepository(db)
	require.NoError(t, cache.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "fp-ep1", FileFingerprint: "file-1", WindowSec: 720,
		Fingerprint: []uint32{0x1A2B3C4D, 42}, AlgoVersion: 1,
	}))
	require.NoError(t, cache.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "fp-ep2", FileFingerprint: "file-2", WindowSec: 720,
		Fingerprint: []uint32{7, 8, 9}, AlgoVersion: 1,
	}))

	got, err := cache.GetBatch(ctx, []string{"fp-ep1", "fp-ep2", "fp-ep3"})
	require.NoError(t, err)
	require.Len(t, got, 2, "fp-ep3 has no entry and must simply be absent")
	require.Contains(t, got, "fp-ep1")
	require.Contains(t, got, "fp-ep2")
	assert.Equal(t, []uint32{0x1A2B3C4D, 42}, got["fp-ep1"].Fingerprint)
	assert.Equal(t, "file-1", got["fp-ep1"].FileFingerprint)
	assert.Equal(t, 720, got["fp-ep1"].WindowSec)
	assert.Equal(t, []uint32{7, 8, 9}, got["fp-ep2"].Fingerprint)

	empty, err := cache.GetBatch(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	// A batched read must agree with the single-entry read path.
	single, err := cache.Get(ctx, "fp-ep1")
	require.NoError(t, err)
	assert.Equal(t, single.Fingerprint, got["fp-ep1"].Fingerprint)
}
