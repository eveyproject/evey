ALTER TABLE profiles
    DROP COLUMN IF EXISTS audio_languages,
    DROP COLUMN IF EXISTS subtitle_languages;
