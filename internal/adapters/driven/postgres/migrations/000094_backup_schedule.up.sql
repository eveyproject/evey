CREATE TABLE backup_schedule (
    id              BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id = TRUE),
    cron_expression TEXT NOT NULL DEFAULT '0 3 * * *',
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    retention_days  INTEGER NOT NULL DEFAULT 30 CHECK (retention_days > 0),
    last_run_at     TIMESTAMPTZ,
    next_run_at     TIMESTAMPTZ,
    last_status     TEXT,
    last_error      TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO backup_schedule (id) VALUES (TRUE) ON CONFLICT DO NOTHING;
