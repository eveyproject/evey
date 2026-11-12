package postgres_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 000195 adds the update follow-up columns: a plugin installed before it
// follows nothing, has automatic updates on and no check on record; the up
// replays, the down drops them.
func TestMigration000195_PluginUpdateFollowIsAdditive(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(194))

	columns := func() int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'plugins' AND column_name IN ('update_source', 'update_source_id', 'auto_update', 'update_state')`).Scan(&n))
		return n
	}
	require.Zero(t, columns())
	_, err := db.Exec(`INSERT INTO plugins (id, name, version, install_dir) VALUES ('plg-old', 'old', '1.0.0', '/x')`)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(195))
	require.Equal(t, 4, columns())
	var src, state string
	var srcID int64
	var auto bool
	require.NoError(t, db.QueryRow(`SELECT update_source, update_source_id, auto_update, update_state::text FROM plugins WHERE id = 'plg-old'`).Scan(&src, &srcID, &auto, &state))
	assert.Empty(t, src)
	assert.Zero(t, srcID)
	assert.True(t, auto)
	assert.Equal(t, "{}", state)

	up, err := os.ReadFile("migrations/000195_plugins_update_follow.up.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(up))
	require.NoError(t, err, "the up replays on a schema that has the columns")

	require.NoError(t, m.Migrate(194))
	assert.Zero(t, columns(), "the down drops the columns")
}
