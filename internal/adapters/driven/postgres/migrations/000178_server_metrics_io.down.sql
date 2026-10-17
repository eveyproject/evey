ALTER TABLE server_metrics DROP COLUMN IF EXISTS net_tx_bps;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS net_rx_bps;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS disk_busiest;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS disk_write_bps;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS disk_read_bps;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS disk_util_percent;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS disk_await_ms;
ALTER TABLE server_metrics DROP COLUMN IF EXISTS iowait_percent;
