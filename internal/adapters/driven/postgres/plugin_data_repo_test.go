package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedPlugin(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO plugins (id, name) VALUES ($1, $2)`, id, "name-"+id)
	require.NoError(t, err)
}

func seedProfile(t *testing.T, db *sql.DB, id, userID string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO profiles (id, user_id, name) VALUES ($1, $2, $3)`, id, userID, "prof-"+id)
	require.NoError(t, err)
}

func TestPluginDataRepo_GlobalSetGetUpsertDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedPlugin(t, db, "p1")
	repo := postgres.NewPluginDataRepository(db)
	ctx := context.Background()

	// absent → ErrNotFound
	_, err := repo.Get(ctx, "p1", "", "", "k")
	require.ErrorIs(t, err, shared.ErrNotFound)

	require.NoError(t, repo.Set(ctx, "p1", "", "", "k", "v1"))
	got, err := repo.Get(ctx, "p1", "", "", "k")
	require.NoError(t, err)
	assert.Equal(t, "v1", got.Value)
	assert.Empty(t, got.UserID)

	// upsert
	require.NoError(t, repo.Set(ctx, "p1", "", "", "k", "v2"))
	got, _ = repo.Get(ctx, "p1", "", "", "k")
	assert.Equal(t, "v2", got.Value)

	require.NoError(t, repo.Delete(ctx, "p1", "", "", "k"))
	_, err = repo.Get(ctx, "p1", "", "", "k")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPluginDataRepo_ScopeSeparation(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedPlugin(t, db, "p1")
	seedUser(t, db, "u1")
	repo := postgres.NewPluginDataRepository(db)
	ctx := context.Background()

	// Same key in global and user scope are independent rows.
	require.NoError(t, repo.Set(ctx, "p1", "", "", "theme", "global"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "", "theme", "user"))

	g, _ := repo.Get(ctx, "p1", "", "", "theme")
	u, _ := repo.Get(ctx, "p1", "u1", "", "theme")
	assert.Equal(t, "global", g.Value)
	assert.Equal(t, "user", u.Value)
	assert.Equal(t, "u1", u.UserID)

	// List is scoped.
	gl, err := repo.List(ctx, "p1", "", "")
	require.NoError(t, err)
	assert.Len(t, gl, 1)
	ul, err := repo.List(ctx, "p1", "u1", "")
	require.NoError(t, err)
	assert.Len(t, ul, 1)
}

// Per-profile rows are keyed by profile alone: two of a user's profiles keep
// independent values for the same key, those rows never leak into the per-user
// scope, and they cascade-delete with the profile.
func TestPluginDataRepo_PerProfileScope(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedPlugin(t, db, "p1")
	seedUser(t, db, "u1")
	seedProfile(t, db, "prf-a", "u1")
	seedProfile(t, db, "prf-b", "u1")
	repo := postgres.NewPluginDataRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "p1", "u1", "prf-a", "city", "Paris"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "prf-b", "city", "Lyon"))

	a, err := repo.Get(ctx, "p1", "u1", "prf-a", "city")
	require.NoError(t, err)
	assert.Equal(t, "Paris", a.Value)
	assert.Equal(t, "prf-a", a.ProfileID)
	b, err := repo.Get(ctx, "p1", "u1", "prf-b", "city")
	require.NoError(t, err)
	assert.Equal(t, "Lyon", b.Value)

	// per-profile data must not appear in the per-user scope
	_, err = repo.Get(ctx, "p1", "u1", "", "city")
	require.ErrorIs(t, err, shared.ErrNotFound)

	// List is per-profile.
	la, err := repo.List(ctx, "p1", "u1", "prf-a")
	require.NoError(t, err)
	assert.Len(t, la, 1)

	// Deleting a profile cascades only its own rows.
	_, err = db.Exec(`DELETE FROM profiles WHERE id=$1`, "prf-a")
	require.NoError(t, err)
	_, err = repo.Get(ctx, "p1", "u1", "prf-a", "city")
	require.ErrorIs(t, err, shared.ErrNotFound, "per-profile data must cascade with the profile")
	b2, err := repo.Get(ctx, "p1", "u1", "prf-b", "city")
	require.NoError(t, err)
	assert.Equal(t, "Lyon", b2.Value, "another profile's data survives")
}

func TestPluginDataRepo_CascadeOnPluginDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedPlugin(t, db, "p1")
	seedUser(t, db, "u1")
	repo := postgres.NewPluginDataRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "p1", "", "", "g", "x"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "", "k", "y"))

	// Uninstall = hard DELETE FROM plugins → ON DELETE CASCADE wipes plugin_data.
	_, err := db.Exec(`DELETE FROM plugins WHERE id = $1`, "p1")
	require.NoError(t, err)

	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM plugin_data WHERE plugin_id=$1`, "p1").Scan(&n))
	assert.Zero(t, n, "plugin data must cascade-delete with the plugin")
}

func TestPluginDataRepo_CascadeOnUserDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedPlugin(t, db, "p1")
	seedUser(t, db, "u1")
	repo := postgres.NewPluginDataRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "p1", "", "", "g", "x"))   // global survives
	require.NoError(t, repo.Set(ctx, "p1", "u1", "", "k", "y")) // user-scoped cascades

	_, err := db.Exec(`DELETE FROM users WHERE id = $1`, "u1")
	require.NoError(t, err)

	_, err = repo.Get(ctx, "p1", "u1", "", "k")
	require.ErrorIs(t, err, shared.ErrNotFound, "user-scoped data must cascade with the user")
	g, err := repo.Get(ctx, "p1", "", "", "g")
	require.NoError(t, err)
	assert.Equal(t, "x", g.Value, "global data must survive a user deletion")
}

// Count answers the per-scope key count the data RPCs cap on, without
// reading every value back.
func TestPluginDataRepo_CountPerScope(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedPlugin(t, db, "p1")
	seedPlugin(t, db, "p2")
	seedUser(t, db, "u1")
	seedProfile(t, db, "prf-a", "u1")
	repo := postgres.NewPluginDataRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "p1", "", "", "a", "1"))
	require.NoError(t, repo.Set(ctx, "p1", "", "", "b", "1"))
	require.NoError(t, repo.Set(ctx, "p1", "", "", "b", "2"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "", "a", "1"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "prf-a", "a", "1"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "prf-a", "b", "1"))
	require.NoError(t, repo.Set(ctx, "p1", "u1", "prf-a", "c", "1"))
	require.NoError(t, repo.Set(ctx, "p2", "", "", "a", "1"))

	for _, c := range []struct {
		name                  string
		plugin, user, profile string
		want                  int
	}{
		{"global", "p1", "", "", 2},
		{"per user", "p1", "u1", "", 1},
		{"per profile", "p1", "u1", "prf-a", 3},
		{"other plugin", "p2", "", "", 1},
		{"empty", "p2", "u1", "", 0},
	} {
		n, err := repo.Count(ctx, c.plugin, c.user, c.profile)
		require.NoError(t, err, c.name)
		assert.Equal(t, c.want, n, c.name)
	}
}
