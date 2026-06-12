package activity

import (
	"context"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/google/uuid"
)

// defaultSampleInterval is how often a server-metric row is recorded. One row
// a minute is 1 440/day — small enough that the existing
// EVEY_METRICS_RETENTION_DAYS purge comfortably bounds the table.
const defaultSampleInterval = time.Minute

// BytesTotaler reports cumulative bytes served (satisfied by netmeter.Meter).
type BytesTotaler interface{ Total() int64 }

// SysSample is the point-in-time host snapshot the sampler persists.
type SysSample struct {
	CPUPercent    float64
	MemUsedBytes  int64
	MemTotalBytes int64
}

// MetricsSampler periodically records a ServerMetric row: CPU/memory, active
// stream/transcode counts and the streaming bandwidth actually served over
// the last interval (from the byte meter). It is what finally feeds the
// server_metrics table — RecordMetric/ListServerMetrics existed with no
// producer, so the history was empty and "bandwidth consumed" had no source.
type MetricsSampler struct {
	cmd      *CommandService
	stats    activity.PlaySessionStatsReader
	meter    BytesTotaler
	collect  func() SysSample
	interval time.Duration
	logger   *slog.Logger

	lastTotal int64
	lastAt    time.Time
}

// NewMetricsSampler wires a sampler. collect provides the host CPU/memory
// snapshot (injected so tests don't read /proc); stats and meter may be nil
// (their fields are recorded as zero).
func NewMetricsSampler(
	cmd *CommandService,
	stats activity.PlaySessionStatsReader,
	meter BytesTotaler,
	collect func() SysSample,
	logger *slog.Logger,
) *MetricsSampler {
	if logger == nil {
		logger = slog.Default()
	}
	return &MetricsSampler{
		cmd:      cmd,
		stats:    stats,
		meter:    meter,
		collect:  collect,
		interval: defaultSampleInterval,
		logger:   logger.With("component", "metrics_sampler"),
	}
}

// Run samples on a fixed interval until ctx is done. Errors are logged and
// the loop continues — a DB blip must not stop the history.
func (s *MetricsSampler) Run(ctx context.Context) {
	if s.meter != nil {
		s.lastTotal = s.meter.Total()
	}
	s.lastAt = time.Now()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SampleOnce(ctx)
		}
	}
}

// SampleOnce records a single metric row. Exported for tests.
func (s *MetricsSampler) SampleOnce(ctx context.Context) {
	m := &activity.ServerMetric{ID: uuid.New().String()}

	if s.collect != nil {
		sys := s.collect()
		m.CPUPercent = sys.CPUPercent
		m.MemoryUsedBytes = sys.MemUsedBytes
		m.MemoryTotalBytes = sys.MemTotalBytes
	}
	if s.stats != nil {
		if st, err := s.stats.GetServerStats(ctx); err != nil {
			s.logger.Warn("metrics sample: server stats failed", slog.Any("err", err))
		} else {
			m.ActiveStreams = st.ActiveStreams
			m.ActiveTranscodes = st.ActiveTranscodes
		}
	}
	if s.meter != nil {
		now := time.Now()
		total := s.meter.Total()
		if elapsed := now.Sub(s.lastAt).Seconds(); elapsed > 0 && total >= s.lastTotal {
			m.BandwidthBps = int64(float64(total-s.lastTotal) * 8 / elapsed)
		}
		s.lastTotal, s.lastAt = total, now
	}

	if err := s.cmd.RecordMetric(ctx, m); err != nil {
		s.logger.Warn("metrics sample: record failed", slog.Any("err", err))
	}
}
