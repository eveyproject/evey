package library

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
)

type tickFakeScanner struct{ calls atomic.Int32 }

func (f *tickFakeScanner) ScanLibrary(_ context.Context, _ string) (int, int, error) {
	f.calls.Add(1)
	return 0, 0, nil
}

// tick must advance NextRunAt at trigger time, not after the scan completes -
// otherwise a tick during a long scan re-fires the schedule (the scan lock
// rejects the duplicate but logs a spurious failure every long scan on a NAS).
func TestScheduler_Tick_AdvancesNextRunAtAtTrigger(t *testing.T) {
	repo := libfakes.NewFakeScanScheduleRepository()
	past := time.Now().Add(-time.Hour)
	require.NoError(t, repo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 * * * *",
		Enabled:        true,
		NextRunAt:      &past,
	}))
	scanner := &tickFakeScanner{}
	sched := NewScheduler(repo, scanner, slog.New(slog.NewTextHandler(io.Discard, nil)))

	sched.tick(context.Background())

	// NextRunAt is advanced synchronously at trigger (before the scan goroutine).
	got, err := repo.GetByLibraryID(context.Background(), "lib-1")
	require.NoError(t, err)
	require.NotNil(t, got.NextRunAt)
	assert.True(t, got.NextRunAt.After(time.Now()), "NextRunAt must advance at trigger, not after the scan")

	// A second tick must not re-fire it (NextRunAt is now in the future).
	sched.tick(context.Background())
	sched.scanWG.Wait()
	assert.Equal(t, int32(1), scanner.calls.Load(), "only the first tick triggers a scan")
}
