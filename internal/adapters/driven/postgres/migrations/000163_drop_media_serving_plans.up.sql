-- media_serving_plans held mkvgo's scan-time ops.Ingest verdict per media
-- item, written by every scan and read by nothing: the playback decision
-- resolves from the probed streams and the client, and the scan consumes the
-- verdict in memory (seek-index repair, keyframe warm-up). Dropped.
DROP TABLE IF EXISTS media_serving_plans;
