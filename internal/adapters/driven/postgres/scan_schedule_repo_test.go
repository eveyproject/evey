package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanScheduleRepo_Upsert_GetByLibraryID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	schedRepo := postgres.NewScanScheduleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sslib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	nextRun := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	sched := &library.ScanSchedule{
		LibraryID:      lib.ID,
		CronExpression: "0 */6 * * *",
		Enabled:        true,
		NextRunAt:      &nextRun,
	}
	require.NoError(t, schedRepo.Upsert(ctx, sched))

	got, err := schedRepo.GetByLibraryID(ctx, string(lib.ID))
	require.NoError(t, err)
	assert.Equal(t, lib.ID, got.LibraryID)
	assert.Equal(t, "0 */6 * * *", got.CronExpression)
	assert.True(t, got.Enabled)
	assert.NotNil(t, got.NextRunAt)
}

func TestScanScheduleRepo_Upsert_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	schedRepo := postgres.NewScanScheduleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sslib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	sched := &library.ScanSchedule{
		LibraryID: lib.ID, CronExpression: "0 0 * * *", Enabled: true,
	}
	require.NoError(t, schedRepo.Upsert(ctx, sched))

	sched.CronExpression = "0 */12 * * *"
	sched.Enabled = false
	require.NoError(t, schedRepo.Upsert(ctx, sched))

	got, err := schedRepo.GetByLibraryID(ctx, string(lib.ID))
	require.NoError(t, err)
	assert.Equal(t, "0 */12 * * *", got.CronExpression)
	assert.False(t, got.Enabled)
}

func TestScanScheduleRepo_ListEnabled(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	schedRepo := postgres.NewScanScheduleRepository(db)
	ctx := context.Background()

	lib1 := newTestLibrary("sslib3a")
	lib2 := newTestLibrary("sslib3b")
	require.NoError(t, libRepo.Create(ctx, lib1))
	require.NoError(t, libRepo.Create(ctx, lib2))

	require.NoError(t, schedRepo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: lib1.ID, CronExpression: "0 0 * * *", Enabled: true,
	}))
	require.NoError(t, schedRepo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: lib2.ID, CronExpression: "0 12 * * *", Enabled: false,
	}))

	got, err := schedRepo.ListEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, lib1.ID, got[0].LibraryID)

	// The admin table lists every schedule, disabled ones included, by library.
	all, err := schedRepo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, lib1.ID, all[0].LibraryID)
	assert.True(t, all[0].Enabled)
	assert.Equal(t, lib2.ID, all[1].LibraryID)
	assert.False(t, all[1].Enabled)
	assert.Equal(t, "0 12 * * *", all[1].CronExpression)
}

func TestScanScheduleRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	schedRepo := postgres.NewScanScheduleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sslib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	require.NoError(t, schedRepo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: lib.ID, CronExpression: "0 0 * * *", Enabled: true,
	}))

	require.NoError(t, schedRepo.Delete(ctx, string(lib.ID)))

	_, err := schedRepo.GetByLibraryID(ctx, string(lib.ID))
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestScanScheduleRepo_UpdateAfterRun(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	schedRepo := postgres.NewScanScheduleRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("sslib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	require.NoError(t, schedRepo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: lib.ID, CronExpression: "0 0 * * *", Enabled: true,
	}))

	lastRun := time.Now().UTC().Truncate(time.Millisecond)
	nextRun := lastRun.Add(24 * time.Hour)
	require.NoError(t, schedRepo.UpdateAfterRun(ctx, string(lib.ID), lastRun, nextRun))

	got, err := schedRepo.GetByLibraryID(ctx, string(lib.ID))
	require.NoError(t, err)
	require.NotNil(t, got.LastRunAt)
	assert.Equal(t, lastRun, got.LastRunAt.Truncate(time.Millisecond))
	require.NotNil(t, got.NextRunAt)
	assert.Equal(t, nextRun, got.NextRunAt.Truncate(time.Millisecond))
}

func TestScanScheduleRepo_GetByLibraryID_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	schedRepo := postgres.NewScanScheduleRepository(db)
	_, err := schedRepo.GetByLibraryID(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}
