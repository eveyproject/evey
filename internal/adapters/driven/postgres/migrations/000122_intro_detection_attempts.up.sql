-- Bounded, version-gated intro-detection retries. An episode where detection
-- finds no usable intro/credits leaves no media_segments row, so the startup
-- backfill (ListSeasonsMissingSegments + detectSeason) re-decodes it — a full
-- software HEVC decode for blackdetect/Chromaprint — on EVERY restart, forever,
-- landing on the same empty answer. This burns CPU on a weak NAS for nothing.
--
-- intro_detection_attempts records, per media item, how many times detection ran
-- without producing a "good" intro AND the algorithm version it ran under. The
-- detector skips an item once attempts >= a cap at the CURRENT version, and
-- re-runs it again only when introDetectionAlgoVersion is bumped (a real
-- detection improvement) — the row's older version then no longer matches, so it
-- becomes eligible again exactly once. No backfill: an absent row = never
-- attempted = eligible, the existing behaviour; the table fills as the sweep runs.
CREATE TABLE intro_detection_attempts (
    media_item_id   TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    attempts        INTEGER NOT NULL DEFAULT 0,
    algo_version    INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
