package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

type TranscodeProfileRepository struct {
	db *sql.DB
}

func NewTranscodeProfileRepository(db *sql.DB) *TranscodeProfileRepository {
	return &TranscodeProfileRepository{db: db}
}

func (r *TranscodeProfileRepository) GetByID(ctx context.Context, id string) (*transcoding.TranscodeProfile, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanProfile(r.db.QueryRowContext(ctx,
		`SELECT id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset
		 FROM transcode_profiles WHERE id = $1`, id))
}

func (r *TranscodeProfileRepository) List(ctx context.Context) ([]*transcoding.TranscodeProfile, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset
		 FROM transcode_profiles ORDER BY name LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*transcoding.TranscodeProfile
	for rows.Next() {
		p := &transcoding.TranscodeProfile{}
		if err := rows.Scan(&p.ID, &p.Name, &p.VideoCodec, &p.AudioCodec, &p.Container,
			&p.MaxWidth, &p.MaxHeight, &p.VideoBitrate, &p.AudioBitrate, &p.Preset); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *TranscodeProfileRepository) Create(ctx context.Context, profile *transcoding.TranscodeProfile) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO transcode_profiles (id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		profile.ID, profile.Name, profile.VideoCodec, profile.AudioCodec, profile.Container,
		profile.MaxWidth, profile.MaxHeight, profile.VideoBitrate, profile.AudioBitrate, profile.Preset,
	)
	return err
}

func (r *TranscodeProfileRepository) Update(ctx context.Context, profile *transcoding.TranscodeProfile) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE transcode_profiles SET name=$2, video_codec=$3, audio_codec=$4, container=$5, max_width=$6, max_height=$7, video_bitrate=$8, audio_bitrate=$9, preset=$10 WHERE id=$1`,
		profile.ID, profile.Name, profile.VideoCodec, profile.AudioCodec, profile.Container,
		profile.MaxWidth, profile.MaxHeight, profile.VideoBitrate, profile.AudioBitrate, profile.Preset,
	)
	return err
}

func (r *TranscodeProfileRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM transcode_profiles WHERE id = $1`, id)
	return err
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
