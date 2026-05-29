ALTER TABLE playback_qoe_events
    ALTER COLUMN user_id       TYPE UUID USING user_id::uuid,
    ALTER COLUMN media_item_id TYPE UUID USING media_item_id::uuid;
