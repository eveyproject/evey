package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
		profile, language, title, is_default, is_forced, width, height, framerate, bit_depth, pixel_format, color_space,
		color_transfer, hdr_type, dovi_profile, dovi_bl_compat, field_order, idet_interlaced, idet_telecine,
		sample_aspect_ratio, channels, channel_layout, sample_rate,
		bitrate, is_external, external_path, level,
		commentary, hearing_impaired, visual_impaired, text_descriptions, original, frame_rate_mode) VALUES `)

	args := make([]any, 0, len(streams)*38)
	for i, s := range streams {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 38
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
			base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19,
			base+20, base+21, base+22, base+23, base+24, base+25, base+26, base+27, base+28, base+29, base+30,
			base+31, base+32, base+33, base+34, base+35, base+36, base+37, base+38)
		args = append(args,
			s.ID, s.MediaItemID, s.StreamIndex, s.StreamType, s.Codec, s.CodecLongName,
			s.Profile, s.Language, s.Title, s.IsDefault, s.IsForced, s.Width, s.Height,
			s.Framerate, s.BitDepth, s.PixelFormat, s.ColorSpace, s.ColorTransfer, s.HDRType,
			s.DoViProfile, s.DoViBLCompat, s.FieldOrder, s.IdetInterlaced, s.IdetTelecine,
			s.SampleAspectRatio, s.Channels, s.ChannelLayout,
			s.SampleRate, s.Bitrate, s.IsExternal, s.ExternalPath, s.Level,
			s.Commentary, s.HearingImpaired, s.VisualImpaired, s.TextDescriptions, s.Original, s.FrameRateMode,
		)
	}

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, b.String(), args...)
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
		profile, language, title, is_default, is_forced, width, height, framerate, bit_depth, pixel_format, color_space,
		color_transfer, hdr_type, dovi_profile, dovi_bl_compat, field_order, idet_interlaced, idet_telecine,
		sample_aspect_ratio, channels, channel_layout, sample_rate,
		bitrate, is_external, external_path, level,
		commentary, hearing_impaired, visual_impaired, text_descriptions, original, frame_rate_mode) VALUES `)

	args := make([]any, 0, len(streams)*38)
	for i, s := range streams {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 38
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
			base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19,
			base+20, base+21, base+22, base+23, base+24, base+25, base+26, base+27, base+28, base+29, base+30,
			base+31, base+32, base+33, base+34, base+35, base+36, base+37, base+38)
		args = append(args,
			s.ID, s.MediaItemID, s.StreamIndex, s.StreamType, s.Codec, s.CodecLongName,
			s.Profile, s.Language, s.Title, s.IsDefault, s.IsForced, s.Width, s.Height,
			s.Framerate, s.BitDepth, s.PixelFormat, s.ColorSpace, s.ColorTransfer, s.HDRType,
			s.DoViProfile, s.DoViBLCompat, s.FieldOrder, s.IdetInterlaced, s.IdetTelecine,
			s.SampleAspectRatio, s.Channels, s.ChannelLayout,
			s.SampleRate, s.Bitrate, s.IsExternal, s.ExternalPath, s.Level,
			s.Commentary, s.HearingImpaired, s.VisualImpaired, s.TextDescriptions, s.Original, s.FrameRateMode,
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
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, stream_index, stream_type, codec, codec_long_name, profile, language, title,
		 is_default, is_forced, width, height, framerate, bit_depth, pixel_format, color_space, color_transfer, hdr_type,
		 dovi_profile, dovi_bl_compat, field_order, idet_interlaced, idet_telecine, sample_aspect_ratio, channels,
		 channel_layout, sample_rate, bitrate, is_external, external_path, level, keyframes_ms,
		 commentary, hearing_impaired, visual_impaired, text_descriptions, original, frame_rate_mode
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
			&s.Framerate, &s.BitDepth, &s.PixelFormat, &s.ColorSpace, &s.ColorTransfer, &s.HDRType,
			&s.DoViProfile, &s.DoViBLCompat, &s.FieldOrder, &s.IdetInterlaced, &s.IdetTelecine,
			&s.SampleAspectRatio, &s.Channels, &s.ChannelLayout,
			&s.SampleRate, &s.Bitrate, &s.IsExternal, &s.ExternalPath, &s.Level, pq.Array(&s.Keyframes),
			&s.Commentary, &s.HearingImpaired, &s.VisualImpaired, &s.TextDescriptions, &s.Original, &s.FrameRateMode,
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

func (r *MediaStreamRepository) GetByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]*library.MediaStream, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.GetByMediaItemIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	if len(mediaItemIDs) == 0 {
		return map[shared.MediaItemID][]*library.MediaStream{}, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	ids := make([]string, 0, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		ids = append(ids, string(id))
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, stream_index, stream_type, codec, codec_long_name, profile, language, title,
		 is_default, is_forced, width, height, framerate, bit_depth, pixel_format, color_space, color_transfer, hdr_type,
		 dovi_profile, dovi_bl_compat, field_order, idet_interlaced, idet_telecine, sample_aspect_ratio, channels,
		 channel_layout, sample_rate, bitrate, is_external, external_path, level,
		 commentary, hearing_impaired, visual_impaired, text_descriptions, original, frame_rate_mode
		 FROM media_streams WHERE media_item_id = ANY($1) ORDER BY media_item_id, stream_index`, pq.Array(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[shared.MediaItemID][]*library.MediaStream, len(mediaItemIDs))
	for rows.Next() {
		s := &library.MediaStream{}
		if err := rows.Scan(
			&s.ID, &s.MediaItemID, &s.StreamIndex, &s.StreamType, &s.Codec, &s.CodecLongName,
			&s.Profile, &s.Language, &s.Title, &s.IsDefault, &s.IsForced, &s.Width, &s.Height,
			&s.Framerate, &s.BitDepth, &s.PixelFormat, &s.ColorSpace, &s.ColorTransfer, &s.HDRType,
			&s.DoViProfile, &s.DoViBLCompat, &s.FieldOrder, &s.IdetInterlaced, &s.IdetTelecine,
			&s.SampleAspectRatio, &s.Channels, &s.ChannelLayout,
			&s.SampleRate, &s.Bitrate, &s.IsExternal, &s.ExternalPath, &s.Level,
			&s.Commentary, &s.HearingImpaired, &s.VisualImpaired, &s.TextDescriptions, &s.Original, &s.FrameRateMode,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[s.MediaItemID] = append(result[s.MediaItemID], s)
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
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_streams WHERE media_item_id = $1`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// ListVideoLevelMissing returns up to `limit` media item IDs whose video
// stream is missing copy-rung metadata, skipping the `exclude` IDs:
//   - h264: codec level never resolved (level IS NULL);
//   - hevc/av1/vp9 (the HDR-capable codecs): level IS NULL, OR color_transfer
//     IS NULL - the HDR ground-truth sentinel. color_transfer only fills
//     post-migration-000110, so a NULL means "never probed for DoVi": such a
//     stream is gated off the copy rung (a Dolby Vision profile-5 title
//     recorded as plain HDR10 would raw-serve with broken colours) until this
//     backfill re-probes it.
//
// Scope stays h264+hevc because the metadata only feeds the video-copy rung;
// other codecs are left alone. The query is itself the backfill worker's
// resumability mechanism: as the columns fill in, items drop out, so a restart
// re-queries only the remainder - no cursor, no full re-scan. `exclude` is the
// worker's in-run skip-set (failed / nothing-resolved items) so a stuck file
// isn't re-returned in a tight loop.
func (r *MediaStreamRepository) ListVideoLevelMissing(ctx context.Context, limit int, exclude []string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ListVideoLevelMissing")
	defer span.End()
	span.SetAttributes(attribute.Int("limit", limit), attribute.Int("exclude.count", len(exclude)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// A nil slice would make pq.Array render SQL NULL, and `x = ANY(NULL)` is NULL
	// (not false), which would filter out EVERY row - so an empty skip-set must be
	// an empty array, never NULL.
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT DISTINCT media_item_id FROM media_streams
		 WHERE `+levelEligibleSQL+`
		   AND `+levelPendingSQL+`
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

// SetVideoLevels fills the copy-rung metadata columns (level, color_transfer,
// dovi_profile, dovi_bl_compat) for the video streams of one media item from a
// fresh probe, matched by stream_index. Each column is written only where it
// IS NULL (COALESCE keeps existing values; idempotent) and the row is touched
// only when the probe actually resolves something new - so unlike a full
// ReplaceBatch a degraded/empty probe cannot wipe the existing stream
// metadata. Returns the number of rows updated (0 ⇒ probe resolved nothing
// new, the caller marks the item to avoid a re-probe loop).
func (r *MediaStreamRepository) SetVideoLevels(ctx context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.SetVideoLevels")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	updated := 0
	for _, s := range streams {
		if s == nil || s.StreamType != library.Video {
			continue
		}
		if s.Level == nil && s.ColorTransfer == nil && s.DoViProfile == nil &&
			s.FieldOrder == nil && s.SampleAspectRatio == nil {
			continue // probe resolved nothing this backfill can fill
		}
		res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
			`UPDATE media_streams SET
			   level = COALESCE(level, $1),
			   color_transfer = COALESCE(color_transfer, $2),
			   dovi_profile = COALESCE(dovi_profile, $3),
			   dovi_bl_compat = COALESCE(dovi_bl_compat, $4),
			   field_order = COALESCE(field_order, $5),
			   sample_aspect_ratio = COALESCE(sample_aspect_ratio, $6)
			 WHERE media_item_id = $7 AND stream_index = $8 AND stream_type = 'video'
			   AND ((level IS NULL AND $1::int IS NOT NULL)
			     OR (color_transfer IS NULL AND $2::text IS NOT NULL)
			     OR (dovi_profile IS NULL AND $3::smallint IS NOT NULL)
			     OR (field_order IS NULL AND $5::text IS NOT NULL)
			     OR (sample_aspect_ratio IS NULL AND $6::text IS NOT NULL))`,
			s.Level, s.ColorTransfer, s.DoViProfile, s.DoViBLCompat,
			s.FieldOrder, s.SampleAspectRatio,
			string(mediaItemID), s.StreamIndex)
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

// MarkVideoLevelProbed stamps the item's video streams with the level probe
// version at which a probe of the file succeeded, whether or not it resolved
// anything, so a stream the prober never fills stops being a candidate (see
// levelPendingSQL). Returns rows updated.
func (r *MediaStreamRepository) MarkVideoLevelProbed(ctx context.Context, mediaItemID shared.MediaItemID, probeVersion int) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.MarkVideoLevelProbed")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.Int("probe_version", probeVersion))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_streams SET level_probe_version = $1
		 WHERE media_item_id = $2 AND stream_type = 'video' AND level_probe_version < $1`,
		probeVersion, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetIdetTelecine stores the idet telecine verdict on the item's video stream.
func (r *MediaStreamRepository) SetIdetTelecine(ctx context.Context, mediaItemID shared.MediaItemID, telecine bool) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.SetIdetTelecine")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_streams SET idet_telecine = $1
		 WHERE media_item_id = $2 AND stream_type = 'video'`,
		telecine, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetIdetInterlaced stores the idet content-detection verdict on the item's
// video stream(s). Unlike the other backfill columns it writes a DEFINITIVE
// verdict (true OR false): the candidate query keys on idet_interlaced IS
// NULL, so a "not interlaced" result must persist as FALSE or the worker
// would re-run the (decode-heavy) probe on every pass. Never overwrites an
// existing verdict.
func (r *MediaStreamRepository) SetIdetInterlaced(ctx context.Context, mediaItemID shared.MediaItemID, interlaced bool) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.SetIdetInterlaced")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_streams SET idet_interlaced = $1
		 WHERE media_item_id = $2 AND stream_type = 'video' AND idet_interlaced IS NULL`,
		interlaced, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListVideoKeyframesMissing returns up to `limit` media item IDs that carry a
// copy-eligible video stream WITHOUT a keyframe index (keyframes_ms IS NULL),
// skipping the `exclude` IDs. The keyframe scan is the expensive probe
// (~tens of seconds per MKV), so the candidate set mirrors the copy
// eligibility gates and naturally runs after the level/HDR backfill:
//   - h264: level IS NOT NULL (CopyRungEligible needs the level);
//   - hevc/av1/vp9: level + color_transfer known (HDR ground truth probed),
//     and not Dolby Vision unless profile 8 with a compatible base layer
//     (1=HDR10, 2=SDR, 4=HLG) - a profile-5 title is never served as copy,
//     so indexing its keyframes would be pure waste.
//
// Like ListVideoLevelMissing, the query is the keyframe backfill worker's
// resumability mechanism (it shrinks as keyframes get filled). Rows stamped
// unindexable at probeVersion or later (see MarkKeyframesUnindexable) are
// settled questions and stay out until the prober version is bumped.
func (r *MediaStreamRepository) ListVideoKeyframesMissing(ctx context.Context, limit int, exclude []string, probeVersion int16) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ListVideoKeyframesMissing")
	defer span.End()
	span.SetAttributes(attribute.Int("limit", limit), attribute.Int("exclude.count", len(exclude)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT DISTINCT media_item_id FROM media_streams
		 WHERE `+keyframesEligibleSQL+`
		   AND `+keyframesPendingSQL("$3")+`
		   AND NOT (media_item_id = ANY($1))
		 ORDER BY media_item_id
		 LIMIT $2`, pq.Array(exclude), limit, probeVersion)
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
// media item. The probe reads the first video stream (ffmpeg v:0) - the only one
// the copy rung serves - so it targets stream_type='video'; on the rare
// multi-video file every video row gets v:0's index, harmless since secondary
// video streams are never served as copy. Writes only where keyframes_ms IS NULL
// (idempotent, never overwrites) and NEVER deletes. An empty slice is skipped (a
// probe that resolved none must NOT write an empty array - it would read back as
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
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
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

// MarkKeyframesUnindexable stamps the item's index-less video stream(s) with
// the prober version that settled on "no keyframe index possible" - the probe
// looked at real bytes and found nothing extractable (avi and friends), so
// retrying at the same prober version is guaranteed waste. Scoped to
// keyframes_ms IS NULL: an indexed row already left the candidate set and must
// keep its 0 stamp. A rescan of a replaced file recreates the stream rows
// (fresh 0), and a version bump re-opens stamped rows - both paths lead back
// into ListVideoKeyframesMissing naturally.
func (r *MediaStreamRepository) MarkKeyframesUnindexable(ctx context.Context, mediaItemID shared.MediaItemID, probeVersion int16) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.MarkKeyframesUnindexable")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.Int("probe_version", int(probeVersion)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_streams SET keyframes_probe_version = $1
		 WHERE media_item_id = $2 AND stream_type = 'video' AND keyframes_ms IS NULL`,
		probeVersion, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetFrameRateMode stores the cfr/vfr classification on one media item's
// video stream(s), gathered in the SAME per-file mkvgo pass the keyframe
// backfill already pays for (see the keyframe backfill worker). Unlike
// SetKeyframes, "" is a legitimate write (the classification came back
// unknown) - the guard is on the COLUMN's default rather than on the value:
// writes only where frame_rate_mode is still ” (never overwrites a prior
// classification), mirroring SetKeyframes' NULL-guard idempotency. Returns
// the number of rows updated.
//
// algoVersion stamps WHICH classifier produced the verdict, so a later
// improvement can find and re-analyze it (ReclassifyFrameRateMode). Writing it
// here matters as much as on the reclassify path: without the stamp a freshly
// classified row would read back as stale on the very next drain.
func (r *MediaStreamRepository) SetFrameRateMode(ctx context.Context, mediaItemID shared.MediaItemID, mode string, algoVersion int16) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.SetFrameRateMode")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.String("frame_rate_mode", mode))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_streams SET frame_rate_mode = $1, frame_rate_algo_version = $2
		 WHERE media_item_id = $3 AND stream_type = 'video' AND frame_rate_mode = ''`,
		mode, algoVersion, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ReclassifyFrameRateMode OVERWRITES an existing cfr/vfr verdict, which is
// exactly what SetFrameRateMode refuses to do. It is the only write that can
// undo a stale classification: a row keeps whatever the classifier of its day
// decided, so the pre-v0.25.1 false 'vfr' verdicts (a B-frame decode-order
// artifact read as a frame-duration outlier) would otherwise pin those titles
// to a forced transcode forever.
//
// An "" mode (the analyzer cannot speak for this container - non-EBML, too few
// frames) stamps the version WITHOUT touching the verdict. Both halves matter:
// dropping a possibly-correct verdict would be a regression, but skipping the
// stamp would leave the row below the current version and re-listed as a
// candidate on every single drain, so an MP4 library would be re-walked
// forever. The stamp records "the current classifier looked and had nothing to
// add".
//
// Guarded on algo_version < the caller's, so it is idempotent and cannot walk
// a verdict backwards if two drains overlap.
func (r *MediaStreamRepository) ReclassifyFrameRateMode(ctx context.Context, mediaItemID shared.MediaItemID, mode string, algoVersion int16) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ReclassifyFrameRateMode")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.String("frame_rate_mode", mode))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_streams
		 SET frame_rate_mode = CASE WHEN $1 <> '' THEN $1 ELSE frame_rate_mode END,
		     frame_rate_algo_version = $2
		 WHERE media_item_id = $3 AND stream_type = 'video' AND frame_rate_algo_version < $2`,
		mode, algoVersion, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListVideoFrameRateStale returns media item IDs whose video stream carries no
// verdict from the CURRENT classifier, skipping `exclude`. That covers both a
// stale cfr/vfr verdict AND a row never classified at all: the predicate is the
// version, not the mode.
//
// Rows sitting at frame_rate_mode = ” are NOT harmless, which is why they are
// in scope. ” fails OPEN at the copy gate (CopyRungEligible only blocks a
// confirmed 'vfr'), so a genuinely variable-rate source that was never analyzed
// is served as a -c:v copy and drifts A/V. They also never self-heal: the
// classification only ever rode along a keyframe write, so a row whose
// keyframes are already indexed would stay unclassified forever.
//
// SCOPED to copy-eligible sources, mirroring ListVideoKeyframesMissing's gates.
// The verdict has exactly ONE consumer, CopyRungEligible: on a source that can
// never be served as copy it changes no decision, so analyzing it is pure cost -
// and the cost is a full-file walk of tens of seconds. Excluding a row is safe
// because the gates are themselves self-healing: an item still missing its level
// stays at version 0 and becomes a candidate as soon as the level backfill fills
// it in.
//
// Like the other backfill queries this is the drain's resumability mechanism -
// it shrinks as rows get stamped, so an interrupted run simply resumes.
func (r *MediaStreamRepository) ListVideoFrameRateStale(ctx context.Context, limit int, algoVersion int16, exclude []string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.ListVideoFrameRateStale")
	defer span.End()
	span.SetAttributes(attribute.Int("limit", limit), attribute.Int("exclude.count", len(exclude)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT DISTINCT media_item_id FROM media_streams
		 WHERE stream_type = 'video'
		   AND frame_rate_algo_version < $1
		   AND level IS NOT NULL
		   AND (lower(codec) = 'h264'
		     OR (lower(codec) IN ('hevc', 'av1', 'vp9') AND color_transfer IS NOT NULL
		         AND (dovi_profile IS NULL
		           OR (dovi_profile = 8 AND dovi_bl_compat IN (1, 2, 4)))))
		   AND NOT (media_item_id = ANY($2))
		 ORDER BY media_item_id
		 LIMIT $3`, algoVersion, pq.Array(exclude), limit)
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

var _ library.MediaStreamRepository = (*MediaStreamRepository)(nil)

// VideoDims returns the item's video resolution, or (0,0) when it has no video
// stream. Serves the complexity worker, which passes it to the classifier so
// that ffprobe is not forked purely to re-read a number the scan already stored.
func (r *MediaStreamRepository) VideoDims(ctx context.Context, mediaItemID shared.MediaItemID) (int, int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.VideoDims")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var w, h sql.NullInt64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT width, height FROM media_streams
		  WHERE media_item_id = $1 AND stream_type = 'video'
		    AND width IS NOT NULL AND height IS NOT NULL
		  ORDER BY stream_index
		  LIMIT 1`, string(mediaItemID)).Scan(&w, &h)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		span.RecordError(err)
		return 0, 0, err
	}
	return int(w.Int64), int(h.Int64), nil
}

// RefreshProbedFields implements library.MediaStreamRepository: the probe's own
// columns are written onto the matching rows, every other column (the
// backfills' level, idet, frame-rate mode, keyframes and their versions)
// stays as it is.
func (r *MediaStreamRepository) RefreshProbedFields(ctx context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_stream.RefreshProbedFields")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields begin tx: %w", err)
	}
	defer tx.Rollback()

	type storedStream struct{ streamType, codec string }
	rows, err := tx.QueryContext(ctx, `SELECT stream_index, stream_type, codec FROM media_streams
		WHERE media_item_id = $1 AND NOT is_external FOR UPDATE`, string(mediaItemID))
	if err != nil {
		return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields read: %w", err)
	}
	defer rows.Close()
	stored := map[int]storedStream{}
	for rows.Next() {
		var idx int
		var st storedStream
		if err := rows.Scan(&idx, &st.streamType, &st.codec); err != nil {
			return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields scan: %w", err)
		}
		stored[idx] = st
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields rows: %w", err)
	}
	var probed []*library.MediaStream
	for _, s := range streams {
		if !s.IsExternal {
			probed = append(probed, s)
		}
	}
	if len(probed) != len(stored) {
		return 0, library.ErrStreamSetChanged
	}
	for _, s := range probed {
		st, ok := stored[s.StreamIndex]
		if !ok || st.streamType != string(s.StreamType) || st.codec != s.Codec {
			return 0, library.ErrStreamSetChanged
		}
	}

	changed := 0
	for _, s := range probed {
		res, err := tx.ExecContext(ctx, `UPDATE media_streams SET
			framerate = COALESCE($4, framerate), bit_depth = COALESCE($5, bit_depth),
			pixel_format = COALESCE($6, pixel_format), color_space = COALESCE($7, color_space),
			color_transfer = COALESCE($8, color_transfer), hdr_type = COALESCE($9, hdr_type),
			bitrate = COALESCE($10, bitrate), channels = COALESCE($11, channels),
			channel_layout = COALESCE($12, channel_layout)
		WHERE media_item_id = $1 AND stream_index = $2 AND stream_type = $3 AND NOT is_external
		  AND (framerate, bit_depth, pixel_format, color_space, color_transfer, hdr_type, bitrate, channels, channel_layout)
		      IS DISTINCT FROM (COALESCE($4, framerate), COALESCE($5, bit_depth), COALESCE($6, pixel_format),
		                        COALESCE($7, color_space), COALESCE($8, color_transfer), COALESCE($9, hdr_type),
		                        COALESCE($10, bitrate), COALESCE($11, channels), COALESCE($12, channel_layout))`,
			string(mediaItemID), s.StreamIndex, string(s.StreamType),
			s.Framerate, s.BitDepth, s.PixelFormat, s.ColorSpace, s.ColorTransfer, s.HDRType, s.Bitrate,
			s.Channels, s.ChannelLayout)
		if err != nil {
			return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields update: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields rows affected: %w", err)
		}
		changed += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("MediaStreamRepo.RefreshProbedFields commit: %w", err)
	}
	return changed, nil
}
