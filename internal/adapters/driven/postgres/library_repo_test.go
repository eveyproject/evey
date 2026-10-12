package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLibrary(id string) *library.Library {
	return &library.Library{
		ID:                  shared.LibraryID(id),
		Name:                "Library " + id,
		Type:                library.Movie,
		Paths:               []string{"/media/" + id},
		ScanIntervalMinutes: 360,
		Enabled:             true,
		CreatedAt:           time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt:           time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestLibraryRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lib1")
	require.NoError(t, repo.Create(ctx, lib))

	got, err := repo.GetByID(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, lib.Name, got.Name)
	assert.Equal(t, lib.Paths, got.Paths)
	assert.Equal(t, lib.Type, got.Type)
}

func TestLibraryRepo_ScanFlags_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("flags1")
	lib.ScanOnStartup = true
	lib.RealtimeMonitoring = true
	require.NoError(t, repo.Create(ctx, lib))

	// Create → GetByID must preserve both flags (the bug was they were never persisted).
	got, err := repo.GetByID(ctx, lib.ID)
	require.NoError(t, err)
	assert.True(t, got.ScanOnStartup, "scan_on_startup must persist")
	assert.True(t, got.RealtimeMonitoring, "realtime_monitoring must persist")

	// List path scans the same columns.
	list, err := repo.List(ctx, library.ListLibrariesFilter{})
	require.NoError(t, err)
	var seen *library.Library
	for _, l := range list {
		if l.ID == lib.ID {
			seen = l
		}
	}
	require.NotNil(t, seen)
	assert.True(t, seen.ScanOnStartup)
	assert.True(t, seen.RealtimeMonitoring)

	// Update must persist a flip back to false.
	got.ScanOnStartup = false
	got.RealtimeMonitoring = false
	require.NoError(t, repo.Update(ctx, got))
	after, err := repo.GetByID(ctx, lib.ID)
	require.NoError(t, err)
	assert.False(t, after.ScanOnStartup)
	assert.False(t, after.RealtimeMonitoring)
}

func TestLibraryRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestLibrary("lib2")))
	require.NoError(t, repo.Create(ctx, newTestLibrary("lib3")))

	list, err := repo.List(ctx, library.ListLibrariesFilter{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestLibraryRepo_List_Pagination(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	// seed 5 libs with deterministic names for ORDER BY name
	for _, id := range []string{"page-a", "page-b", "page-c", "page-d", "page-e"} {
		require.NoError(t, repo.Create(ctx, newTestLibrary(id)))
	}

	// limit=2, offset=0
	page1, err := repo.List(ctx, library.ListLibrariesFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	// limit=2, offset=2
	page2, err := repo.List(ctx, library.ListLibrariesFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// pages are disjoint
	seen := map[shared.LibraryID]bool{}
	for _, l := range page1 {
		seen[l.ID] = true
	}
	for _, l := range page2 {
		assert.False(t, seen[l.ID], "page2 should not contain page1 ids")
	}

	// limit=0 → all (no LIMIT clause)
	all, err := repo.List(ctx, library.ListLibrariesFilter{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(all), 5)
}

func TestLibraryRepo_List_LimitClampedToMax(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestLibrary("clamp-1")))

	// requesting beyond max is accepted silently (clamped); no error
	list, err := repo.List(ctx, library.ListLibrariesFilter{Limit: 10_000})
	require.NoError(t, err)
	assert.NotEmpty(t, list)
}

func TestLibraryRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lib4")
	require.NoError(t, repo.Create(ctx, lib))

	lib.Name = "Updated Library"
	lib.Enabled = false
	lib.Paths = []string{"/media/new"}
	lib.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, lib))

	got, err := repo.GetByID(ctx, shared.LibraryID(lib.ID))
	require.NoError(t, err)
	assert.Equal(t, "Updated Library", got.Name)
	assert.False(t, got.Enabled)
	assert.Equal(t, []string{"/media/new"}, got.Paths)
}

func TestLibraryRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lib5")
	require.NoError(t, repo.Create(ctx, lib))
	require.NoError(t, repo.Delete(ctx, shared.LibraryID(lib.ID)))

	_, err := repo.GetByID(ctx, shared.LibraryID(lib.ID))
	assert.Error(t, err)
}

func TestLibraryRepo_Update_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("ghost")
	err := repo.Update(ctx, lib)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestLibraryRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	err := repo.Delete(ctx, shared.LibraryID("nonexistent"))
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

// Same completion for libraries: scan_on_startup and realtime_monitoring are
// two adjacent booleans, and enabled is a third a few slots earlier. Swap any
// pair and a library silently stops watching for new files, or starts a full
// scan on every boot - both invisible until someone notices the behaviour
// changed. The existing Update test asserts name/enabled/paths only.
func TestLibraryRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()
	lib := newTestLibrary("lib-rt")
	require.NoError(t, repo.Create(ctx, lib))

	scanned := time.Now().UTC().Truncate(time.Millisecond).Add(-2 * time.Hour)
	lib.Name = "Round Trip Library"
	lib.Type = library.TVShow
	lib.Paths = []string{"/media/a", "/media/b"}
	lib.ScanIntervalMinutes = 45
	lib.Enabled = false
	// The two adjacent booleans, set to DIFFERENT values on purpose.
	lib.ScanOnStartup = true
	lib.RealtimeMonitoring = false
	lib.LastScannedAt = &scanned
	lib.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, lib))

	got, err := repo.GetByID(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Round Trip Library", got.Name)
	assert.Equal(t, library.TVShow, got.Type)
	assert.Equal(t, []string{"/media/a", "/media/b"}, got.Paths)
	assert.Equal(t, 45, got.ScanIntervalMinutes)
	assert.False(t, got.Enabled)
	assert.True(t, got.ScanOnStartup, "scan_on_startup must not pick up realtime_monitoring's value")
	assert.False(t, got.RealtimeMonitoring)
	require.NotNil(t, got.LastScannedAt)
	assert.Equal(t, scanned, got.LastScannedAt.UTC())
}
