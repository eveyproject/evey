package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type ServerMetricRepository struct {
	db *sql.DB
}

func NewServerMetricRepository(db *sql.DB) *ServerMetricRepository {
	return &ServerMetricRepository{db: db}
}

func (r *ServerMetricRepository) Create(ctx context.Context, m *activity.ServerMetric) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_metrics (id, cpu_percent, memory_used_bytes, memory_total_bytes, active_streams,
		 active_transcodes, bandwidth_bps, storage_used_bytes, storage_total_bytes, recorded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		m.ID, m.CPUPercent, m.MemoryUsedBytes, m.MemoryTotalBytes, m.ActiveStreams,
		m.ActiveTranscodes, m.BandwidthBps, m.StorageUsedBytes, m.StorageTotalBytes, m.RecordedAt,
	)
	return err
}

func (r *ServerMetricRepository) GetByTimeRange(ctx context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, cpu_percent, memory_used_bytes, memory_total_bytes, active_streams, active_transcodes,
		 bandwidth_bps, storage_used_bytes, storage_total_bytes, recorded_at
		 FROM server_metrics WHERE recorded_at >= $1 AND recorded_at <= $2
		 ORDER BY recorded_at`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*activity.ServerMetric
	for rows.Next() {
		m := &activity.ServerMetric{}
		if err := rows.Scan(&m.ID, &m.CPUPercent, &m.MemoryUsedBytes, &m.MemoryTotalBytes, &m.ActiveStreams,
			&m.ActiveTranscodes, &m.BandwidthBps, &m.StorageUsedBytes, &m.StorageTotalBytes, &m.RecordedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *ServerMetricRepository) Purge(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM server_metrics WHERE recorded_at < $1`, before)
	return err
}

var _ activity.ServerMetricRepository = (*ServerMetricRepository)(nil)
