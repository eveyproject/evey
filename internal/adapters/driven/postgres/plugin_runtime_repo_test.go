package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginRuntimeRepo_Create_GetByLanguageAndVersion(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRuntimeRepository(db)
	ctx := context.Background()

	rt := &plugin.PluginRuntime{
		ID:       "prt1",
		Language: "python",
		Version:  "3.11",
		Status:   "supported",
		Notes:    "tested ok",
		TestedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, rt))

	got, err := repo.GetByLanguageAndVersion(ctx, "python", "3.11")
	require.NoError(t, err)
	assert.Equal(t, "prt1", got.ID)
	assert.Equal(t, plugin.RuntimeStatus("supported"), got.Status)
}

func TestPluginRuntimeRepo_ListByLanguage(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRuntimeRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, &plugin.PluginRuntime{ID: "prt2a", Language: "go", Version: "1.21", Status: "supported", TestedAt: now}))
	require.NoError(t, repo.Create(ctx, &plugin.PluginRuntime{ID: "prt2b", Language: "go", Version: "1.22", Status: "supported", TestedAt: now}))

	list, err := repo.ListByLanguage(ctx, "go")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestPluginRuntimeRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRuntimeRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, &plugin.PluginRuntime{ID: "prt3a", Language: "js", Version: "18", Status: "supported", TestedAt: now}))
	require.NoError(t, repo.Create(ctx, &plugin.PluginRuntime{ID: "prt3b", Language: "python", Version: "3.12", Status: "beta", TestedAt: now}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestPluginRuntimeRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRuntimeRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, &plugin.PluginRuntime{ID: "prt4", Language: "rust", Version: "1.75", Status: "experimental", TestedAt: now}))
	require.NoError(t, repo.Delete(ctx, "prt4"))

	_, err := repo.GetByLanguageAndVersion(ctx, "rust", "1.75")
	assert.Error(t, err)
}

func TestPluginRuntimeRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRuntimeRepository(db)
	err := repo.Delete(context.Background(), "prt-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}
