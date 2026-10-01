-- Opening hours of a kids profile (rules of weekdays and time ranges, JSON),
-- and how long a parent's PIN keeps a closed profile open. NULL = none.
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS access_schedule JSONB;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS schedule_override_until TIMESTAMPTZ;
