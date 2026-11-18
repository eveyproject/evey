package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Run on the HOST (needs a docker socket):
// CGO_ENABLED=0 go test ./cmd/evey/ -run TestFingerprintHousekeeping

// The rows still in the database move to disk with their content and
// extraction time, leave the database, and never overwrite a file already
// there; files of media items that are gone are swept.
func TestFingerprintHousekeeping_MoveThenPrune(t *testing.T) {
	testhelpers.SkipIfNoDocker(t)
	if testing.Short() {
		t.Skip("needs a real Postgres")
	}
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	lib := &library.Library{ID: "lib-fp", Name: "Lib", Type: library.Movie, Paths: []string{"/media/fp"}, Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	items := postgres.NewMediaItemRepository(db)
	for _, id := range []string{"ep-1", "ep-2", "ep-3"} {
		require.NoError(t, items.Create(ctx, &library.MediaItem{
			ID: shared.MediaItemID(id), LibraryID: lib.ID, Type: library.MovieItem, Title: id, SortTitle: id,
			Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{}, Genres: []string{}, Studios: []string{}, Tags: []string{}},
		}))
	}

	r := postgres.NewRepos(db)
	require.NoError(t, r.IntroAudioFingerprintCache.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep-1", FileFingerprint: "f1", WindowSec: 720, Fingerprint: []uint32{1, 2, 3}, AlgoVersion: 1,
	}))
	require.NoError(t, r.IntroAudioFingerprintCache.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep-2", FileFingerprint: "stale-row", WindowSec: 720, Fingerprint: []uint32{9}, AlgoVersion: 1,
	}))
	require.NoError(t, r.IntroFingerprintCache.Upsert(ctx, &library.IntroFingerprintCacheEntry{
		MediaItemID: "ep-3", FileFingerprint: "f3", PhashTiles: []uint64{11, 12}, PhashTimestamps: []int64{100, 200}, AlgoVersion: 2,
	}))
	rowAt, err := r.IntroAudioFingerprintCache.Get(ctx, "ep-1")
	require.NoError(t, err)

	root := t.TempDir()
	audio := filesystem.NewAudioFingerprintStore(root)
	phash := filesystem.NewPhashFingerprintStore(root)
	// ep-2 already has a newer file: the row must not replace it.
	require.NoError(t, audio.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep-2", FileFingerprint: "newer-file", WindowSec: 720, Fingerprint: []uint32{5}, AlgoVersion: 1,
	}))

	moveAudioFingerprints(ctx, r.IntroAudioFingerprintCache, audio)
	movePhashFingerprints(ctx, r.IntroFingerprintCache, phash)

	got, err := audio.Get(ctx, "ep-1")
	require.NoError(t, err)
	assert.Equal(t, []uint32{1, 2, 3}, got.Fingerprint)
	assert.Equal(t, "f1", got.FileFingerprint)
	assert.True(t, got.ExtractedAt.Equal(rowAt.ExtractedAt.Truncate(time.Millisecond)), "the original extraction time moves with the entry")
	kept, err := audio.Get(ctx, "ep-2")
	require.NoError(t, err)
	assert.Equal(t, "newer-file", kept.FileFingerprint)
	ph, err := phash.Get(ctx, "ep-3")
	require.NoError(t, err)
	assert.Equal(t, []uint64{11, 12}, ph.PhashTiles)
	assert.Equal(t, []int64{100, 200}, ph.PhashTimestamps)

	for _, table := range []string{"intro_audio_fingerprint_cache", "intro_fingerprint_cache"} {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		assert.Zero(t, n, "%s is emptied once its rows moved", table)
	}

	// A media item deleted afterwards leaves an orphan file; the sweep takes it
	// and keeps the others.
	_, err = db.ExecContext(ctx, `DELETE FROM media_items WHERE id = 'ep-1'`)
	require.NoError(t, err)
	pruneFingerprintOrphans(ctx, db, "audio", audio.MediaItemIDs, audio.Delete)
	pruneFingerprintOrphans(ctx, db, "phash", phash.MediaItemIDs, phash.Delete)
	assert.False(t, audio.Has("ep-1"))
	assert.True(t, audio.Has("ep-2"))
	assert.True(t, phash.Has("ep-3"))
}

// A failed media lookup must not be read as "every media item is gone".
func TestFingerprintHousekeeping_PruneKeepsFilesWhenDatabaseFails(t *testing.T) {
	testhelpers.SkipIfNoDocker(t)
	if testing.Short() {
		t.Skip("needs a real Postgres")
	}
	db, cleanup := testhelpers.StartPostgres(t)
	ctx := context.Background()
	audio := filesystem.NewAudioFingerprintStore(t.TempDir())
	require.NoError(t, audio.Upsert(ctx, &library.IntroAudioFingerprintCacheEntry{
		MediaItemID: "ep-1", WindowSec: 720, Fingerprint: []uint32{1}, AlgoVersion: 1,
	}))
	cleanup()
	pruneFingerprintOrphans(ctx, db, "audio", audio.MediaItemIDs, audio.Delete)
	assert.True(t, audio.Has("ep-1"))
}
