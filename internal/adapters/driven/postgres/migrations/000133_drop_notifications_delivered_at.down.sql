ALTER TABLE notifications ADD COLUMN IF NOT EXISTS delivered_at timestamptz;
