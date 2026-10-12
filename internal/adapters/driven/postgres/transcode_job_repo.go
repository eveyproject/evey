package postgres

import (
	"context"
	"database/sql"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// durationMsArg adapts a *shared.Duration to the int64-or-NULL value the
// duration_ms column accepts. Returns nil for a nil duration.
func durationMsArg(d *shared.Duration) any {
	if d == nil {
		return nil
	}
	return d.Milliseconds()
}

// scanDurationMs converts a nullable int64 read from duration_ms into the
// domain *shared.Duration. A negative stored value would be a corrupt row;
// log and drop it (returning nil) rather than failing the whole read.
func scanDurationMs(jobID string, raw *int64) *shared.Duration {
	if raw == nil {
		return nil
	}
	d, err := shared.NewDurationMs(*raw)
	if err != nil {
		slog.Warn("invalid duration_ms in transcode_sessions row", slog.String("jobID", jobID), slog.Int64("ms", *raw), slog.Any("err", err))
		return nil
	}
	return &d
}

type TranscodeJobRepository struct {
	db *sql.DB
}

func NewTranscodeJobRepository(db *sql.DB) *TranscodeJobRepository {
	return &TranscodeJobRepository{db: db}
}

func (r *TranscodeJobRepository) Create(ctx context.Context, job *transcoding.TranscodeJob) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcodejob.Create")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_job.id", job.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO transcode_sessions (id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		job.ID, job.MediaItemID, job.InputCodec, job.OutputCodec, job.HWAccelType, job.Preset,
		job.ResolutionIn, job.ResolutionOut, job.BitrateIn, job.BitrateOut, job.Speed,
		job.Status, job.ErrorMessage, job.StartedAt, job.EndedAt, durationMsArg(job.Duration),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (r *TranscodeJobRepository) GetByID(ctx context.Context, id string) (*transcoding.TranscodeJob, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcodejob.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_job.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	job, err := r.scanJob(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms
		 FROM transcode_sessions WHERE id = $1`, id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return job, err
}

func (r *TranscodeJobRepository) Update(ctx context.Context, job *transcoding.TranscodeJob) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcodejob.Update")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_job.id", job.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE transcode_sessions SET media_item_id=$1, input_codec=$2, output_codec=$3, hw_accel_type=$4,
		 preset=$5, resolution_in=$6, resolution_out=$7, bitrate_in=$8, bitrate_out=$9, speed=$10,
		 status=$11, error_message=$12, started_at=$13, ended_at=$14, duration_ms=$15
		 WHERE id=$16`,
		job.MediaItemID, job.InputCodec, job.OutputCodec, job.HWAccelType,
		job.Preset, job.ResolutionIn, job.ResolutionOut, job.BitrateIn, job.BitrateOut, job.Speed,
		job.Status, job.ErrorMessage, job.StartedAt, job.EndedAt, durationMsArg(job.Duration), job.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (r *TranscodeJobRepository) ListActive(ctx context.Context) ([]*transcoding.TranscodeJob, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcodejob.ListActive")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms
		 FROM transcode_sessions WHERE status IN ('queued', 'running')
		 ORDER BY started_at`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	jobs, err := r.scanJobRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return jobs, err
}

func (r *TranscodeJobRepository) ListByMediaItem(ctx context.Context, mediaItemID string) ([]*transcoding.TranscodeJob, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcodejob.ListByMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, input_codec, output_codec, hw_accel_type, preset,
		 resolution_in, resolution_out, bitrate_in, bitrate_out, speed, status, error_message,
		 started_at, ended_at, duration_ms
		 FROM transcode_sessions WHERE media_item_id = $1
		 ORDER BY started_at DESC LIMIT 100`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	jobs, err := r.scanJobRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return jobs, err
}

func (r *TranscodeJobRepository) scanJob(row *sql.Row) (*transcoding.TranscodeJob, error) {
	job := &transcoding.TranscodeJob{}
	var durationMs *int64
	err := row.Scan(
		&job.ID, &job.MediaItemID, &job.InputCodec, &job.OutputCodec, &job.HWAccelType, &job.Preset,
		&job.ResolutionIn, &job.ResolutionOut, &job.BitrateIn, &job.BitrateOut, &job.Speed,
		&job.Status, &job.ErrorMessage, &job.StartedAt, &job.EndedAt, &durationMs,
	)
	if err != nil {
		return nil, err
	}
	job.Duration = scanDurationMs(job.ID, durationMs)
	return job, nil
}

func (r *TranscodeJobRepository) scanJobRows(rows *sql.Rows) ([]*transcoding.TranscodeJob, error) {
	var result []*transcoding.TranscodeJob
	for rows.Next() {
		job := &transcoding.TranscodeJob{}
		var durationMs *int64
		if err := rows.Scan(
			&job.ID, &job.MediaItemID, &job.InputCodec, &job.OutputCodec, &job.HWAccelType, &job.Preset,
			&job.ResolutionIn, &job.ResolutionOut, &job.BitrateIn, &job.BitrateOut, &job.Speed,
			&job.Status, &job.ErrorMessage, &job.StartedAt, &job.EndedAt, &durationMs,
		); err != nil {
			return nil, err
		}
		job.Duration = scanDurationMs(job.ID, durationMs)
		result = append(result, job)
	}
	return result, rows.Err()
}

var _ transcoding.TranscodeJobRepository = (*TranscodeJobRepository)(nil)
