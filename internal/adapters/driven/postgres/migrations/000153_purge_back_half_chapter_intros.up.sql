-- An mkv chapter named "intro" in the back half of the file is the credits
-- roll (or a next-episode teaser), not a title sequence: the chapter pass has
-- classified it as end_credits since 2026-08-05, but rows stored before that
-- guard were never revisited - a chapter-sourced intro counts as covered by
-- the sweep. Prod 2026-08-29: six episodes of one show served a skip-intro
-- button over their last 34 seconds. Purge the class; the episodes re-enter
-- the work list and the guarded chapter pass re-judges them.
DELETE FROM media_segments s
USING media_files f
WHERE f.media_item_id = s.media_item_id
  AND s.type = 'intro'
  AND s.source = 'mkv_chapter'
  AND f.duration_ms > 0
  AND s.start_ms > f.duration_ms / 2;
