-- Extended disposition flags on media streams, sourced from Matroska
-- FlagCommentary / FlagHearingImpaired / FlagVisualImpaired /
-- FlagTextDescriptions / FlagOriginal and the matching ffprobe disposition
-- fields (comment / hearing_impaired / visual_impaired / descriptions /
-- original). All default to FALSE so existing rows keep their current
-- semantics (absent = flag not set).
ALTER TABLE media_streams
    ADD COLUMN IF NOT EXISTS commentary         BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS hearing_impaired   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS visual_impaired    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS text_descriptions  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS original           BOOLEAN NOT NULL DEFAULT FALSE;
