-- Trickplay generation failures were never persisted: the background sweep
-- (boot + worker ticks) lists items with no manifest, so a file ffmpeg/ffprobe
-- cannot process was re-attempted at EVERY boot, forever (prod 2026-07-26:
-- the same two files failing on each restart).
--
-- One row = "generation failed against this exact file". The missing-list
-- skips an item while its media_files row still matches the recorded
-- (file_size, mod_time) pair: replacing or re-downloading the file changes the
-- pair and the item re-enters the candidates naturally. A successful
-- generation deletes the row (Create clears it).
CREATE TABLE IF NOT EXISTS trickplay_failures (
    media_item_id TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    file_size     BIGINT NOT NULL,
    mod_time      TIMESTAMPTZ NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    failed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
