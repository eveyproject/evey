package activity

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/activity/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTotaler struct{ total int64 }

func (f *fakeTotaler) Total() int64 { return f.total }

func TestMetricsSampler_SampleOnce_RecordsHostStreamsAndBandwidth(t *testing.T) {
	repo := fakes.NewFakeServerMetricRepository()
	cmd := NewCommandService(nil, repo, nil, nil)
	stats := fakes.NewFakePlaySessionStatsReader()
	stats.ServerStatsResult = &activity.ServerStatsResult{ActiveStreams: 3, ActiveTranscodes: 1}
	meter := &fakeTotaler{}

	s := NewMetricsSampler(cmd, stats, meter, func() SysSample {
		return SysSample{CPUPercent: 42.5, MemUsedBytes: 100, MemTotalBytes: 200}
	}, nil)

	// Pretend the last sample was 10s ago with 0 bytes served; 10 MB since
	// then ≈ 8 Mbps.
	s.lastAt = time.Now().Add(-10 * time.Second)
	s.lastTotal = 0
	meter.total = 10_000_000

	s.SampleOnce(context.Background())

	rows, err := repo.GetByTimeRange(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	m := rows[0]
	assert.Equal(t, 42.5, m.CPUPercent)
	assert.Equal(t, int64(100), m.MemoryUsedBytes)
	assert.Equal(t, int64(200), m.MemoryTotalBytes)
	assert.Equal(t, 3, m.ActiveStreams)
	assert.Equal(t, 1, m.ActiveTranscodes)
	assert.InDelta(t, 8_000_000, m.BandwidthBps, 500_000, "10MB over ~10s is ~8Mbps")
	assert.False(t, m.RecordedAt.IsZero())

	// The delta baseline advanced: an immediate second sample with no new
	// bytes must record ~0 bps, not re-count the same 10MB.
	s.SampleOnce(context.Background())
	rows, err = repo.GetByTimeRange(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Less(t, rows[1].BandwidthBps, int64(100_000), "no new bytes → near-zero rate")
}

func TestMetricsSampler_NilSourcesAreZero(t *testing.T) {
	repo := fakes.NewFakeServerMetricRepository()
	cmd := NewCommandService(nil, repo, nil, nil)
	s := NewMetricsSampler(cmd, nil, nil, nil, nil)

	s.SampleOnce(context.Background())

	rows, err := repo.GetByTimeRange(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].CPUPercent)
	assert.Zero(t, rows[0].BandwidthBps)
}
