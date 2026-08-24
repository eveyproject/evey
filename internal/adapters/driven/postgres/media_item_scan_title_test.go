package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scan_title is the filesystem-derived title: the ONE value that survives a
// wrong provider match and lets a mis-enriched row be repaired without going
// back to the disk. Update persists whatever the caller passes, and used to
// substitute item.Title when the caller left ScanTitle empty - so any caller
// holding a partially-built item silently overwrote the stable title with the
// (possibly wrong) provider one. Measured consequence on 2026-06-03: rows whose
// scan_title had become "Johnny" instead of the parsed "Johnny English", after
// a bad match. An empty ScanTitle must MEAN "I am not changing it", never
// "overwrite it with the display title".
func TestMediaItemRepo_Update_EmptyScanTitleKeepsTheStoredOne(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("scanlib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi-scan-1", lib.ID)
	item.Title = "Some Movie Two"
	item.ScanTitle = "Some Movie Two"
	require.NoError(t, itemRepo.Create(ctx, item))

	// A caller that read the row through a projection without scan_title, then
	// wrote back a provider title. This is the shape that destroyed the stable
	// value.
	partial := newTestMediaItem("mi-scan-1", lib.ID)
	partial.Title = "Some Other Work"
	partial.ScanTitle = ""
	require.NoError(t, itemRepo.Update(ctx, partial))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Some Other Work", got.Title, "the display title is the caller's to change")
	assert.Equal(t, "Some Movie Two", got.ScanTitle,
		"the filesystem-derived title must survive: it is the only repair source that does not need the disk")
}

// The heal pass DOES rewrite scan_title on purpose (the parser improved). A
// non-empty value from the caller must still win, or healOrphanTitles becomes
// a no-op and ResearchMetadata keeps resetting Title from a stale value.
func TestMediaItemRepo_Update_NonEmptyScanTitleStillWins(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("scanlib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi-scan-2", lib.ID)
	item.ScanTitle = "Some Movie Dirty"
	require.NoError(t, itemRepo.Create(ctx, item))

	healed := newTestMediaItem("mi-scan-2", lib.ID)
	healed.Title = "Some Movie Clean"
	healed.ScanTitle = "Some Movie Clean"
	require.NoError(t, itemRepo.Update(ctx, healed))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Some Movie Clean", got.ScanTitle)
}

// The rest of the media_items UPDATE, alongside the scan_title rule above.
// overview, tagline and index_number were never read back after an Update:
// title/sort_title/original_title/overview/tagline sit in five consecutive
// positional slots, all text, so a shift anywhere in that run swaps a tagline
// into an overview on every scanned item and nothing complains.
func TestMediaItemRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib-rt")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("mi-col-rt", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	idx := 7
	item.Title = "Display Title"
	item.ScanTitle = "Scan Title"
	item.SortTitle = "sort title"
	item.OriginalTitle = "Original Title"
	item.Overview = "An overview sentence."
	item.Tagline = "A tagline."
	item.IndexNumber = &idx
	require.NoError(t, itemRepo.Update(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Display Title", got.Title)
	assert.Equal(t, "Scan Title", got.ScanTitle)
	assert.Equal(t, "sort title", got.SortTitle)
	assert.Equal(t, "Original Title", got.OriginalTitle)
	assert.Equal(t, "An overview sentence.", got.Overview)
	assert.Equal(t, "A tagline.", got.Tagline)
	require.NotNil(t, got.IndexNumber)
	assert.Equal(t, 7, *got.IndexNumber)
}
