-- media_files.interleave_backstep_ms: the largest backward step of a sampled
-- cluster timestamp (see library.MediaFile.InterleaveBackstepMs). 0 = tracks
-- side by side; above the flag the file is refused for playback until a re-mux.
ALTER TABLE media_files ADD COLUMN IF NOT EXISTS interleave_backstep_ms BIGINT NOT NULL DEFAULT 0;
