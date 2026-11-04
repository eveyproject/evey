package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertPerson192(t *testing.T, db *sql.DB, id, name, providerIDs string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO people (id, name, provider_ids) VALUES ($1, $2, $3::jsonb)`, id, name, providerIDs)
	require.NoError(t, err, id)
}

func personName192(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM people WHERE id = $1`, id).Scan(&name))
	return name
}

// 000192 moves the person identity from the name to the provider id on a
// table filled under the old unique name: it must build on those rows, then
// let two homonyms with distinct ids coexist while still refusing two id-less
// rows of one name, and clean the stored names without making two id-less
// names collide.
func TestMigration000192_ProviderIDIdentityOnExistingRows(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(191))

	insertPerson192(t, db, "with-id", "Sam Taylor", `{"tmdb":"101"}`)
	insertPerson192(t, db, "no-id", "Pat Lee", `{}`)
	insertPerson192(t, db, "empty-id", "Lee Pat", `{"tmdb":""}`)
	insertPerson192(t, db, "dirty-with-id", "Ann\u200b Lee", `{"tmdb":"303"}`)
	insertPerson192(t, db, "dirty-free", "Cl\u202eare", `{}`)
	// Its cleaned name is taken by an id-less row: cleaning it would collide.
	insertPerson192(t, db, "dirty-taken", "Pat\x07 Lee", `{}`)
	// Two distinct names under the old unique that the clean-up makes one:
	// up itself creates these homonyms, so down must cope with them.
	insertPerson192(t, db, "kim-a", "Kim\u200b Lee", `{"tmdb":"401"}`)
	insertPerson192(t, db, "kim-b", "Kim Lee\u200b", `{"tmdb":"402"}`)
	// Same name as the id-bearing row above: impossible under the old unique,
	// a homonym with another id.
	_, err := db.Exec(`INSERT INTO people (id, name, provider_ids) VALUES ('homonym', 'Sam Taylor', '{"tmdb":"202"}')`)
	require.Error(t, err, "the old unique name refuses a homonym")

	// Pinned to 192, not Up: the down below must revert this migration, not
	// whichever one comes after it.
	require.NoError(t, m.Migrate(192))

	insertPerson192(t, db, "homonym", "Sam Taylor", `{"tmdb":"202"}`)
	_, err = db.Exec(`INSERT INTO people (id, name, provider_ids) VALUES ('dup-no-id', 'Pat Lee', '{}')`)
	require.Error(t, err, "two id-less rows of one name stay refused")
	_, err = db.Exec(`INSERT INTO people (id, name, provider_ids) VALUES ('dup-empty-id', 'Lee Pat', '{"tmdb":""}')`)
	require.Error(t, err, "an empty id is no id")

	assert.Equal(t, "Ann Lee", personName192(t, db, "dirty-with-id"))
	assert.Equal(t, "Clare", personName192(t, db, "dirty-free"))
	assert.Equal(t, "Pat\x07 Lee", personName192(t, db, "dirty-taken"), "kept: its cleaned name belongs to another id-less row")
	assert.Equal(t, "Pat Lee", personName192(t, db, "no-id"))

	var indexdef string
	require.NoError(t, db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_people_tmdb_id'`).Scan(&indexdef))
	assert.NotContains(t, indexdef, "UNIQUE", "rows sharing one id may already exist")

	assert.Equal(t, "Kim Lee", personName192(t, db, "kim-a"))
	assert.Equal(t, "Kim Lee", personName192(t, db, "kim-b"))
	insertPerson192(t, db, "same-id-a", "Jo Park", `{"tmdb":"501"}`)
	insertPerson192(t, db, "same-id-b", "Jo Park", `{"tmdb":"501"}`)
	var before int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM people`).Scan(&before))

	// Down keeps every row: the previous code needs a unique name, so the
	// homonyms after the first are renamed with their provider id (their row
	// id when the provider id is shared too), never dropped, and the revert
	// ends clean instead of leaving the schema dirty.
	require.NoError(t, m.Migrate(191))
	version, dirty, err := m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(191), version)
	assert.False(t, dirty)
	var after int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM people`).Scan(&after))
	assert.Equal(t, before, after, "no row lost")
	// The row kept as is: the id-less one, else the lowest row id.
	assert.Equal(t, "Sam Taylor", personName192(t, db, "homonym"))
	assert.Equal(t, "Sam Taylor (tmdb 101)", personName192(t, db, "with-id"))
	assert.Equal(t, "Kim Lee", personName192(t, db, "kim-a"))
	assert.Equal(t, "Kim Lee (tmdb 402)", personName192(t, db, "kim-b"))
	assert.Equal(t, "Jo Park", personName192(t, db, "same-id-a"))
	assert.Equal(t, "Jo Park (same-id-b)", personName192(t, db, "same-id-b"))
	_, err = db.Exec(`INSERT INTO people (id, name, provider_ids) VALUES ('again', 'Sam Taylor', '{"tmdb":"909"}')`)
	require.Error(t, err, "the full unique name is back")
}

// With no homonym the revert restores the full unique name.
func TestMigration000192_DownWithoutHomonymsRestoresUniqueName(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(192))
	insertPerson192(t, db, "a", "Sam Taylor", `{"tmdb":"101"}`)
	require.NoError(t, m.Migrate(191))
	_, err := db.Exec(`INSERT INTO people (id, name, provider_ids) VALUES ('b', 'Sam Taylor', '{"tmdb":"202"}')`)
	require.Error(t, err)
}
