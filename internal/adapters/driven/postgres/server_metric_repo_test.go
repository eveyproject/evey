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
