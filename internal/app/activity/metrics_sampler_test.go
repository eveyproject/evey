package activity

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/activity/fakes"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
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
	assert.InEpsilon(t, 42.5, m.CPUPercent, 1e-9)
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

// EVEY_METRICS_SAMPLE_INTERVAL_SEC was parsed into config and then never read:
// the sampler hardcoded its interval, so an operator who set the variable was
// ignored without a word. A setting that silently does nothing is worse than
// no setting - it sends its user looking for the effect somewhere else.
func TestNewMetricsSampler_HonoursTheConfiguredInterval(t *testing.T) {
	s := NewMetricsSampler(nil, nil, nil, nil, nil).WithSampleInterval(15 * time.Second)
	if got := s.SampleInterval(); got != 15*time.Second {
		t.Fatalf("interval: want 15s, got %v", got)
	}
}

// A non-positive value means "unset" (envInt yields 0 when absent), and an
// absurdly small one would hammer the database once a tick; both fall back to
// the built-in default rather than being applied literally.
func TestNewMetricsSampler_RejectsNonPositiveAndTooSmallIntervals(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Minute, 500 * time.Millisecond} {
		s := NewMetricsSampler(nil, nil, nil, nil, nil).WithSampleInterval(d)
		if got := s.SampleInterval(); got != defaultSampleInterval {
			t.Errorf("interval for %v: want the default %v, got %v", d, defaultSampleInterval, got)
		}
	}
}

// The disks' saturation never showed in the history because no sample carried
// it: the autumn's two production incidents were idle cores waiting on a disk.
// A sample now keeps the disk that waited the most (not the busiest one: a
// disk at 100 % serving requests in 2 ms is fine), sums the rates and fills
// the storage columns that were written as zero since the table existed.
func TestMetricsSampler_RecordsDisksIOWaitNetworkAndStorage(t *testing.T) {
	repo := fakes.NewFakeServerMetricRepository()
	cmd := NewCommandService(nil, repo, nil, nil)
	s := NewMetricsSampler(cmd, nil, nil, func() SysSample {
		return SysSample{
			IOWaitPercent: 42,
			Net:           sysinfo.NetIO{RxBps: 1000, TxBps: 2000},
			Disks: []sysinfo.DiskIO{
				{Label: "QEMU HARDDISK drive-scsi1", AwaitMs: 8, UtilPercent: 100, ReadBps: 10, WriteBps: 1},
				{Label: "QEMU HARDDISK drive-scsi2", AwaitMs: 35, UtilPercent: 60, ReadBps: 20, WriteBps: 2},
			},
		}
	}, nil).WithStorage(func() (int64, int64) { return 300, 1000 })

	s.SampleOnce(context.Background())

	rows, err := repo.GetByTimeRange(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	m := rows[0]
	assert.InEpsilon(t, 42.0, m.IOWaitPercent, 1e-9)
	assert.Equal(t, "QEMU HARDDISK drive-scsi2", m.DiskBusiest, "the disk that waited the most, not the busiest")
	assert.InEpsilon(t, 35.0, m.DiskAwaitMs, 1e-9)
	assert.InEpsilon(t, 60.0, m.DiskUtilPercent, 1e-9)
	assert.Equal(t, int64(30), m.DiskReadBps)
	assert.Equal(t, int64(3), m.DiskWriteBps)
	assert.Equal(t, int64(1000), m.NetRxBps)
	assert.Equal(t, int64(2000), m.NetTxBps)
	assert.Equal(t, int64(300), m.StorageUsedBytes)
	assert.Equal(t, int64(1000), m.StorageTotalBytes)
}

// A week of one-minute samples is 10 080 rows, above the repository's cap, so
// a chart asks for buckets: the width adapts to the window unless the client
// named one, and always lands on a step two clients share.
func TestMetricBucket_FitsTheWindowUnderThePointCap(t *testing.T) {
	cases := []struct {
		window, requested, want time.Duration
	}{
		{time.Hour, 0, time.Minute},
		{24 * time.Hour, 0, 2 * time.Minute},
		{7 * 24 * time.Hour, 0, 15 * time.Minute},
		{30 * 24 * time.Hour, 0, time.Hour},
		{365 * 24 * time.Hour, 0, 24 * time.Hour},
		{7 * 24 * time.Hour, 7 * time.Minute, 10 * time.Minute},
		{time.Hour, 24 * time.Hour, 24 * time.Hour},
	}
	for _, c := range cases {
		if got := MetricBucket(c.window, c.requested); got != c.want {
			t.Errorf("window %v requested %v: want %v, got %v", c.window, c.requested, c.want, got)
		}
	}
}
