package postgres_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 000193 only adds a nullable column: rows stored before it read back NULL
// (never probed for it, not track 0), the down drops it, and the up replays
// on a schema that already has it.
func TestMigration000193_SourceTrackIDIsAdditive(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(192))

	hasColumn := func() bool {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'media_streams' AND column_name = 'source_track_id'`).Scan(&n))
		return n == 1
	}
	require.False(t, hasColumn())

	require.NoError(t, m.Migrate(193))
	require.True(t, hasColumn())
	var nullable string
	require.NoError(t, db.QueryRow(`SELECT is_nullable FROM information_schema.columns
		WHERE table_name = 'media_streams' AND column_name = 'source_track_id'`).Scan(&nullable))
	assert.Equal(t, "YES", nullable)

	up, err := os.ReadFile("migrations/000193_media_streams_source_track_id.up.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(up))
	require.NoError(t, err, "the up replays on a schema that has the column")

	require.NoError(t, m.Migrate(192))
	assert.False(t, hasColumn(), "the down drops the column")
}
