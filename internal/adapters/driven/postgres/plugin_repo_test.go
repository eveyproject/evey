package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
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
		Status:       plugin.Stopped,
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
	p.Status = plugin.Running
	p.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got.Version)
	assert.Equal(t, plugin.Running, got.Status)
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

func TestPluginRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	err := repo.Delete(context.Background(), "plugin-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPluginRepo_Update_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	p := newTestPlugin("plugin-missing")
	err := repo.Update(context.Background(), p)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPluginRepo_Kind_Plugin(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl-kind-plugin")
	p.Kind = plugin.KindPlugin
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, plugin.KindPlugin, got.Kind)
}

func TestPluginRepo_Kind_Theme(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl-kind-theme")
	p.Kind = plugin.KindTheme
	p.Language = "css"
	p.Entrypoint = ""
	p.Capabilities = []string{}
	p.Permissions = []string{}
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, plugin.KindTheme, got.Kind)
}

func TestPluginRepo_Frontend_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	p := newTestPlugin("pl-frontend")
	p.Kind = plugin.KindPlugin
	p.Frontend = []plugin.FrontModule{{Slot: "home.section", Src: "front.js"}}
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, got.Frontend, 1)
	assert.Equal(t, "home.section", got.Frontend[0].Slot)
	assert.Equal(t, "front.js", got.Frontend[0].Src)
}

// TestPluginRepo_Kind_Backfill verifies the Go-level backfill that deduces
// Kind from Language when a row was written without a kind value
// (simulates rows that predate the DEFAULT constraint clean-up).
func TestPluginRepo_Kind_Backfill(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()

	insertMinimal := func(t *testing.T, db *sql.DB, id, language, kind string) {
		t.Helper()
		_, err := db.ExecContext(ctx, `
			INSERT INTO plugins (id, name, version, language, entrypoint, install_dir,
			                     capabilities, permissions, config_schema, config,
			                     enabled, status, installed_at, updated_at, kind)
			VALUES ($1, $2, '1.0.0', $3, '', '',
			        '[]', '[]', '{}', '{}',
			        true, 'stopped', NOW(), NOW(), $4)`,
			id, "plugin_"+id, language, kind)
		require.NoError(t, err)
	}

	// Row with kind='' (simulates pre-migration-cleanup row) and non-css language
	insertMinimal(t, db, "bl-plugin", "python", "")
	got, err := repo.GetByID(ctx, "bl-plugin")
	require.NoError(t, err)
	assert.Equal(t, plugin.KindPlugin, got.Kind)

	// Row with kind='' (simulates pre-migration-cleanup row) and css language
	insertMinimal(t, db, "bl-theme", "css", "")
	got, err = repo.GetByID(ctx, "bl-theme")
	require.NoError(t, err)
	assert.Equal(t, plugin.KindTheme, got.Kind)
}

// UpdateConfig writes the config column ONLY: a plugin calling SetConfig with
// a stale snapshot must not be able to restore a permission the admin just
// revoked through the wide Update.
func TestPluginRepo_UpdateConfig_TouchesOnlyConfig(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewPluginRepository(db)
	ctx := context.Background()
	p := newTestPlugin("plg-cfg")
	p.Permissions = []string{"read", "external_fetch"}
	require.NoError(t, repo.Create(ctx, p))

	// The admin revokes a permission.
	p.Permissions = []string{"read"}
	require.NoError(t, repo.Update(ctx, p))

	require.NoError(t, repo.UpdateConfig(ctx, "plg-cfg", map[string]any{"api_key": "k", "threshold": 3.5}))
	got, err := repo.GetByID(ctx, "plg-cfg")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"api_key": "k", "threshold": 3.5}, got.Config)
	assert.Equal(t, []string{"read"}, got.Permissions, "the revoked permission stays revoked")
	assert.True(t, got.Enabled)

	require.ErrorIs(t, repo.UpdateConfig(ctx, "plg-ghost", map[string]any{}), shared.ErrNotFound)
}
