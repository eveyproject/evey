package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Real-time monitoring is the default from 000187: a library written before
// it, which could not have it, gets it, and a row inserted without the column
// gets it too.
func TestMigration000187_RealtimeMonitoringIsTheDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	db := migrationDB(t)
	m := newMigrator(t, db)
	if err := m.Migrate(186); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	lib := newTestLibrary("mig187")
	lib.RealtimeMonitoring = false
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, m.Migrate(187))

	got, err := libRepo.GetByID(ctx, lib.ID)
	require.NoError(t, err)
	assert.True(t, got.RealtimeMonitoring, "a library from before the migration is switched on")

	_, err = db.ExecContext(ctx, `INSERT INTO libraries (id, name, type, paths) VALUES ('mig187-raw', 'Raw', 'movie', '["/media/raw"]')`)
	require.NoError(t, err)
	raw, err := libRepo.GetByID(ctx, shared.LibraryID("mig187-raw"))
	require.NoError(t, err)
	assert.True(t, raw.RealtimeMonitoring, "the column defaults to on")

	require.NoError(t, m.Migrate(186))
	got, err = libRepo.GetByID(ctx, lib.ID)
	require.NoError(t, err)
	assert.True(t, got.RealtimeMonitoring, "going down keeps the values, only the default goes back")
}
