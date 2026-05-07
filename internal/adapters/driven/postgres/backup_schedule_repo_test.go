package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestBackupScheduleRepo_UpdateAfterRun_Success(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	lastRun := time.Now().UTC().Truncate(time.Millisecond)
	nextRun := lastRun.Add(24 * time.Hour)
	require.NoError(t, repo.UpdateAfterRun(ctx, lastRun, nextRun, "success", ""))

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.LastRunAt)
	assert.WithinDuration(t, lastRun, *got.LastRunAt, time.Second)
	require.NotNil(t, got.NextRunAt)
	assert.WithinDuration(t, nextRun, *got.NextRunAt, time.Second)
	assert.Equal(t, "success", got.LastStatus)
	assert.Equal(t, "", got.LastError)
}

func TestBackupScheduleRepo_UpdateAfterRun_Failed(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedBackupSchedule(t, db)

	repo := postgres.NewBackupScheduleRepository(db)
	ctx := context.Background()

	lastRun := time.Now().UTC().Truncate(time.Millisecond)
	nextRun := lastRun.Add(24 * time.Hour)
	require.NoError(t, repo.UpdateAfterRun(ctx, lastRun, nextRun, "failed", "disk full"))

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
