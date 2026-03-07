ALTER TABLE profiles
    ADD COLUMN IF NOT EXISTS audio_languages    TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS subtitle_languages TEXT[] NOT NULL DEFAULT '{}';
