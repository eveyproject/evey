package postgres

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	_, err := r.db.ExecContext(ctx,
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
	rows, err := r.db.QueryContext(ctx,
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
	_, err := r.db.ExecContext(ctx, `DELETE FROM server_metrics WHERE recorded_at < $1`, before)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ activity.ServerMetricRepository = (*ServerMetricRepository)(nil)
