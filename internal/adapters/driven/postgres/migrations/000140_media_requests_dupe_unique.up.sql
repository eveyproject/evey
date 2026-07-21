-- Make the duplicate guard real.
--
-- 000134 created idx_media_requests_dupe and its comment called it a "duplicate
-- guard", but the index is not UNIQUE - it only made the lookup fast. The
-- service reads (HasOpenRequest) and then writes, with nothing in between and
-- no transaction, so two concurrent calls - a double-click, a client retry -
-- both pass the check and both insert. The operator's moderation queue then
-- holds two identical open requests and nothing at the database level ever said
-- no.
--
-- "Open" means "not rejected", matching HasOpenRequest: a rejected request must
-- not block asking again, whereas pending/approved/later all should.

-- Existing duplicates would make the unique index fail to build, so collapse
-- them first, keeping the OLDEST row of each group - it carries the original
-- request date and any admin note already attached to it.
DELETE FROM media_requests a
USING media_requests b
WHERE a.status <> 'rejected'
  AND b.status <> 'rejected'
  AND a.profile_id = b.profile_id
  AND a.media_type = b.media_type
  AND a.provider_id = b.provider_id
  AND a.provider_id <> ''
  AND (a.created_at > b.created_at OR (a.created_at = b.created_at AND a.id > b.id));

-- Partial unique index: only open requests are constrained, and only rows that
-- actually name a provider (provider_id = '' is the free-text request path,
-- which has no identity to deduplicate on).
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_requests_dupe_unique
    ON media_requests (profile_id, media_type, provider_id)
    WHERE status <> 'rejected' AND provider_id <> '';
