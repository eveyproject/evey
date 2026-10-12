package library_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type erroringScheduleRepo struct {
	*libfakes.FakeScanScheduleRepository
	updateErr error
	mu        sync.Mutex
	calls     int
}

func (r *erroringScheduleRepo) UpdateAfterRun(ctx context.Context, libraryID string, lastRun, nextRun time.Time) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.FakeScanScheduleRepository.UpdateAfterRun(ctx, libraryID, lastRun, nextRun)
}

type safeBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func captureLogger(buf *safeBuf) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// Kills scheduler.go:96 NEGATION - err != nil branch logs Error when scanner fails.
func TestScheduler_RunScan_ScanErrorLogged(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{returnErr: errors.New("scan boom")}
	var buf safeBuf
	sched := applib.NewScheduler(repo, scanner, captureLogger(&buf))

	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	require.NoError(t, repo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: "lib-err", CronExpression: "0 * * * *", Enabled: true, NextRunAt: &past,
	}))

	sched.SetNowFunc(func() time.Time { return time.Now() })
	sched.Tick(ctx)
	sched.WaitForScans()

	out := buf.String()
	assert.Contains(t, out, "scan failed", "should log scan failure")
	assert.Contains(t, out, "scan boom")
	assert.NotContains(t, out, "scan done", "should not log success on error")
}

// Kills scheduler.go:96 NEGATION (success branch) - logs "scan done" only when err nil.
func TestScheduler_RunScan_SuccessLogged(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	var buf safeBuf
	sched := applib.NewScheduler(repo, scanner, captureLogger(&buf))

	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	require.NoError(t, repo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: "lib-ok", CronExpression: "0 * * * *", Enabled: true, NextRunAt: &past,
	}))

	sched.SetNowFunc(func() time.Time { return time.Now() })
	sched.Tick(ctx)
	sched.WaitForScans()

	out := buf.String()
	assert.Contains(t, out, "scan done")
	assert.NotContains(t, out, "scan failed")
}

// Kills scheduler.go:102 NEGATION - invalid cron expression logs warn.
func TestScheduler_RunScan_InvalidCronLogged(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	var buf safeBuf
	sched := applib.NewScheduler(repo, scanner, captureLogger(&buf))

	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	// Bypass SetSchedule validation by inserting directly
	require.NoError(t, repo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: "lib-bad", CronExpression: "not-a-cron", Enabled: true, NextRunAt: &past,
	}))

	sched.SetNowFunc(func() time.Time { return time.Now() })
	sched.Tick(ctx)
	sched.WaitForScans()

	out := buf.String()
	assert.Contains(t, out, "invalid cron expression")
}

// Kills scheduler.go:102 NEGATION (valid cron branch) - no warn logged.
func TestScheduler_RunScan_ValidCronNoWarn(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	var buf safeBuf
	sched := applib.NewScheduler(repo, scanner, captureLogger(&buf))

	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	require.NoError(t, repo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: "lib-valid", CronExpression: "0 * * * *", Enabled: true, NextRunAt: &past,
	}))

	sched.SetNowFunc(func() time.Time { return time.Now() })
	sched.Tick(ctx)
	sched.WaitForScans()

	out := buf.String()
	assert.NotContains(t, out, "invalid cron expression")
}

// Kills scheduler.go:105 NEGATION - UpdateAfterRun failure logs Error.
func TestScheduler_RunScan_UpdateAfterRunErrorLogged(t *testing.T) {
	base := libfakes.NewFakeScanScheduleRepository()
	repo := &erroringScheduleRepo{FakeScanScheduleRepository: base, updateErr: errors.New("update boom")}
	scanner := &fakeScannerForScheduler{}
	var buf safeBuf
	sched := applib.NewScheduler(repo, scanner, captureLogger(&buf))

	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	require.NoError(t, base.Upsert(ctx, &library.ScanSchedule{
		LibraryID: "lib-upd", CronExpression: "0 * * * *", Enabled: true, NextRunAt: &past,
	}))

	sched.SetNowFunc(func() time.Time { return time.Now() })
	sched.Tick(ctx)
	sched.WaitForScans()

	out := buf.String()
	assert.Contains(t, out, "update after run")
	assert.Contains(t, out, "update boom")
}

// Kills scheduler.go:105 NEGATION (success) - no update error log when ok.
func TestScheduler_RunScan_UpdateAfterRunSuccessNoError(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	scanner := &fakeScannerForScheduler{}
	var buf safeBuf
	sched := applib.NewScheduler(repo, scanner, captureLogger(&buf))

	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	require.NoError(t, repo.Upsert(ctx, &library.ScanSchedule{
		LibraryID: "lib-upd-ok", CronExpression: "0 * * * *", Enabled: true, NextRunAt: &past,
	}))

	sched.SetNowFunc(func() time.Time { return time.Now() })
	sched.Tick(ctx)
	sched.WaitForScans()

	out := buf.String()
	assert.NotContains(t, out, "update after run")
}

