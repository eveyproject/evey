package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerMetricRepo_Create_GetByTimeRange(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewServerMetricRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	m := &activity.ServerMetric{
		ID:                "sm1",
		CPUPercent:        42.5,
		MemoryUsedBytes:   4000000000,
		MemoryTotalBytes:  8000000000,
		ActiveStreams:     3,
		ActiveTranscodes:  1,
		BandwidthBps:      50000000,
		StorageUsedBytes:  100000000000,
		StorageTotalBytes: 500000000000,
		RecordedAt:        now,
	}
	require.NoError(t, repo.Create(ctx, m))

	results, err := repo.GetByTimeRange(ctx, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "sm1", results[0].ID)
	assert.InDelta(t, 42.5, results[0].CPUPercent, 0.01)
}

func TestServerMetricRepo_Purge(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewServerMetricRepository(db)
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Millisecond)
	m := &activity.ServerMetric{
		ID:         "sm2",
		RecordedAt: old,
	}
	require.NoError(t, repo.Create(ctx, m))

	require.NoError(t, repo.Purge(ctx, time.Now().Add(-24*time.Hour)))

	results, err := repo.GetByTimeRange(ctx, old.Add(-1*time.Hour), old.Add(1*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, results)
}

// Consumed bandwidth integrates the sampled rate over the gap to the previous
// sample, with the gap capped at 5 minutes so downtime is never billed as
// served traffic; a lone sample integrates to nothing.
func TestServerMetricRepo_TotalBandwidthBytes(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewServerMetricRepository(db)
	ctx := context.Background()
	base := time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC)
	mk := func(id string, at time.Time, bps int64) {
		t.Helper()
		require.NoError(t, repo.Create(ctx, &activity.ServerMetric{ID: id, BandwidthBps: bps, RecordedAt: at}))
	}
	mk("bw-1", base, 80_000_000)
	total, err := repo.TotalBandwidthBytes(ctx)
	require.NoError(t, err)
	assert.Zero(t, total, "one sample has no gap to integrate over")

	// 60 s later at 80 Mb/s: 10 MB/s x 60 s.
	mk("bw-2", base.Add(60*time.Second), 80_000_000)
	total, err = repo.TotalBandwidthBytes(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 600_000_000, total)

	// An hour of downtime, then a sample at 8 Mb/s: billed for 300 s only.
	mk("bw-3", base.Add(61*time.Minute), 8_000_000)
	total, err = repo.TotalBandwidthBytes(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 600_000_000+1_000_000*300, total)
}

// The new columns round-trip, a week folds into buckets whose averages and
// maxima match the samples and name the disk behind the worst wait, and the
// quiet wait is the lowest one recorded by a nearly idle disk.
func TestServerMetricRepo_BucketedAndIdleAwait(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewServerMetricRepository(db)
	ctx := context.Background()
	base := time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC)
	mk := func(id string, at time.Time, m activity.ServerMetric) {
		t.Helper()
		m.ID, m.RecordedAt = id, at
		require.NoError(t, repo.Create(ctx, &m))
	}
	// Three samples in the first 15-minute bucket, one in the next.
	mk("io-1", base, activity.ServerMetric{CPUPercent: 10, IOWaitPercent: 10, DiskAwaitMs: 7, DiskUtilPercent: 5, DiskBusiest: "disk A", DiskReadBps: 100, NetRxBps: 10, StorageUsedBytes: 10, StorageTotalBytes: 100, ActiveStreams: 1})
	mk("io-2", base.Add(time.Minute), activity.ServerMetric{CPUPercent: 20, IOWaitPercent: 70, DiskAwaitMs: 40, DiskUtilPercent: 60, DiskBusiest: "disk B", DiskReadBps: 300, NetRxBps: 30, StorageUsedBytes: 11, StorageTotalBytes: 100, ActiveStreams: 5})
	mk("io-3", base.Add(2*time.Minute), activity.ServerMetric{CPUPercent: 30, IOWaitPercent: 40, DiskAwaitMs: 30, DiskUtilPercent: 50, DiskBusiest: "disk B", DiskReadBps: 200, NetRxBps: 20, StorageUsedBytes: 12, StorageTotalBytes: 100, ActiveStreams: 3})
	mk("io-4", base.Add(16*time.Minute), activity.ServerMetric{CPUPercent: 50, DiskAwaitMs: 3, DiskUtilPercent: 1, DiskBusiest: "disk A"})

	raw, err := repo.GetByTimeRange(ctx, base, base.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, raw, 4)
	assert.Equal(t, "disk B", raw[1].DiskBusiest)
	assert.InDelta(t, 40, raw[1].DiskAwaitMs, 0.01)
	assert.EqualValues(t, 300, raw[1].DiskReadBps)
	assert.EqualValues(t, 30, raw[1].NetRxBps)
	assert.EqualValues(t, 11, raw[1].StorageUsedBytes)

	buckets, err := repo.GetBucketed(ctx, base, base.Add(time.Hour), 15*time.Minute)
	require.NoError(t, err)
	require.Len(t, buckets, 2)
	b := buckets[0]
	assert.True(t, b.RecordedAt.Equal(base), "bucket start: %v", b.RecordedAt)
	assert.InDelta(t, 20, b.CPUPercent, 0.01, "average")
	assert.InDelta(t, 40, b.IOWaitPercent, 0.01, "average")
	assert.InDelta(t, 40, b.DiskAwaitMs, 0.01, "maximum")
	assert.InDelta(t, 60, b.DiskUtilPercent, 0.01, "maximum")
	assert.EqualValues(t, 200, b.DiskReadBps, "average")
	assert.EqualValues(t, 20, b.NetRxBps, "average")
	assert.EqualValues(t, 12, b.StorageUsedBytes, "maximum")
	assert.Equal(t, 5, b.ActiveStreams, "maximum")
	assert.Equal(t, "disk B", b.DiskBusiest, "the disk behind the worst wait")
	assert.Equal(t, "disk A", buckets[1].DiskBusiest)

	idle, err := repo.IdleDiskAwaitMs(ctx, base)
	require.NoError(t, err)
	assert.InDelta(t, 3, idle, 0.01, "the 7 ms sample was idle too, the 3 ms one is lower; the busy ones do not count")
	idle, err = repo.IdleDiskAwaitMs(ctx, base.Add(time.Hour))
	require.NoError(t, err)
	assert.Zero(t, idle, "nothing recorded since")
}
