CREATE TABLE IF NOT EXISTS transcode_profiles (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    video_codec   TEXT NOT NULL DEFAULT 'h264',
    audio_codec   TEXT NOT NULL DEFAULT 'aac',
    container     TEXT NOT NULL DEFAULT 'hls',
    max_width     INT NOT NULL DEFAULT 1920,
    max_height    INT NOT NULL DEFAULT 1080,
    video_bitrate BIGINT NOT NULL DEFAULT 8000000,
    audio_bitrate BIGINT NOT NULL DEFAULT 192000,
    preset        TEXT NOT NULL DEFAULT 'medium'
);

INSERT INTO transcode_profiles (id, name, video_codec, audio_codec, container, max_width, max_height, video_bitrate, audio_bitrate, preset) VALUES
    ('tp-1080p',  '1080p H.264',  'h264', 'aac', 'hls', 1920, 1080, 8000000,  192000, 'medium'),
    ('tp-720p',   '720p H.264',   'h264', 'aac', 'hls', 1280, 720,  4000000,  128000, 'medium'),
    ('tp-480p',   '480p H.264',   'h264', 'aac', 'hls', 854,  480,  2000000,  128000, 'fast'),
    ('tp-4k',     '4K HEVC',      'hevc', 'aac', 'hls', 3840, 2160, 20000000, 192000, 'slow')
ON CONFLICT (id) DO NOTHING;
