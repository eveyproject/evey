package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrickplayRepo_Create_GetByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tpmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC().Truncate(time.Millisecond)
	m := &library.TrickplayManifest{
		ID:          "trick1",
		MediaItemID: item.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi1",
		CreatedAt:   now,
	}
	require.NoError(t, tpRepo.Create(ctx, m))

	got, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)
	assert.Equal(t, m.MediaItemID, got.MediaItemID)
	assert.Equal(t, m.Interval, got.Interval)
	assert.Equal(t, m.Width, got.Width)
	assert.Equal(t, m.Height, got.Height)
	assert.Equal(t, m.TileWidth, got.TileWidth)
	assert.Equal(t, m.TileHeight, got.TileHeight)
	assert.Equal(t, m.ImageCount, got.ImageCount)
	assert.Equal(t, m.ImagePath, got.ImagePath)
	assert.Equal(t, m.CreatedAt, got.CreatedAt)
}

func TestTrickplayRepo_Create_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tpmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	first := &library.TrickplayManifest{
		ID:          "trick2a",
		MediaItemID: item.ID,
		Interval:    5,
		Width:       160,
		Height:      90,
		TileWidth:   4,
		TileHeight:  4,
		ImageCount:  16,
		ImagePath:   "/trickplay/tpmi2/v1",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, tpRepo.Create(ctx, first))

	updated := &library.TrickplayManifest{
		ID:          "trick2b",
		MediaItemID: item.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi2/v2",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, tpRepo.Create(ctx, updated))

	got, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, updated.Interval, got.Interval)
	assert.Equal(t, updated.Width, got.Width)
	assert.Equal(t, updated.ImageCount, got.ImageCount)
	assert.Equal(t, updated.ImagePath, got.ImagePath)
}

func TestTrickplayRepo_GetByMediaItemID_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	_, err := tpRepo.GetByMediaItemID(ctx, "nonexistent")
	assert.Equal(t, shared.ErrNotFound, err)
}

// TestTrickplayRepo_GetByMediaItemIDs_BatchesAndSkipsMissing proves the batch
// read returns every requested id that has a manifest, keyed correctly, and
// simply omits ids with none (rather than an error or a nil-pointer entry) -
// the semantics intro propagation's sibling sweep relies on.
func TestTrickplayRepo_GetByMediaItemIDs_BatchesAndSkipsMissing(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	itemWith := newTestMediaItem("tpmi4a", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemWith))
	itemMissing := newTestMediaItem("tpmi4b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemMissing))

	now := time.Now().UTC().Truncate(time.Millisecond)
	m := &library.TrickplayManifest{
		ID:          "trick4",
		MediaItemID: itemWith.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi4a",
		CreatedAt:   now,
	}
	require.NoError(t, tpRepo.Create(ctx, m))

	got, err := tpRepo.GetByMediaItemIDs(ctx, []shared.MediaItemID{itemWith.ID, itemMissing.ID, "nonexistent"})
	require.NoError(t, err)
	require.Len(t, got, 1, "only the id with a manifest must appear in the map")
	require.Contains(t, got, itemWith.ID)
	assert.Equal(t, m.ID, got[itemWith.ID].ID)
	assert.Equal(t, m.Interval, got[itemWith.ID].Interval)
	assert.Equal(t, m.ImagePath, got[itemWith.ID].ImagePath)
	assert.NotContains(t, got, itemMissing.ID)
}

func TestTrickplayRepo_GetByMediaItemIDs_EmptyInput(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	got, err := tpRepo.GetByMediaItemIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// seedTrickplayMovie inserts a movie with a file carrying the exact size and
// mod time a failure record is matched against.
func seedTrickplayMovie(t *testing.T, itemRepo *postgres.MediaItemRepository, libID shared.LibraryID, id string, size int64, mod time.Time) {
	t.Helper()
	item := newTestMediaItem(id, libID)
	item.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Size: size, ModTime: mod}
	require.NoError(t, itemRepo.Create(context.Background(), item))
}

