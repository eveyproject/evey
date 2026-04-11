CREATE INDEX IF NOT EXISTS idx_quick_connect_short_code ON quick_connect_requests (UPPER(RIGHT(code, 6)));
