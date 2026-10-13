package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// seedFileItem creates a movie whose file has the given size, mtime and
// fingerprint, and returns it for a later replacement.
func seedFileItem(t *testing.T, itemRepo *postgres.MediaItemRepository, libID shared.LibraryID, id string, size int64, mod time.Time) *library.MediaItem {
	t.Helper()
	item := newTestMediaItem(id, libID)
	item.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Size: size, ModTime: mod, Fingerprint: fingerprintOf(size, mod)}
	require.NoError(t, itemRepo.Create(context.Background(), item))
	return item
}

// fingerprintOf is the scanner's fingerprint (filesystem.Scanner).
func fingerprintOf(size int64, mod time.Time) string {
	return fmt.Sprintf("%d-%d", size, mod.UnixNano())
}

// replaceFile is what the scan writes for a file replaced at the same path.
func replaceFile(t *testing.T, itemRepo *postgres.MediaItemRepository, item *library.MediaItem, size int64, mod time.Time) {
	t.Helper()
	item.File.Size, item.File.ModTime, item.File.Fingerprint = size, mod, fingerprintOf(size, mod)
	require.NoError(t, itemRepo.Update(context.Background(), item))
}

// A manifest made from a file since replaced is not served and the sweep
// lists the item again, the boot sweep included; a manifest from before
// stamping keeps being served.
func TestTrickplayRepo_ManifestOfAReplacedFileIsNeitherServedNorDone(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	tpRepo := postgres.NewTrickplayRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("tpsrc")
	require.NoError(t, libRepo.Create(ctx, lib))
	mod := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	item := seedFileItem(t, itemRepo, lib.ID, "tpsrc-m1", 100, mod)
	legacy := seedFileItem(t, itemRepo, lib.ID, "tpsrc-m2", 300, mod)

	require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{
		ID: "tpsrc-1", MediaItemID: item.ID, ImagePath: "/t/1", AlgoVersion: 1,
		FileSize: 100, ModTime: mod, CreatedAt: time.Now(),
	}))
	require.NoError(t, tpRepo.Create(ctx, &library.TrickplayManifest{
		ID: "tpsrc-2", MediaItemID: legacy.ID, ImagePath: "/t/2", AlgoVersion: 1, CreatedAt: time.Now(),
	}))
	got, err := tpRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(100), got.FileSize)
	assert.True(t, got.ModTime.Equal(mod))
	ids, err := tpRepo.ListMissingMediaItemIDs(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, ids)

	replaceFile(t, itemRepo, item, 200, mod.Add(time.Hour))

	_, err = tpRepo.GetByMediaItemID(ctx, item.ID)
	require.ErrorIs(t, err, shared.ErrNotFound, "the manifest of the replaced file is served")
	batch, err := tpRepo.GetByMediaItemIDs(ctx, []shared.MediaItemID{item.ID, legacy.ID})
	require.NoError(t, err)
	assert.NotContains(t, batch, item.ID)
	assert.Contains(t, batch, legacy.ID, "a manifest from before stamping is still served")
	ids, err = tpRepo.ListMissingMediaItemIDs(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{string(item.ID)}, ids, "the boot sweep remakes the replaced file's sprites")
}

// A segment a detector found in a replaced file is not served and does not
// settle the item; an operator mark stays. A detection that read the old file
// but writes after the replacement is stamped with the old file and hidden too.
func TestMediaSegmentRepo_DetectedSegmentOfAReplacedFileIsHidden(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("segsrc")
	require.NoError(t, libRepo.Create(ctx, lib))
	mod := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	item := seedFileItem(t, itemRepo, lib.ID, "segsrc-m1", 100, mod)
	oldFP := item.File.Fingerprint

	_, err := segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 1000, EndMs: 60000, Confidence: 0.9, Source: "phash"})
	require.NoError(t, err)
	require.NoError(t, segRepo.Upsert(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "end_credits", StartMs: 3000000, EndMs: 3100000, Confidence: 1, Source: "manual"}))
	segs, err := segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, segs, 2)
	for _, s := range segs {
		if s.Source == "phash" {
			assert.Equal(t, oldFP, s.SourceFingerprint, "a detected segment is stamped with the file it was found in")
		} else {
			assert.Empty(t, s.SourceFingerprint, "an operator mark is no file's")
		}
	}

	replaceFile(t, itemRepo, item, 200, mod.Add(time.Hour))

	segs, err = segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, "manual", segs[0].Source)
	batch, err := segRepo.GetByMediaItemIDs(ctx, []shared.MediaItemID{item.ID})
	require.NoError(t, err)
	assert.Len(t, batch[item.ID], 1)

	// Late write from a detection that read the old file.
	_, err = segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 1000, EndMs: 60000, Confidence: 0.9, Source: "phash", SourceFingerprint: oldFP})
	require.NoError(t, err)
	segs, err = segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Len(t, segs, 1, "a late detection of the old file is served")

	// A detection of the new file is served.
	_, err = segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: item.ID, Type: "intro", StartMs: 2000, EndMs: 62000, Confidence: 0.9, Source: "phash", SourceFingerprint: item.File.Fingerprint})
	require.NoError(t, err)
	segs, err = segRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Len(t, segs, 2)
}

// An episode settled by a chapter intro of its old file is back on the
// detection sweep's work list once the file is replaced.
func TestMediaSegmentRepo_ReplacedFileReopensIntroDetection(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	segRepo := postgres.NewMediaSegmentRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("segsweep")
	require.NoError(t, libRepo.Create(ctx, lib))
	season := newTestMediaItem("segsweep-season", lib.ID)
	season.Type = library.Season
	require.NoError(t, itemRepo.Create(ctx, season))
	mod := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	ep := newTestMediaItem("segsweep-ep", lib.ID)
	ep.Type, ep.ParentID = library.Episode, string(season.ID)
	ep.File = &library.MediaFile{Path: "/media/ep.mkv", Size: 100, ModTime: mod, Fingerprint: fingerprintOf(100, mod)}
	require.NoError(t, itemRepo.Create(ctx, ep))
	_, err := segRepo.UpsertUnlessManual(ctx, &library.MediaSegment{MediaItemID: ep.ID, Type: "intro", StartMs: 30_000, EndMs: 90_000, Confidence: 1, Source: "mkv_chapter"})
	require.NoError(t, err)

	got, err := segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 100, nil)
	require.NoError(t, err)
	assert.NotContains(t, got, string(season.ID), "premise: a chapter intro settles the episode")

	replaceFile(t, itemRepo, ep, 200, mod.Add(time.Hour))
	got, err = segRepo.ListSeasonsPendingIntroDetection(ctx, 5, 2, 100, nil)
	require.NoError(t, err)
	assert.Contains(t, got, string(season.ID))
}
