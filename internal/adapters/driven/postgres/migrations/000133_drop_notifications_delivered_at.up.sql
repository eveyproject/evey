-- delivered_at was never written by any code path (always NULL) and nothing
-- read it: IsDelivered() had no caller. Web push delivery is fire-and-forget;
-- per-notification delivery tracking was an FCM-era design that never shipped.
ALTER TABLE notifications DROP COLUMN IF EXISTS delivered_at;
