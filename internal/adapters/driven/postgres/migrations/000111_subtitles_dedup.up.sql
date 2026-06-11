-- A double-click on "download subtitle" inserted two identical rows (fresh
-- id each call, no natural unique). The natural key is (media_item_id,
-- file_path): the provider download lands on the same on-disk file, so a
-- repeat must upsert the existing row, not duplicate it.
-- Dedup existing duplicates first (keep the oldest id), then enforce.
DELETE FROM downloaded_subtitles a USING downloaded_subtitles b
 WHERE a.media_item_id = b.media_item_id
   AND a.file_path = b.file_path
   AND a.id > b.id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_downloaded_subtitles_media_path
    ON downloaded_subtitles(media_item_id, file_path);
