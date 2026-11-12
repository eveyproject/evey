package postgres_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 000194 adds display_name with a {} default: a plugin installed before it
// reads back an empty map (clients fall back on name), the up replays, the
// down drops the column.
func TestMigration000194_PluginDisplayNameIsAdditive(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(193))

	hasColumn := func() bool {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'plugins' AND column_name = 'display_name'`).Scan(&n))
		return n == 1
	}
	require.False(t, hasColumn())
	_, err := db.Exec(`INSERT INTO plugins (id, name, version, install_dir) VALUES ('plg-old', 'old', '1.0.0', '/x')`)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(194))
	require.True(t, hasColumn())
	var dn string
	require.NoError(t, db.QueryRow(`SELECT display_name::text FROM plugins WHERE id = 'plg-old'`).Scan(&dn))
	assert.Equal(t, "{}", dn)

	up, err := os.ReadFile("migrations/000194_plugins_display_name.up.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(up))
	require.NoError(t, err, "the up replays on a schema that has the column")

	require.NoError(t, m.Migrate(193))
	assert.False(t, hasColumn(), "the down drops the column")
}
