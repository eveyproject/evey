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

// A device's refusal belongs to the bytes it was handed: a scan rewriting
// the same file keeps it, a replaced or repaired file (other size, other
// fingerprint) is handed over whole again.
func TestMediaItemRepo_DirectPlayRefused_FollowsTheBytes(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewMediaItemRepository(db)
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, newTestLibrary("dplib")))
	item := fullItem("dp1", "dplib", "fp-dp1")
	require.NoError(t, repo.Create(ctx, item))

	require.NoError(t, repo.MarkDirectPlayRefused(ctx, item.ID))
	got, err := repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, got.File.DirectPlayRefused)
	assert.True(t, got.File.RefusesDirectPlay())

	// A scan holding an older read of the row writes it back: the refusal
	// stays, the struct never carries it.
	item.Title = "Rescanned"
	require.NoError(t, repo.Update(ctx, item))
	got, err = repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, got.File.DirectPlayRefused, "same bytes, same refusal")

	// The file was replaced.
	item.File.Size++
	item.File.Fingerprint = "fp-dp1-new"
	require.NoError(t, repo.Update(ctx, item))
	got, err = repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.False(t, got.File.DirectPlayRefused, "other bytes get a fresh chance")
}

// The media-health scan's diagnosis reaches the row by path, with the check
// version, and a clean one clears it.
func TestMediaItemRepo_SetContainerDamage(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewMediaItemRepository(db)
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, newTestLibrary("cdlib")))
	item := fullItem("cd1", "cdlib", "fp-cd1")
	item.File.ContainerDamagedBytes, item.File.ContainerCheckVersion = 0, 0
	require.NoError(t, repo.Create(ctx, item))

	require.NoError(t, repo.SetContainerDamage(ctx, item.File.Path, 6))
	got, err := repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 6, got.File.ContainerDamagedBytes)
	assert.Equal(t, library.ContainerCheckVersion, got.File.ContainerCheckVersion)

	require.NoError(t, repo.SetContainerDamage(ctx, item.File.Path, 0))
	got, err = repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Zero(t, got.File.ContainerDamagedBytes, "a repaired file is handed over whole again")
}
