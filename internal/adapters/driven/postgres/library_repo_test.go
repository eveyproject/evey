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

	got, err := repo.GetByID(ctx, shared.LibraryID(lib.ID))
	require.NoError(t, err)
	assert.Equal(t, lib.Name, got.Name)
	assert.Equal(t, lib.Paths, got.Paths)
	assert.Equal(t, lib.Type, got.Type)
}

func TestLibraryRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLibraryRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestLibrary("lib2")))
	require.NoError(t, repo.Create(ctx, newTestLibrary("lib3")))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
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
