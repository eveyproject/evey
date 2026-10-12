package postgres

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ServerMetricRepository struct {
	db *sql.DB
}

func NewServerMetricRepository(db *sql.DB) *ServerMetricRepository {
	return &ServerMetricRepository{db: db}
}

func (r *ServerMetricRepository) Create(ctx context.Context, m *activity.ServerMetric) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_metric.Create")
	defer span.End()
	span.SetAttributes(attribute.String("server_metric.id", m.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO server_metrics (id, cpu_percent, memory_used_bytes, memory_total_bytes, active_streams,
		 active_transcodes, bandwidth_bps, storage_used_bytes, storage_total_bytes, recorded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		m.ID, m.CPUPercent, m.MemoryUsedBytes, m.MemoryTotalBytes, m.ActiveStreams,
		m.ActiveTranscodes, m.BandwidthBps, m.StorageUsedBytes, m.StorageTotalBytes, m.RecordedAt,
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
		`SELECT id, cpu_percent, memory_used_bytes, memory_total_bytes, active_streams, active_transcodes,
		 bandwidth_bps, storage_used_bytes, storage_total_bytes, recorded_at
		 FROM server_metrics WHERE recorded_at >= $1 AND recorded_at <= $2
		 ORDER BY recorded_at LIMIT 10000`, from, to)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*activity.ServerMetric
	for rows.Next() {
		m := &activity.ServerMetric{}
		if err := rows.Scan(&m.ID, &m.CPUPercent, &m.MemoryUsedBytes, &m.MemoryTotalBytes, &m.ActiveStreams,
			&m.ActiveTranscodes, &m.BandwidthBps, &m.StorageUsedBytes, &m.StorageTotalBytes, &m.RecordedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
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
