package library_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeScannerForScheduler struct {
	called    []string
	returnErr error
}

func (f *fakeScannerForScheduler) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	f.called = append(f.called, libraryID)
	return 1, 0, f.returnErr
}

func TestNextCronTime_EveryHour(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC), next)
}

func TestNextCronTime_Every6Hours(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 */6 * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 18, 0, 0, 0, time.UTC), next)
}

func TestNextCronTime_Daily(t *testing.T) {
	from := time.Date(2026, 3, 10, 3, 30, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 3 * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 11, 3, 0, 0, 0, time.UTC), next)
}

func TestNextCronTime_Weekly(t *testing.T) {
	// 2026-03-10 is Tuesday (weekday 2), so next Sunday (0) is 2026-03-15
	from := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 0 * * 0", from)
	require.NoError(t, err)
	assert.Equal(t, 0, int(next.Weekday()))
}

func TestNextCronTime_InvalidExpr(t *testing.T) {
	_, err := applib.NextCronTime("bad", time.Now())
	require.Error(t, err)
}

func TestNextCronTime_Every30Min(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 10, 0, 0, time.UTC)
	next, err := applib.NextCronTime("*/30 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC), next)
}

func TestScheduler_SetAndGetSchedule(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	got, err := sched.SetSchedule(context.Background(), "lib-1", "0 */6 * * *", true)
	require.NoError(t, err)
	assert.Equal(t, "lib-1", got.LibraryID)
	assert.True(t, got.Enabled)
	assert.NotNil(t, got.NextRunAt)

	fetched, err := sched.GetSchedule(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "0 */6 * * *", fetched.CronExpression)
}

func TestScheduler_SetSchedule_InvalidCron(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	_, err := sched.SetSchedule(context.Background(), "lib-1", "bad", true)
	require.Error(t, err)
}

func TestScheduler_DeleteSchedule(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	sched.SetSchedule(context.Background(), "lib-1", "0 * * * *", true)
	err := sched.DeleteSchedule(context.Background(), "lib-1")
	require.NoError(t, err)

	_, err = sched.GetSchedule(context.Background(), "lib-1")
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestScheduler_GetSchedule_NotFound(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	_, err := sched.GetSchedule(context.Background(), "nonexistent")
	require.ErrorIs(t, err, library.ErrNotFound)
}