func TestTrickplayRepo_ListMissing_SkipsRecordedFailures(t *testing.T) {
	// A recorded failure suppresses the item ONLY while the media_files row
	// still matches the recorded (size, mod time) pair: the failure was a
	// verdict on those exact bytes. A replaced file no longer matches and the
	// item re-enters the candidates; before this existed the same failing
	// files were re-attempted at every single boot (prod 2026-07-26).
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	mod := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	lib := newTestLibrary("tplib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	seedTrickplayMovie(t, itemRepo, lib.ID, "tpf-m1", 100, mod)
	seedTrickplayMovie(t, itemRepo, lib.ID, "tpf-m2", 200, mod)

	ids, err := tpRepo.ListMissingMediaItemIDs(ctx, 1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"tpf-m1", "tpf-m2"}, ids)

	require.NoError(t, tpRepo.RecordFailure(ctx, "tpf-m1", 100, mod, "ffprobe: exit status 1"))
	ids, err = tpRepo.ListMissingMediaItemIDs(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"tpf-m2"}, ids, "a failure matching the current file suppresses the item")
	cov, err := tpRepo.TrickplayCoverage(ctx, 1)
	require.NoError(t, err)
	assertCoveragePending(t, cov, 1)
	assert.Equal(t, int64(2), cov.TotalItems, "a settled failure counts as done, not as gone")

	// A STALE failure (recorded against other bytes) must not suppress.
	require.NoError(t, tpRepo.RecordFailure(ctx, "tpf-m2", 999, mod, "old failure"))
	ids, err = tpRepo.ListMissingMediaItemIDs(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"tpf-m2"}, ids, "a failure recorded against a different size no longer matches")
}

func TestTrickplayRepo_CreateClearsRecordedFailure(t *testing.T) {
	// A successful generation settles the question the failure recorded: once
	// the manifest is later deleted (regeneration flows), the item must come
	// back as a candidate instead of staying suppressed by a stale failure.
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	mod := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	lib := newTestLibrary("tplib6")
	require.NoError(t, libRepo.Create(ctx, lib))
	seedTrickplayMovie(t, itemRepo, lib.ID, "tpf-m3", 100, mod)

	require.NoError(t, tpRepo.RecordFailure(ctx, "tpf-m3", 100, mod, "ffprobe: exit status 1"))
	require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{
		ID: "tpf-man-1", MediaItemID: "tpf-m3", Interval: 7, Width: 320,
		TileWidth: 5, TileHeight: 5, ImageCount: 1, ImagePath: "/cache/tpf-m3",
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))
	require.NoError(t, tpRepo.Delete(ctx, "tpf-m3"))

	ids, err := tpRepo.ListMissingMediaItemIDs(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"tpf-m3"}, ids, "Create cleared the failure; without a manifest the item is a candidate again")
}

func TestTrickplayRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tplib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("tpmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	m := &library.TrickplayManifest{
		ID:          "trick3",
		MediaItemID: item.ID,
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   5,
		TileHeight:  5,
		ImageCount:  25,
		ImagePath:   "/trickplay/tpmi3",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, tpRepo.Create(ctx, m))

	require.NoError(t, tpRepo.Delete(ctx, item.ID))

	_, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	assert.Equal(t, shared.ErrNotFound, err)
}

// A manifest stamped below the current algo version is re-listed by the
// missing-sweep (its generation parameters changed); regenerating at the
// current version removes it. Legacy rows carry version 0 via the migration
// default, so a params change finally reaches pre-existing manifests.
func TestTrickplayRepo_ListMissing_ReturnsOutdatedManifests(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	mod := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	lib := newTestLibrary("tplib7")
	require.NoError(t, libRepo.Create(ctx, lib))
	seedTrickplayMovie(t, itemRepo, lib.ID, "tpv-m1", 100, mod)

	// Legacy manifest: algo_version 0 (the migration default).
	require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{
		ID: "tpv-man-0", MediaItemID: "tpv-m1", Interval: 10, Width: 320,
		TileWidth: 10, TileHeight: 10, ImageCount: 1, ImagePath: "/cache/tpv-m1",
		AlgoVersion: 0, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))

	ids, err := tpRepo.ListMissingMediaItemIDs(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"tpv-m1"}, ids, "an algo_version-0 manifest is outdated at current version 1")

	// Regenerated at the current version: no longer listed.
	require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{
		ID: "tpv-man-1", MediaItemID: "tpv-m1", Interval: 7, Width: 320,
		TileWidth: 5, TileHeight: 5, ImageCount: 1, ImagePath: "/cache/tpv-m1",
		AlgoVersion: 1, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))
	ids, err = tpRepo.ListMissingMediaItemIDs(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, ids)

	// Round-trip: the stored version comes back on the read paths.
	m, err := tpRepo.GetByMediaItemID(ctx, "tpv-m1")
	require.NoError(t, err)
	assert.Equal(t, 1, m.AlgoVersion)
}
