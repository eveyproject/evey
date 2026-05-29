-- CMCD (CTA-5004) ingest: one row per segment request that carried a CMCD
-- payload. Used by the admin dashboard (rebuffer ratio, p95 startup, top
-- starved titles) and by the feedback loop (per-title ladder learning,
-- throttle wake-up on imminent starvation).
--
-- Volume sizing: 1 HLS session ≈ 1 segment/3s ≈ 1200 rows/h. A busy 5-user
-- evening: ~6000 rows/h. 30-day retention default = ~4.3M rows. Indexes are
-- sized for that.
CREATE TABLE IF NOT EXISTS playback_qoe_events (
    id                  BIGSERIAL PRIMARY KEY,
    ts                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Who/what. user_id is null for anonymous/shared link plays; media_item_id
    -- can stay null when we can't resolve it (defensive — we never want a
    -- failing FK to drop the metric).
    user_id             UUID,
    media_item_id       UUID,
    session_id          TEXT,        -- CMCD sid: opaque per-session UUID set by the player
    content_id          TEXT,        -- CMCD cid: opaque per-content key set by the player

    -- What was being fetched.
    variant_idx         INT,
    segment_idx         INT,
    object_type         TEXT,        -- CMCD ot: a|v|av|i|m|t|tt
    object_duration_ms  INT,

    -- Client-side telemetry.
    buffer_length_ms    INT,
    encoded_kbps        INT,         -- CMCD br
    top_bitrate_kbps    INT,         -- CMCD tb
    measured_kbps       INT,         -- CMCD mtp
    requested_max_kbps  INT,         -- CMCD rtp
    deadline_ms         INT,
    playback_rate       DOUBLE PRECISION,
    streaming_format    TEXT,        -- "h" HLS | "d" DASH | "s" Smooth
    stream_type         TEXT,        -- "v" VOD | "l" live
    cmcd_version        INT,

    -- Signals (the actually-actionable bits).
    buffer_starved      BOOLEAN NOT NULL DEFAULT FALSE,  -- bs
    startup             BOOLEAN NOT NULL DEFAULT FALSE,  -- su

    -- Round-trip the full raw payload as JSONB so we don't lose custom keys
    -- or future CTA-5004 additions before we promote them to typed columns.
    raw                 JSONB,

    -- Device class derived from User-Agent at insert time (web/ios/android/tv/...).
    -- Cheap to compute, expensive to derive at dashboard query time across 4M rows.
    device_class        TEXT,

    -- For rate limiting / debugging only — never used for auth.
    remote_ip           INET
);

-- Dashboard queries: "rebuffer ratio over last 7d", "top starved titles",
-- "p95 startup time" all filter by ts then group by media_item_id / session_id.
CREATE INDEX IF NOT EXISTS idx_qoe_ts                ON playback_qoe_events (ts DESC);
CREATE INDEX IF NOT EXISTS idx_qoe_media_ts          ON playback_qoe_events (media_item_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_qoe_session           ON playback_qoe_events (session_id, ts DESC);
-- Sparse index: most rows have starved=FALSE so this stays tiny.
CREATE INDEX IF NOT EXISTS idx_qoe_starved_ts        ON playback_qoe_events (ts DESC) WHERE buffer_starved = TRUE;
