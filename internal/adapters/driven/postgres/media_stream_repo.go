package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type MediaStreamRepository struct {
	db *sql.DB
}

func NewMediaStreamRepository(db *sql.DB) *MediaStreamRepository {
	return &MediaStreamRepository{db: db}
}

func (r *MediaStreamRepository) CreateBatch(ctx context.Context, streams []*library.MediaStream) error {
	if len(streams) == 0 {
		return nil
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
	return err
}

func (r *MediaStreamRepository) GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, stream_index, stream_type, codec, codec_long_name, profile, language, title,
		 is_default, is_forced, width, height, framerate, bit_depth, color_space, hdr_type, channels,
		 channel_layout, sample_rate, bitrate, is_external, external_path
		 FROM media_streams WHERE media_item_id = $1 ORDER BY stream_index`, mediaItemID)
	if err != nil {
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
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *MediaStreamRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_streams WHERE media_item_id = $1`, mediaItemID)
	return err
}

var _ library.MediaStreamRepository = (*MediaStreamRepository)(nil)
