package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlugin(id string) *plugin.Plugin {
	return &plugin.Plugin{
		ID:           id,
		Name:         "plugin_" + id,
		Version:      "1.0.0",
		Description:  "Test plugin",
		Author:       "tester",
		Language:     "python",
		Capabilities: []string{"metadata"},
		Permissions:  []string{"read"},
		ConfigSchema: map[string]any{},
		Config:       map[string]any{},
		Enabled:      true,
		Status:       string(plugin.Stopped),
		ErrorMessage: "",
		InstalledAt:  time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt:    time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestPluginRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl1")
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.Version, got.Version)
	assert.Equal(t, p.Capabilities, got.Capabilities)
}

func TestPluginRepo_GetByName(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl2")
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByName(ctx, p.Name)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
}

func TestPluginRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestPlugin("pl3a")))
	require.NoError(t, repo.Create(ctx, newTestPlugin("pl3b")))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestPluginRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl4")
	require.NoError(t, repo.Create(ctx, p))

	p.Version = "2.0.0"
	p.Status = string(plugin.Running)
	p.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got.Version)
	assert.Equal(t, string(plugin.Running), got.Status)
}

func TestPluginRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl5")
	require.NoError(t, repo.Create(ctx, p))
	require.NoError(t, repo.Delete(ctx, p.ID))

	_, err := repo.GetByID(ctx, p.ID)
	assert.Error(t, err)
}
