package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackupTables_AllExistInSchema guards against the BackupTables list drifting
// from the schema. A wrong or renamed table name (e.g. "webhooks" instead of
// "webhook_configs") makes pg_dump skip or fail that --table, silently dropping
// the table from every backup - and losing it on restore.
func TestBackupTables_AllExistInSchema(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	for _, tbl := range appbackup.BackupTables {
		var exists bool
		err := db.QueryRowContext(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
			tbl).Scan(&exists)
		require.NoErrorf(t, err, "checking table %q", tbl)
		assert.Truef(t, exists, "BackupTables lists %q but it is not a table in the schema - pg_dump would skip/fail it, losing it on restore", tbl)
	}
}

// seedBackupSchedule re-inserts the singleton row wiped by truncateAll.
func seedBackupSchedule(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO backup_schedule (id) VALUES (TRUE) ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
}

func TestBackupScheduleRepo_Get_DefaultRow(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	sched, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "0 3 * * *", sched.CronExpression)
	assert.False(t, sched.Enabled)
	assert.Equal(t, 30, sched.RetentionDays)
	assert.Nil(t, sched.LastRunAt)
	assert.Nil(t, sched.NextRunAt)
}

func TestBackupScheduleRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	nextRun := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	updated := &backup.BackupSchedule{
		CronExpression: "0 */6 * * *",
		Enabled:        true,
		RetentionDays:  14,
		NextRunAt:      &nextRun,
	}
	require.NoError(t, repo.Update(ctx, updated))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "0 */6 * * *", got.CronExpression)
	assert.True(t, got.Enabled)
	assert.Equal(t, 14, got.RetentionDays)
	require.NotNil(t, got.NextRunAt)
	assert.WithinDuration(t, nextRun, *got.NextRunAt, time.Second)
}

func TestBackupScheduleRepo_AdvanceNextRun_MovesOnlyTheSchedule(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	before, err := repo.Get(ctx)
	require.NoError(t, err)

	nextRun := time.Now().UTC().Truncate(time.Millisecond).Add(24 * time.Hour)
	require.NoError(t, repo.AdvanceNextRun(ctx, nextRun))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.NextRunAt)
	assert.WithinDuration(t, nextRun, *got.NextRunAt, time.Second)
	// The advance fires at TRIGGER time, before the run has an outcome: the
	// run bookkeeping must be untouched.
	assert.Equal(t, before.LastRunAt, got.LastRunAt)
	assert.Equal(t, before.LastStatus, got.LastStatus)
	assert.Equal(t, before.LastError, got.LastError)
}

func TestBackupScheduleRepo_RecordRunResult_Success(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	before, err := repo.Get(ctx)
	require.NoError(t, err)

	lastRun := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.RecordRunResult(ctx, lastRun, "success", ""))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.LastRunAt)
	assert.WithinDuration(t, lastRun, *got.LastRunAt, time.Second)
	assert.Equal(t, "success", got.LastStatus)
	assert.Equal(t, "", got.LastError)
	// The outcome landing must not move the schedule: it advanced at trigger
	// time, and a manual RunNow must not eat the next scheduled slot.
	assert.Equal(t, before.NextRunAt, got.NextRunAt)
}

func TestBackupScheduleRepo_RecordRunResult_Failed(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	lastRun := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.RecordRunResult(ctx, lastRun, "failed", "disk full"))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.LastStatus)
	assert.Equal(t, "disk full", got.LastError)
}

func TestBackupScheduleRepo_Update_DisableRe_enable(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Update(ctx, &backup.BackupSchedule{
		CronExpression: "0 2 * * *",
		Enabled:        true,
		RetentionDays:  7,
	}))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.True(t, got.Enabled)

	require.NoError(t, repo.Update(ctx, &backup.BackupSchedule{
		CronExpression: "0 2 * * *",
		Enabled:        false,
		RetentionDays:  7,
	}))

	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
}
