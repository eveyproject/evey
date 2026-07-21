CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
DROP INDEX IF EXISTS idx_webhook_deliveries_webhook_sent;
