package postgres_test

// Two files that happen to share a change fingerprint must not deadlock a
// library.
//
// Prod, 2026-09-01: twelve times in one day, "MediaItemRepo.Update: upsert
// media_files: duplicate key value violates unique constraint
// idx_media_files_fingerprint", always on the same item. The fingerprint is
// size + mtime, which two distinct files genuinely can share - a copy made with
// cp -p, two files restored from the same backup - and the unique index turned
// that coincidence into an item that failed to update on every single scan,
// for ever.
//
// The fingerprint is a hint about whether a file changed. It is not an
// identity, and it must never be able to refuse a write.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
)

func withFile(item *library.MediaItem, path, fingerprint string) *library.MediaItem {
	item.File = &library.MediaFile{
		Path:        path,
		Size:        4096,
		Fingerprint: fingerprint,
		ModTime:     time.Now().UTC().Truncate(time.Second),
	}
	return item
}

func TestMediaItemRepo_ACollidingFingerprintDoesNotBlockTheWrite(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("fplib")
	require.NoError(t, libRepo.Create(ctx, lib))

	const shared_ = "4096-1700000000000000000"
	first := withFile(newTestMediaItem("fp-first", lib.ID), "/media/films/a.mkv", shared_)
	require.NoError(t, itemRepo.Create(ctx, first))

	// The same size and mtime on a different file. Before the fix this write
	// failed, and kept failing at every scan.
	second := withFile(newTestMediaItem("fp-second", lib.ID), "/media/films/b.mkv", shared_)
	require.NoError(t, itemRepo.Create(ctx, second),
		"a coincidence between two files must never refuse a write")

	got, err := itemRepo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	require.NotNil(t, got.File)
	assert.Equal(t, "/media/films/b.mkv", got.File.Path)
	assert.Equal(t, shared_, got.File.Fingerprint, "the file just scanned keeps its fingerprint")

	// The older row loses only its change-detection hint: it is re-probed at the
	// next scan, which is the honest outcome of a genuine collision. Its path
	// and identity are untouched.
	older, err := itemRepo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	require.NotNil(t, older.File)
	assert.Equal(t, "/media/films/a.mkv", older.File.Path)
	assert.Empty(t, older.File.Fingerprint, "the loser gives up the hint, not the row")
}

// Re-writing the SAME item with the same fingerprint is the common path and
// must not clear anything.
func TestMediaItemRepo_ReWritingTheSameItemKeepsItsFingerprint(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("fplib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	const fp = "4096-1700000000000000001"
	item := withFile(newTestMediaItem("fp-same", lib.ID), "/media/films/c.mkv", fp)
	require.NoError(t, itemRepo.Create(ctx, item))

	item.Title = "Renamed"
	require.NoError(t, itemRepo.Update(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, got.File)
	assert.Equal(t, fp, got.File.Fingerprint)
}
