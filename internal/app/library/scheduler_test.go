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

func TestScheduler_ListSchedules_Empty(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	list, err := sched.ListSchedules(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestScheduler_ListSchedules_Multiple(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	_, err := sched.SetSchedule(context.Background(), "lib-1", "0 * * * *", true)
	require.NoError(t, err)
	_, err = sched.SetSchedule(context.Background(), "lib-2", "0 */6 * * *", false)
	require.NoError(t, err)

	list, err := sched.ListSchedules(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)
	ids := []string{list[0].LibraryID, list[1].LibraryID}
	assert.Contains(t, ids, "lib-1")
	assert.Contains(t, ids, "lib-2")
}

func TestScheduler_SetSchedule_NextRunIsAfterNow(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	got, err := sched.SetSchedule(context.Background(), "lib-1", "0 * * * *", true)
	require.NoError(t, err)
	require.NotNil(t, got.NextRunAt)
	assert.True(t, got.NextRunAt.After(time.Now().Add(-time.Second)))
}

func TestScheduler_SetSchedule_CronExpressionStored(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	_, err := sched.SetSchedule(context.Background(), "lib-1", "30 2 * * *", true)
	require.NoError(t, err)

	fetched, err := sched.GetSchedule(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "30 2 * * *", fetched.CronExpression)
	assert.True(t, fetched.Enabled)
}

func TestScheduler_SetSchedule_DisabledStored(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	got, err := sched.SetSchedule(context.Background(), "lib-1", "0 3 * * *", false)
	require.NoError(t, err)
	assert.False(t, got.Enabled)

	fetched, err := sched.GetSchedule(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.False(t, fetched.Enabled)
}

func TestScheduler_SetSchedule_UpsertOverwrites(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	_, err := sched.SetSchedule(context.Background(), "lib-1", "0 * * * *", true)
	require.NoError(t, err)
	_, err = sched.SetSchedule(context.Background(), "lib-1", "30 */2 * * *", false)
	require.NoError(t, err)

	fetched, err := sched.GetSchedule(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "30 */2 * * *", fetched.CronExpression)
	assert.False(t, fetched.Enabled)
}

func TestNextCronTime_SpecificDayOfMonth(t *testing.T) {
	from := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 0 15 * *", from)
	require.NoError(t, err)
	assert.Equal(t, 15, next.Day())
	assert.Equal(t, 0, next.Hour())
	assert.Equal(t, 0, next.Minute())
}

func TestNextCronTime_SpecificMonth(t *testing.T) {
	from := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 0 1 6 *", from)
	require.NoError(t, err)
	assert.Equal(t, time.June, next.Month())
	assert.Equal(t, 1, next.Day())
}

func TestNextCronTime_CommaValues(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0,30 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 30, next.Minute())
	assert.Equal(t, 14, next.Hour())
}

func TestNextCronTime_Range(t *testing.T) {
	from := time.Date(2026, 3, 10, 20, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 9-17 * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 9, next.Hour())
	assert.Equal(t, 11, next.Day())
}

func TestNextCronTime_InvalidStep(t *testing.T) {
	_, err := applib.NextCronTime("*/0 * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step")
}

func TestNextCronTime_InvalidRange(t *testing.T) {
	_, err := applib.NextCronTime("* * 31-1 * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid range")
}

func TestNextCronTime_OutOfRangeValue(t *testing.T) {
	_, err := applib.NextCronTime("60 * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value")
}

func TestNextCronTime_NegativeValue(t *testing.T) {
	_, err := applib.NextCronTime("-1 * * * *", time.Now())
	require.Error(t, err)
}

func TestNextCronTime_TooFewFields(t *testing.T) {
	_, err := applib.NextCronTime("0 *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 5 fields")
}

func TestNextCronTime_TooManyFields(t *testing.T) {
	_, err := applib.NextCronTime("0 * * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 5 fields")
}

func TestNextCronTime_InvalidStepNotANumber(t *testing.T) {
	_, err := applib.NextCronTime("*/abc * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step")
}

func TestNextCronTime_InvalidHourRange(t *testing.T) {
	_, err := applib.NextCronTime("0 25 * * *", time.Now())
	require.Error(t, err)
}

func TestNextCronTime_InvalidDOW(t *testing.T) {
	_, err := applib.NextCronTime("0 0 * * 7", time.Now())
	require.Error(t, err)
}

func TestNextCronTime_InvalidMonth(t *testing.T) {
	_, err := applib.NextCronTime("0 0 * 13 *", time.Now())
	require.Error(t, err)
}

func TestNextCronTime_InvalidDayOfMonth(t *testing.T) {
	_, err := applib.NextCronTime("0 0 32 * *", time.Now())
	require.Error(t, err)
}

func TestNextCronTime_EveryMinute(t *testing.T) {
	from := time.Date(2026, 3, 10, 14, 30, 45, 0, time.UTC)
	next, err := applib.NextCronTime("* * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 14, 31, 0, 0, time.UTC), next)
}

func TestNextCronTime_ExactFromTime(t *testing.T) {
	from := time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC), next)
}

func TestScheduler_RunStopsOnCancel(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	sched := applib.NewScheduler(repo, scanner, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sched.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}
