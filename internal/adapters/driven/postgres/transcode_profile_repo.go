package postgres

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type TranscodeProfileRepository struct {
	db *sql.DB
}

func NewTranscodeProfileRepository(db *sql.DB) *TranscodeProfileRepository {
	return &TranscodeProfileRepository{db: db}
}

func (r *TranscodeProfileRepository) GetByID(ctx context.Context, id string) (*transcoding.TranscodeProfile, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_profile.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_profile.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	p, err := r.scanProfile(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset
		 FROM transcode_profiles WHERE id = $1`, id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return p, nil
}

func (r *TranscodeProfileRepository) List(ctx context.Context) ([]*transcoding.TranscodeProfile, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_profile.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset
		 FROM transcode_profiles ORDER BY name LIMIT 100`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*transcoding.TranscodeProfile
	for rows.Next() {
		p := &transcoding.TranscodeProfile{}
		if err := rows.Scan(&p.ID, &p.Name, &p.VideoCodec, &p.AudioCodec, &p.Container,
			&p.MaxWidth, &p.MaxHeight, &p.VideoBitrate, &p.AudioBitrate, &p.Preset); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *TranscodeProfileRepository) Create(ctx context.Context, profile *transcoding.TranscodeProfile) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_profile.Create")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_profile.id", profile.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO transcode_profiles (id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		profile.ID, profile.Name, profile.VideoCodec, profile.AudioCodec, profile.Container,
		profile.MaxWidth, profile.MaxHeight, profile.VideoBitrate, profile.AudioBitrate, profile.Preset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *TranscodeProfileRepository) Update(ctx context.Context, profile *transcoding.TranscodeProfile) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_profile.Update")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_profile.id", profile.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE transcode_profiles SET name=$2, video_codec=$3, audio_codec=$4, container=$5, max_width=$6, max_height=$7, video_bitrate=$8, audio_bitrate=$9, preset=$10 WHERE id=$1`,
		profile.ID, profile.Name, profile.VideoCodec, profile.AudioCodec, profile.Container,
		profile.MaxWidth, profile.MaxHeight, profile.VideoBitrate, profile.AudioBitrate, profile.Preset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *TranscodeProfileRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.transcode_profile.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("transcode_profile.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM transcode_profiles WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *TranscodeProfileRepository) scanProfile(row *sql.Row) (*transcoding.TranscodeProfile, error) {
	p := &transcoding.TranscodeProfile{}
	err := row.Scan(&p.ID, &p.Name, &p.VideoCodec, &p.AudioCodec, &p.Container,
		&p.MaxWidth, &p.MaxHeight, &p.VideoBitrate, &p.AudioBitrate, &p.Preset)
	if err != nil {
		return nil, err
	}
	return p, nil
}

var _ transcoding.TranscodeProfileRepository = (*TranscodeProfileRepository)(nil)
