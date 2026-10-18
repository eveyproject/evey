package postgres

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ServerMetricRepository struct {
	db *sql.DB
}

func NewServerMetricRepository(db *sql.DB) *ServerMetricRepository {
	return &ServerMetricRepository{db: db}
}

// serverMetricColumns is the column list every read shares, in the order
// scanMetric expects.
const serverMetricColumns = `id, cpu_percent, memory_used_bytes, memory_total_bytes, active_streams, active_transcodes,
	 bandwidth_bps, storage_used_bytes, storage_total_bytes, iowait_percent, disk_await_ms, disk_util_percent,
	 disk_read_bps, disk_write_bps, disk_busiest, net_rx_bps, net_tx_bps, recorded_at`

// maxMetricRows bounds any history read. One-minute samples over a week are
// 10 080 rows, so a raw read of a week is cut short: callers wanting more
// than a day go through GetBucketed.
// activity.maxRequestedMetricPoints mirrors it: keep the two equal.
const maxMetricRows = 10000

func (r *ServerMetricRepository) Create(ctx context.Context, m *activity.ServerMetric) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.Create")
	defer span.End()
	span.SetAttributes(attribute.String("server_metric.id", m.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO server_metrics (`+serverMetricColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		m.ID, m.CPUPercent, m.MemoryUsedBytes, m.MemoryTotalBytes, m.ActiveStreams,
		m.ActiveTranscodes, m.BandwidthBps, m.StorageUsedBytes, m.StorageTotalBytes,
		m.IOWaitPercent, m.DiskAwaitMs, m.DiskUtilPercent, m.DiskReadBps, m.DiskWriteBps, m.DiskBusiest,
		m.NetRxBps, m.NetTxBps, m.RecordedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ServerMetricRepository) GetByTimeRange(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.GetByTimeRange")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT `+serverMetricColumns+`
		 FROM server_metrics WHERE recorded_at >= $1 AND recorded_at <= $2
		 ORDER BY recorded_at LIMIT $3`, from, to, maxMetricRows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanMetrics(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// GetBucketed groups the samples by buckets of `bucket` seconds aligned on the
// epoch. The id of a bucket row is its start time, since no single sample
// stands for it. The bucket's disk is the one behind its worst wait, so the
// label under a spike names the disk that spiked.
func (r *ServerMetricRepository) GetBucketed(ctx context.Context, from, to time.Time, bucket time.Duration) ([]*activity.ServerMetric, error) {
	if bucket < time.Second {
		return r.GetByTimeRange(ctx, from, to)
	}
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.GetBucketed")
	defer span.End()
	span.SetAttributes(attribute.Int64("server_metric.bucket_seconds", int64(bucket.Seconds())))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT to_char(b, 'YYYY-MM-DD"T"HH24:MI:SSZ') AS id,
		        AVG(cpu_percent), AVG(memory_used_bytes)::bigint, MAX(memory_total_bytes),
		        MAX(active_streams), MAX(active_transcodes), AVG(bandwidth_bps)::bigint,
		        MAX(storage_used_bytes), MAX(storage_total_bytes),
		        AVG(iowait_percent), MAX(disk_await_ms), MAX(disk_util_percent),
		        AVG(disk_read_bps)::bigint, AVG(disk_write_bps)::bigint,
		        COALESCE((array_agg(disk_busiest ORDER BY disk_await_ms DESC, recorded_at))[1], ''),
		        AVG(net_rx_bps)::bigint, AVG(net_tx_bps)::bigint, b AS recorded_at
		 FROM (
		   SELECT *, to_timestamp(floor(extract(epoch FROM recorded_at) / $3) * $3) AS b
		   FROM server_metrics WHERE recorded_at >= $1 AND recorded_at <= $2
		 ) s
		 GROUP BY b ORDER BY b LIMIT $4`,
		from, to, bucket.Seconds(), maxMetricRows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanMetrics(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// IdleDiskAwaitMs reads the quietest disk wait of the window. The idle
// criterion is the one sysinfo applies live, so the chart's reference line
// and the dashboard's verdict agree.
func (r *ServerMetricRepository) IdleDiskAwaitMs(ctx context.Context, since time.Time) (float64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.IdleDiskAwaitMs")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var idle sql.NullFloat64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT MIN(disk_await_ms) FROM server_metrics
		 WHERE recorded_at >= $1 AND disk_await_ms > 0 AND disk_util_percent < $2`,
		since, sysinfo.IdleAwaitUtilCeilPct,
	).Scan(&idle)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	if !idle.Valid {
		return 0, nil
	}
	return idle.Float64, nil
}

func scanMetrics(rows *sql.Rows) ([]*activity.ServerMetric, error) {
	var result []*activity.ServerMetric
	for rows.Next() {
		m := &activity.ServerMetric{}
		if err := rows.Scan(&m.ID, &m.CPUPercent, &m.MemoryUsedBytes, &m.MemoryTotalBytes, &m.ActiveStreams,
			&m.ActiveTranscodes, &m.BandwidthBps, &m.StorageUsedBytes, &m.StorageTotalBytes,
			&m.IOWaitPercent, &m.DiskAwaitMs, &m.DiskUtilPercent, &m.DiskReadBps, &m.DiskWriteBps, &m.DiskBusiest,
			&m.NetRxBps, &m.NetTxBps, &m.RecordedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *ServerMetricRepository) Purge(ctx context.Context, before time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.Purge")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM server_metrics WHERE recorded_at < $1`, before)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// TotalBandwidthBytes integrates the sampled streaming rate over the metric
// history: Σ rate × gap-to-previous-sample. Gaps are capped at 5 minutes so
// server downtime between two samples doesn't get billed as served traffic.
func (r *ServerMetricRepository) TotalBandwidthBytes(ctx context.Context) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.TotalBandwidthBytes")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var total int64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COALESCE(SUM(bandwidth_bps / 8.0 * LEAST(gap, 300)), 0)::bigint FROM (
			SELECT bandwidth_bps,
			       EXTRACT(EPOCH FROM recorded_at - LAG(recorded_at) OVER (ORDER BY recorded_at)) AS gap
			FROM server_metrics
		) t WHERE gap IS NOT NULL AND gap > 0`,
	).Scan(&total)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return total, nil
}

var _ activity.ServerMetricRepository = (*ServerMetricRepository)(nil)
