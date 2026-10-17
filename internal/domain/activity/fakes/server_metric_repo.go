package fakes

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

type FakeServerMetricRepository struct {
	mu    sync.RWMutex
	items []*activity.ServerMetric
	Err   error
}

func NewFakeServerMetricRepository() *FakeServerMetricRepository {
	return &FakeServerMetricRepository{}
}

// TotalBandwidthBytes mirrors the SQL integral with a fixed 60s sample gap.
func (r *FakeServerMetricRepository) TotalBandwidthBytes(_ context.Context) (int64, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var total int64
	for i, m := range r.items {
		if i == 0 {
			continue
		}
		total += m.BandwidthBps * 60 / 8
	}
	return total, nil
}

func (r *FakeServerMetricRepository) Create(_ context.Context, metric *activity.ServerMetric) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items = append(r.items, metric)
	return nil
}

func (r *FakeServerMetricRepository) GetByTimeRange(_ context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*activity.ServerMetric
	for _, m := range r.items {
		if (m.RecordedAt.Equal(from) || m.RecordedAt.After(from)) && (m.RecordedAt.Equal(to) || m.RecordedAt.Before(to)) {
			result = append(result, m)
		}
	}
	return result, nil
}

// GetBucketed folds the in-memory samples the way the SQL does: averages for
// the percentages and rates, maxima for counts, disk wait and storage, the
// disk behind the worst wait, the bucket start as id and time.
func (r *FakeServerMetricRepository) GetBucketed(ctx context.Context, from, to time.Time, bucket time.Duration) ([]*activity.ServerMetric, error) {
	raw, err := r.GetByTimeRange(ctx, from, to)
	if err != nil || bucket < time.Second {
		return raw, err
	}
	var out []*activity.ServerMetric
	counts := map[int64]int{}
	index := map[int64]int{}
	for _, m := range raw {
		start := m.RecordedAt.Unix() / int64(bucket.Seconds()) * int64(bucket.Seconds())
		i, ok := index[start]
		if !ok {
			i = len(out)
			index[start] = i
			at := time.Unix(start, 0).UTC()
			out = append(out, &activity.ServerMetric{ID: at.Format(time.RFC3339), RecordedAt: at})
		}
		b := out[i]
		counts[start]++
		b.CPUPercent += m.CPUPercent
		b.MemoryUsedBytes += m.MemoryUsedBytes
		b.BandwidthBps += m.BandwidthBps
		b.IOWaitPercent += m.IOWaitPercent
		b.DiskReadBps += m.DiskReadBps
		b.DiskWriteBps += m.DiskWriteBps
		b.NetRxBps += m.NetRxBps
		b.NetTxBps += m.NetTxBps
		b.MemoryTotalBytes = max(b.MemoryTotalBytes, m.MemoryTotalBytes)
		b.ActiveStreams = max(b.ActiveStreams, m.ActiveStreams)
		b.ActiveTranscodes = max(b.ActiveTranscodes, m.ActiveTranscodes)
		b.StorageUsedBytes = max(b.StorageUsedBytes, m.StorageUsedBytes)
		b.StorageTotalBytes = max(b.StorageTotalBytes, m.StorageTotalBytes)
		if m.DiskAwaitMs > b.DiskAwaitMs || b.DiskBusiest == "" {
			b.DiskAwaitMs = m.DiskAwaitMs
			b.DiskBusiest = m.DiskBusiest
		}
		b.DiskUtilPercent = max(b.DiskUtilPercent, m.DiskUtilPercent)
	}
	for start, i := range index {
		b, n := out[i], int64(counts[start])
		b.CPUPercent /= float64(n)
		b.IOWaitPercent /= float64(n)
		b.MemoryUsedBytes /= n
		b.BandwidthBps /= n
		b.DiskReadBps /= n
		b.DiskWriteBps /= n
		b.NetRxBps /= n
		b.NetTxBps /= n
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt.Before(out[j].RecordedAt) })
	return out, nil
}

// IdleDiskAwaitMs mirrors the SQL: the lowest positive wait of a nearly idle
// sample since the given time.
func (r *FakeServerMetricRepository) IdleDiskAwaitMs(_ context.Context, since time.Time) (float64, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	idle := 0.0
	for _, m := range r.items {
		if m.RecordedAt.Before(since) || m.DiskAwaitMs <= 0 || m.DiskUtilPercent >= sysinfo.IdleAwaitUtilCeilPct {
			continue
		}
		if idle == 0 || m.DiskAwaitMs < idle {
			idle = m.DiskAwaitMs
		}
	}
	return idle, nil
}

func (r *FakeServerMetricRepository) Purge(_ context.Context, before time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	var kept []*activity.ServerMetric
	for _, m := range r.items {
		if m.RecordedAt.After(before) || m.RecordedAt.Equal(before) {
			kept = append(kept, m)
		}
	}
	r.items = kept
	return nil
}
