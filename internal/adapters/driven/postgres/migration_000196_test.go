package postgres_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
)

// 000196 trusts the Evey project's signing key: the stored fingerprint is the
// one the PEM really has, the row is not seeded (the boot prune would drop
// it), a key the admin already named keeps its name, the up replays and the
// down removes only the row it added.
func TestMigration000196_TrustsTheProjectKey(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(195))
	require.NoError(t, m.Migrate(196))

	var fp, name, pem string
	var seeded bool
	require.NoError(t, db.QueryRow(`SELECT fingerprint, name, public_key_pem, seeded FROM plugin_trusted_publishers`).Scan(&fp, &name, &pem, &seeded))
	gotFP, canonical, err := plugindomain.ParsePublisherKey([]byte(pem))
	require.NoError(t, err)
	assert.Equal(t, gotFP, fp, "the fingerprint is the one the stored key has")
	assert.Equal(t, canonical, pem, "the key is stored the way the repository stores one")
	assert.Equal(t, "Evey project", name)
	assert.False(t, seeded)

	up, err := os.ReadFile("migrations/000196_plugin_trust_project_key.up.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(up))
	require.NoError(t, err, "the up replays")

	require.NoError(t, m.Migrate(195))
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM plugin_trusted_publishers`).Scan(&n))
	assert.Zero(t, n, "the down removes the row")

	_, err = db.Exec(`INSERT INTO plugin_trusted_publishers (fingerprint, name, public_key_pem) VALUES ($1, 'Mine', $2)`, fp, pem)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(196))
	require.NoError(t, db.QueryRow(`SELECT name FROM plugin_trusted_publishers WHERE fingerprint = $1`, fp).Scan(&name))
	assert.Equal(t, "Mine", name, "a key the admin already trusted keeps its name")
	require.NoError(t, m.Migrate(195))
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM plugin_trusted_publishers`).Scan(&n))
	assert.Equal(t, 1, n, "the down leaves the admin's own row")
}
