-- The server_metrics history has carried CPU, memory, streams and streaming
-- rate since the beginning, and nothing about the disks. Both production
-- incidents of the autumn (the audio switch of 2026-09-09, the load test of
-- 2026-10-15) were disk saturation with idle cores: iowait at 60-87 % and each
-- request waiting 27-46 ms, invisible on a CPU/memory chart. Each sample now
-- records the machine's iowait, the busiest disk's wait and busy share, the
-- summed disk and network rates, and the storage totals the sampler always
-- wrote as zero.
ALTER TABLE server_metrics ADD COLUMN iowait_percent DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE server_metrics ADD COLUMN disk_await_ms DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE server_metrics ADD COLUMN disk_util_percent DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE server_metrics ADD COLUMN disk_read_bps BIGINT NOT NULL DEFAULT 0;
ALTER TABLE server_metrics ADD COLUMN disk_write_bps BIGINT NOT NULL DEFAULT 0;
-- The disk whose wait is recorded: its stable identity label (model and
-- serial tail), never the kernel letter, which moves between boots.
ALTER TABLE server_metrics ADD COLUMN disk_busiest TEXT NOT NULL DEFAULT '';
ALTER TABLE server_metrics ADD COLUMN net_rx_bps BIGINT NOT NULL DEFAULT 0;
ALTER TABLE server_metrics ADD COLUMN net_tx_bps BIGINT NOT NULL DEFAULT 0;
