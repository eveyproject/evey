-- A session ended by the stale sweep or by a shutdown was closed without a
-- duration: only a client stop wrote one. The sweep now stores it; the rows
-- it closed before are measured here, start to end. Idempotent: a row with a
-- duration is left alone.
UPDATE play_sessions
   SET duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (ended_at - started_at)) * 1000)::bigint)
 WHERE ended_at IS NOT NULL AND duration_ms IS NULL;
