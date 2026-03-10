CREATE TABLE IF NOT EXISTS scan_schedules (
    library_id      TEXT PRIMARY KEY REFERENCES libraries(id) ON DELETE CASCADE,
    cron_expression TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at     TIMESTAMPTZ,
    next_run_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
