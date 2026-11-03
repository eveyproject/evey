package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
)

// 000190 cleans the controls, overrides, zero-width space and BOM out of the
// stored titles, and leaves the joiners, marks and isolates a title is
// written with: what it removes cannot be put back.
func TestMigration000190_CleansControlsKeepsJoiners(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(189))
	ctx := context.Background()
	lib := newTestLibrary("mig190")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	kept := map[string]string{
		"mig190-fa":    "\u0645\u06CC\u200C\u062E\u0648\u0627\u0647\u0645",
		"mig190-hi":    "\u0915\u094D\u200D\u0937",
		"mig190-emoji": "\U0001F468\u200D\U0001F469\u200D\U0001F467",
		"mig190-ar":    "\u2067\u0641\u064A\u0644\u0645\u2069 - Title",
		"mig190-he":    "\u05E9\u05DC\u05D5\u05DD\u200F 2",
	}
	for id := range kept {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, lib.ID)))
	}
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mig190-dirty", lib.ID)))
	for id, title := range kept {
		_, err := db.Exec(`UPDATE media_items SET title = $1 WHERE id = $2`, title, id)
		require.NoError(t, err)
	}
	_, err := db.Exec(`UPDATE media_items SET title = $1, overview = $2 WHERE id = 'mig190-dirty'`,
		"\u202ETi\u200Btle\x07", "Over\uFEFFview\nsecond line")
	require.NoError(t, err)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	for id, title := range kept {
		var got string
		require.NoError(t, db.QueryRow(`SELECT title FROM media_items WHERE id = $1`, id).Scan(&got))
		assert.Equal(t, title, got, id)
	}
	var title, overview string
	require.NoError(t, db.QueryRow(`SELECT title, overview FROM media_items WHERE id = 'mig190-dirty'`).Scan(&title, &overview))
	assert.Equal(t, "Title", title)
	assert.Equal(t, "Overview\nsecond line", overview)
}
