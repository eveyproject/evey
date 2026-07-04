-- No-op: a data backfill can't be cleanly reversed (we no longer know which rows
-- were empty before). The column keeps its NOT NULL DEFAULT '' contract, so
-- leaving the values in place is harmless.
SELECT 1;
