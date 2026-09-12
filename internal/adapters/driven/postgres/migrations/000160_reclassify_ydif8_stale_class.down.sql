-- Irreversible: the classes NULLed here are recomputed by the complexity worker
-- from the canonical metrics, and the pre-159 native-YDIF class they replaced
-- was wrong by construction. A down that restored it would put a known-bad class
-- back. This is a data fix with no schema change, so its down is a no-op.
SELECT 1;
