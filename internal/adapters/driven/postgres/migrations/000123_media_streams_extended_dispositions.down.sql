ALTER TABLE media_streams
    DROP COLUMN IF EXISTS commentary,
    DROP COLUMN IF EXISTS hearing_impaired,
    DROP COLUMN IF EXISTS visual_impaired,
    DROP COLUMN IF EXISTS text_descriptions,
    DROP COLUMN IF EXISTS original;
