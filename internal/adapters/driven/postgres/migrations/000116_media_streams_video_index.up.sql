-- Partial index on video streams to speed the transcode backfill scans
-- (ListVideoLevelMissing / ListVideoKeyframesMissing). They filter
-- stream_type = 'video' then assorted codec/level/color_transfer/idet/keyframe
-- conditions and ORDER BY media_item_id — with only the media_item_id index
-- they seq-scanned the whole media_streams table (video + audio + subtitle,
-- ~3x the rows) on every backfill pass. Restricting the index to video rows
-- turns that into an index scan over the relevant subset.
CREATE INDEX IF NOT EXISTS idx_media_streams_video
    ON media_streams (media_item_id)
    WHERE stream_type = 'video';
