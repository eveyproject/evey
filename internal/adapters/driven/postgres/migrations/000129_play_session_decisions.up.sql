-- Per-track transcode decisions for the "Now Playing" admin panel. The coarse
-- play_method ('transcode') hides that video may be copied (copy rung) while
-- only audio is re-encoded. Storing the video/audio decisions lets the panel
-- distinguish copy-rung vs a true transcode, and derive the serving engine
-- (mkvgo copy rung vs ffmpeg transcode).
ALTER TABLE play_sessions
  ADD COLUMN video_decision TEXT NOT NULL DEFAULT '',
  ADD COLUMN audio_decision TEXT NOT NULL DEFAULT '';
