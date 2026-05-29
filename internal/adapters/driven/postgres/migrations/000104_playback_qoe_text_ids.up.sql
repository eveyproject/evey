-- The 103 migration declared user_id and media_item_id as UUID, but Evey's
-- ID scheme is a TEXT-prefixed slug ("usr-...", "lib-...", media items are
-- also TEXT). Every insert from the QoE sink was returning
-- "invalid input syntax for type uuid (22P02)" and getting swallowed by
-- the sink's err-log path. Fix the column types so rows actually land.
ALTER TABLE playback_qoe_events
    ALTER COLUMN user_id       TYPE TEXT USING user_id::text,
    ALTER COLUMN media_item_id TYPE TEXT USING media_item_id::text;
