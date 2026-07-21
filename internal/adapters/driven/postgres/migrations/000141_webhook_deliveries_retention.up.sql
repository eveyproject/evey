-- The delivery log had one index, on webhook_id alone, while its only read
-- orders by sent_at DESC. Postgres therefore had to sort every delivery ever
-- recorded for that webhook before applying the LIMIT - work that grows with
-- the table forever, since nothing ever deleted a row.
--
-- The composite index serves the filter AND the ordering, so the read becomes a
-- bounded index scan regardless of history size.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_sent
    ON webhook_deliveries (webhook_id, sent_at DESC);

-- The single-column index is now redundant: (webhook_id, sent_at) serves every
-- query the old one did, and carrying both costs write amplification on the
-- delivery hot path for nothing.
DROP INDEX IF EXISTS idx_webhook_deliveries_webhook_id;
