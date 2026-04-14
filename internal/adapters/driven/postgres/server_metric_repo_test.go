package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/activity"
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
