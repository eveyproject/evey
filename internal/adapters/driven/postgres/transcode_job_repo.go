package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

type TranscodeJobRepository struct {
	db *sql.DB
}

func NewTranscodeJobRepository(db *sql.DB) *TranscodeJobRepository {
	return &TranscodeJobRepository{db: db}
}

func (r *TranscodeJobRepository) Create(ctx context.Context, job *transcoding.TranscodeJob) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO transcode_sessions (id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		job.ID, job.MediaItemID, job.InputCodec, job.OutputCodec, job.HWAccelType, job.Preset,
		job.ResolutionIn, job.ResolutionOut, job.BitrateIn, job.BitrateOut, job.Speed,
		job.Status, job.ErrorMessage, job.StartedAt, job.EndedAt, job.DurationMs,
	)
	return err
}

func (r *TranscodeJobRepository) GetByID(ctx context.Context, id string) (*transcoding.TranscodeJob, error) {
	return r.scanJob(r.db.QueryRowContext(ctx,
		`SELECT id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms
		 FROM transcode_sessions WHERE id = $1`, id))
}

func (r *TranscodeJobRepository) Update(ctx context.Context, job *transcoding.TranscodeJob) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE transcode_sessions SET media_item_id=$1, input_codec=$2, output_codec=$3, hw_accel_type=$4,
		 preset=$5, resolution_in=$6, resolution_out=$7, bitrate_in=$8, bitrate_out=$9, speed=$10,
		 status=$11, error_message=$12, started_at=$13, ended_at=$14, duration_ms=$15
		 WHERE id=$16`,
		job.MediaItemID, job.InputCodec, job.OutputCodec, job.HWAccelType,
		job.Preset, job.ResolutionIn, job.ResolutionOut, job.BitrateIn, job.BitrateOut, job.Speed,
		job.Status, job.ErrorMessage, job.StartedAt, job.EndedAt, job.DurationMs, job.ID,
	)
	return err
}

func (r *TranscodeJobRepository) ListActive(ctx context.Context) ([]*transcoding.TranscodeJob, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms
		 FROM transcode_sessions WHERE status IN ('queued', 'running')
		 ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanJobRows(rows)
}

func (r *TranscodeJobRepository) ListByMediaItem(ctx context.Context, mediaItemID string) ([]*transcoding.TranscodeJob, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms
		 FROM transcode_sessions WHERE media_item_id = $1
		 ORDER BY started_at DESC`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanJobRows(rows)
}

func (r *TranscodeJobRepository) scanJob(row *sql.Row) (*transcoding.TranscodeJob, error) {
	job := &transcoding.TranscodeJob{}
	err := row.Scan(
		&job.ID, &job.MediaItemID, &job.InputCodec, &job.OutputCodec, &job.HWAccelType, &job.Preset,
		&job.ResolutionIn, &job.ResolutionOut, &job.BitrateIn, &job.BitrateOut, &job.Speed,
		&job.Status, &job.ErrorMessage, &job.StartedAt, &job.EndedAt, &job.DurationMs,
	)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (r *TranscodeJobRepository) scanJobRows(rows *sql.Rows) ([]*transcoding.TranscodeJob, error) {
	var result []*transcoding.TranscodeJob
	for rows.Next() {
		job := &transcoding.TranscodeJob{}
		if err := rows.Scan(
			&job.ID, &job.MediaItemID, &job.InputCodec, &job.OutputCodec, &job.HWAccelType, &job.Preset,
			&job.ResolutionIn, &job.ResolutionOut, &job.BitrateIn, &job.BitrateOut, &job.Speed,
			&job.Status, &job.ErrorMessage, &job.StartedAt, &job.EndedAt, &job.DurationMs,
		); err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

var _ transcoding.TranscodeJobRepository = (*TranscodeJobRepository)(nil)