// Kills scheduler.go:143 ARITHMETIC_BASE (t.Month()+1) - month rollover when
// no valid day in current month. Cron "0 0 30 * *" from Feb 15 2026 -> Mar 30.
func TestNextCronTime_MonthRollover(t *testing.T) {
	from := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 0 30 * *", from)
	require.NoError(t, err)
	// February has no 30 -> must advance to March 30
	assert.Equal(t, time.March, next.Month())
	assert.Equal(t, 30, next.Day())
	assert.Equal(t, 2026, next.Year())
}

// Additional boundary: cron specifying only unreachable month forces rollover forward.
func TestNextCronTime_SpecificMonthRollover(t *testing.T) {
	from := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 0 1 2 *", from)
	require.NoError(t, err)
	assert.Equal(t, time.February, next.Month())
	assert.Equal(t, 2027, next.Year())
	assert.Equal(t, 1, next.Day())
}

// Kills parseCronField L171 BOUNDARY (i <= max) for "*" expansion - max value included.
func TestNextCronTime_StarIncludesMaxMinute(t *testing.T) {
	// Minute 59 must be valid: from 14:58:00 -> 14:59:00 with "* * * * *"
	from := time.Date(2026, 3, 10, 14, 58, 0, 0, time.UTC)
	next, err := applib.NextCronTime("* * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 59, next.Minute())
}

// Kills parseCronField L171 BOUNDARY - max hour 23 included for "*".
func TestNextCronTime_StarIncludesMaxHour(t *testing.T) {
	from := time.Date(2026, 3, 10, 22, 59, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 23, next.Hour())
	assert.Equal(t, 10, next.Day())
}

// Kills parseCronField L181 BOUNDARY (i <= max) for "*/N" - reaches max when step hits it.
func TestNextCronTime_StepIncludesMax(t *testing.T) {
	// "*/59 * * * *" yields minutes {0, 59}. From 14:30 -> 14:59.
	from := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	next, err := applib.NextCronTime("*/59 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 59, next.Minute())
}

// Kills parseCronField L190 boundaries (lo < min, hi > max, lo > hi) and L193 (i <= hi).
func TestNextCronTime_RangeAtMinBoundary(t *testing.T) {
	// "0-0 * * * *" -> only minute 0 valid
	from := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	next, err := applib.NextCronTime("0-0 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 0, next.Minute())
	assert.Equal(t, 15, next.Hour())
}

func TestNextCronTime_RangeAtMaxBoundary(t *testing.T) {
	// "59-59 * * * *" -> only minute 59
	from := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	next, err := applib.NextCronTime("59-59 * * * *", from)
	require.NoError(t, err)
	assert.Equal(t, 59, next.Minute())
	assert.Equal(t, 14, next.Hour())
}

func TestNextCronTime_RangeHiExceedsMax(t *testing.T) {
	// hi=60 > max=59 must error
	_, err := applib.NextCronTime("0-60 * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid range")
}

func TestNextCronTime_RangeLoBelowMin_DoM(t *testing.T) {
	// day-of-month min=1, "0-5" has lo=0 < min -> error
	_, err := applib.NextCronTime("0 0 0-5 * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid range")
}

func TestNextCronTime_RangeLoGreaterThanHi(t *testing.T) {
	_, err := applib.NextCronTime("10-5 * * * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid range")
}

// Kills parseCronField L199 BOUNDARY (val < min || val > max) exact values.
func TestNextCronTime_ExactMinValueValid(t *testing.T) {
	// minute 0 is min=0 -> valid
	_, err := applib.NextCronTime("0 0 * * *", time.Now())
	require.NoError(t, err)
}

func TestNextCronTime_ExactMaxValueValid(t *testing.T) {
	// minute 59 is max -> valid; hour 23 max -> valid
	_, err := applib.NextCronTime("59 23 * * *", time.Now())
	require.NoError(t, err)
}

func TestNextCronTime_ExactValueBelowMin_DoM(t *testing.T) {
	// day-of-month min=1, value 0 must error
	_, err := applib.NextCronTime("0 0 0 * *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value")
}

func TestNextCronTime_ExactValueAboveMax_Month(t *testing.T) {
	_, err := applib.NextCronTime("0 0 * 13 *", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value")
}

// sanity - ensure capture logger prevents stderr noise
func TestScheduler_CaptureLoggerWorks(t *testing.T) {
	var buf safeBuf
	lg := captureLogger(&buf)
	lg.Info("hello")
	require.True(t, strings.Contains(buf.String(), "hello"))
}
