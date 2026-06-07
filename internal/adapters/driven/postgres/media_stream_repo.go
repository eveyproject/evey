package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
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
		hdr_type, channels, channel_layout, sample_rate, bitrate, is_external, external_path, level) VALUES `)

	args := make([]any, 0, len(streams)*24)
	for i, s := range streams {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 24
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
			base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19,
			base+20, base+21, base+22, base+23, base+24)
		args = append(args,
			s.ID, s.MediaItemID, s.StreamIndex, s.StreamType, s.Codec, s.CodecLongName,
			s.Profile, s.Language, s.Title, s.IsDefault, s.IsForced, s.Width, s.Height,
			s.Framerate, s.BitDepth, s.ColorSpace, s.HDRType, s.Channels, s.ChannelLayout,
			s.SampleRate, s.Bitrate, s.IsExternal, s.ExternalPath, s.Level,
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
		hdr_type, channels, channel_layout, sample_rate, bitrate, is_external, external_path, level) VALUES `)

	args := make([]any, 0, len(streams)*24)
	for i, s := range streams {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 24
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
			base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19,
			base+20, base+21, base+22, base+23, base+24)
		args = append(args,
			s.ID, s.MediaItemID, s.StreamIndex, s.StreamType, s.Codec, s.CodecLongName,
			s.Profile, s.Language, s.Title, s.IsDefault, s.IsForced, s.Width, s.Height,
			s.Framerate, s.BitDepth, s.ColorSpace, s.HDRType, s.Channels, s.ChannelLayout,
			s.SampleRate, s.Bitrate, s.IsExternal, s.ExternalPath, s.Level,
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
		 channel_layout, sample_rate, bitrate, is_external, external_path, level, keyframes_ms
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
			&s.SampleRate, &s.Bitrate, &s.IsExternal, &s.ExternalPath, &s.Level, pq.Array(&s.Keyframes),
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

// ListVideoLevelMissing returns up to `limit` media item IDs that carry an
// H.264 video stream whose codec level was never resolved (level IS NULL),
// skipping the `exclude` IDs. Scope is h264 because the level only feeds the
// h264 video-copy rung; other codecs are left alone. The query is itself the
// backfill worker's resumability mechanism: as levels get filled in, items drop
// out, so a restart re-queries only the remainder — no cursor, no full re-scan.
// `exclude` is the worker's in-run skip-set (failed / no-level items) so a stuck
// file isn't re-returned in a tight loop.
func (r *MediaStreamRepository) ListVideoLevelMissing(ctx context.Context, limit int, exclude []string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ListVideoLevelMissing")
	defer span.End()
	span.SetAttributes(attribute.Int("limit", limit), attribute.Int("exclude.count", len(exclude)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// A nil slice would make pq.Array render SQL NULL, and `x = ANY(NULL)` is NULL
	// (not false), which would filter out EVERY row — so an empty skip-set must be
	// an empty array, never NULL.
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT media_item_id FROM media_streams
		 WHERE stream_type = 'video' AND lower(codec) = 'h264' AND level IS NULL
		   AND NOT (media_item_id = ANY($1))
		 ORDER BY media_item_id
		 LIMIT $2`, pq.Array(exclude), limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return ids, nil
}

// SetVideoLevels fills the level column for the video streams of one media item
// from a fresh probe, matched by stream_index. It writes only where level IS
// NULL (idempotent, never overwrites an existing value) and NEVER deletes — so
// unlike a full ReplaceBatch a degraded/empty probe cannot wipe the existing
// stream metadata. Returns the number of rows updated (0 ⇒ probe resolved no new
// level, the caller leaves the item NULL and marks it to avoid a re-probe loop).
func (r *MediaStreamRepository) SetVideoLevels(ctx context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.SetVideoLevels")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	updated := 0
	for _, s := range streams {
		if s == nil || s.StreamType != library.Video || s.Level == nil {
			continue
		}
		res, err := r.db.ExecContext(ctx,
			`UPDATE media_streams SET level = $1
			 WHERE media_item_id = $2 AND stream_index = $3 AND stream_type = 'video' AND level IS NULL`,
			*s.Level, string(mediaItemID), s.StreamIndex)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return updated, err
		}
		n, _ := res.RowsAffected()
		updated += int(n)
	}
	return updated, nil
}

// ListVideoKeyframesMissing returns up to `limit` media item IDs that carry an
// H.264 video stream WITH a known level but WITHOUT a keyframe index
// (keyframes_ms IS NULL), skipping the `exclude` IDs. Gated on level IS NOT NULL
// because keyframes only matter for a copy-eligible source (CopyRungEligible
// needs the level), so this naturally runs after the level backfill. Like
// ListVideoLevelMissing, the query is the keyframe backfill worker's
// resumability mechanism (it shrinks as keyframes get filled).
func (r *MediaStreamRepository) ListVideoKeyframesMissing(ctx context.Context, limit int, exclude []string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ListVideoKeyframesMissing")
	defer span.End()
	span.SetAttributes(attribute.Int("limit", limit), attribute.Int("exclude.count", len(exclude)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT media_item_id FROM media_streams
		 WHERE stream_type = 'video' AND lower(codec) = 'h264'
		   AND level IS NOT NULL AND keyframes_ms IS NULL
		   AND NOT (media_item_id = ANY($1))
		 ORDER BY media_item_id
		 LIMIT $2`, pq.Array(exclude), limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return ids, nil
}

// SetKeyframes stores the keyframe index (ms) on the video stream(s) of one
// media item. The probe reads the first video stream (ffmpeg v:0) — the only one
// the copy rung serves — so it targets stream_type='video'; on the rare
// multi-video file every video row gets v:0's index, harmless since secondary
// video streams are never served as copy. Writes only where keyframes_ms IS NULL
// (idempotent, never overwrites) and NEVER deletes. An empty slice is skipped (a
// probe that resolved none must NOT write an empty array — it would read back as
// "indexed, zero keyframes" and wrongly satisfy the not-null candidate query).
// Returns the number of rows updated.
func (r *MediaStreamRepository) SetKeyframes(ctx context.Context, mediaItemID shared.MediaItemID, keyframesMS []int64) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.SetKeyframes")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.Int("keyframes", len(keyframesMS)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(keyframesMS) == 0 {
		return 0, nil
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE media_streams SET keyframes_ms = $1
		 WHERE media_item_id = $2 AND stream_type = 'video' AND keyframes_ms IS NULL`,
		pq.Array(keyframesMS), string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

var _ library.MediaStreamRepository = (*MediaStreamRepository)(nil)
