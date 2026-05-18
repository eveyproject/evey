package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type MediaStreamRepository struct {
	db *sql.DB
}

func NewMediaStreamRepository(db *sql.DB) *MediaStreamRepository {
	return &MediaStreamRepository{db: db}
}

func (r *MediaStreamRepository) CreateBatch(ctx context.Context, streams []*library.MediaStream) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.CreateBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("media_stream.count", len(streams)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(streams) == 0 {
		return nil
	}
	for _, s := range streams {
		if err := s.Validate(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO media_streams (id, media_item_id, stream_index, stream_type, codec, codec_long_name,
		profile, language, title, is_default, is_forced, width, height, framerate, bit_depth, color_space,
		hdr_type, channels, channel_layout, sample_rate, bitrate, is_external, external_path) VALUES `)

	args := make([]any, 0, len(streams)*23)
	for i, s := range streams {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 23
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
			base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19,
			base+20, base+21, base+22, base+23)
		args = append(args,
			s.ID, s.MediaItemID, s.StreamIndex, s.StreamType, s.Codec, s.CodecLongName,
			s.Profile, s.Language, s.Title, s.IsDefault, s.IsForced, s.Width, s.Height,
			s.Framerate, s.BitDepth, s.ColorSpace, s.HDRType, s.Channels, s.ChannelLayout,
			s.SampleRate, s.Bitrate, s.IsExternal, s.ExternalPath,
		)
	}

	_, err := r.db.ExecContext(ctx, b.String(), args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaStreamRepository) ReplaceBatch(ctx context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ReplaceBatch")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.Int("media_stream.count", len(streams)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaStreamRepo.ReplaceBatch begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM media_streams WHERE media_item_id = $1`, string(mediaItemID)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaStreamRepo.ReplaceBatch delete: %w", err)
	}

	if len(streams) == 0 {
		if err := tx.Commit(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		return nil
	}

	for _, s := range streams {
		if err := s.Validate(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO media_streams (id, media_item_id, stream_index, stream_type, codec, codec_long_name,
		profile, language, title, is_default, is_forced, width, height, framerate, bit_depth, color_space,
		hdr_type, channels, channel_layout, sample_rate, bitrate, is_external, external_path) VALUES `)

	args := make([]any, 0, len(streams)*23)
	for i, s := range streams {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 23
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
			base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19,
			base+20, base+21, base+22, base+23)
		args = append(args,
			s.ID, s.MediaItemID, s.StreamIndex, s.StreamType, s.Codec, s.CodecLongName,
			s.Profile, s.Language, s.Title, s.IsDefault, s.IsForced, s.Width, s.Height,
			s.Framerate, s.BitDepth, s.ColorSpace, s.HDRType, s.Channels, s.ChannelLayout,
			s.SampleRate, s.Bitrate, s.IsExternal, s.ExternalPath,
		)
	}

	if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MediaStreamRepo.ReplaceBatch insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaStreamRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaStream, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, stream_index, stream_type, codec, codec_long_name, profile, language, title,
		 is_default, is_forced, width, height, framerate, bit_depth, color_space, hdr_type, channels,
		 channel_layout, sample_rate, bitrate, is_external, external_path
		 FROM media_streams WHERE media_item_id = $1 ORDER BY stream_index`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaStream
	for rows.Next() {
		s := &library.MediaStream{}
		if err := rows.Scan(
			&s.ID, &s.MediaItemID, &s.StreamIndex, &s.StreamType, &s.Codec, &s.CodecLongName,
			&s.Profile, &s.Language, &s.Title, &s.IsDefault, &s.IsForced, &s.Width, &s.Height,
			&s.Framerate, &s.BitDepth, &s.ColorSpace, &s.HDRType, &s.Channels, &s.ChannelLayout,
			&s.SampleRate, &s.Bitrate, &s.IsExternal, &s.ExternalPath,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaStreamRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_streams WHERE media_item_id = $1`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ library.MediaStreamRepository = (*MediaStreamRepository)(nil)
