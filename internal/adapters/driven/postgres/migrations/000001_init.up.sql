-- Evey schema migration

CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    username     TEXT NOT NULL UNIQUE,
    email        TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin     BOOLEAN NOT NULL DEFAULT FALSE,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    oidc_subject TEXT,
    oidc_issuer  TEXT,
    max_streams  INT DEFAULT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS profiles (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    avatar_path  TEXT,
    preferences  JSONB NOT NULL DEFAULT '{}',
    max_rating   TEXT,
    is_default   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_profiles_user_id ON profiles(user_id);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    device_name TEXT NOT NULL DEFAULT '',
    client_ip   TEXT,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash    TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]',
    last_used_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS invite_links (
    id         TEXT PRIMARY KEY,
    created_by TEXT NOT NULL REFERENCES users(id),
    token      TEXT NOT NULL UNIQUE,
    label      TEXT,
    max_uses   INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    is_admin   BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quick_connect_requests (
    id          TEXT PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    secret      TEXT NOT NULL,
    user_id     TEXT REFERENCES users(id),
    device_name TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS roles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    permissions JSONB NOT NULL DEFAULT '[]',
    is_system   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS libraries (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    type                 TEXT NOT NULL,
    paths                JSONB NOT NULL DEFAULT '[]',
    scan_interval_minutes INT NOT NULL DEFAULT 360,
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    last_scanned_at      TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS media_items (
    id               TEXT PRIMARY KEY,
    library_id       TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    parent_id        TEXT REFERENCES media_items(id) ON DELETE CASCADE,
    type             TEXT NOT NULL,
    title            TEXT NOT NULL DEFAULT '',
    sort_title       TEXT NOT NULL DEFAULT '',
    original_title   TEXT NOT NULL DEFAULT '',
    overview         TEXT NOT NULL DEFAULT '',
    tagline          TEXT NOT NULL DEFAULT '',
    file_path        TEXT,
    file_size        BIGINT NOT NULL DEFAULT 0,
    duration_ms      BIGINT NOT NULL DEFAULT 0,
    bitrate          BIGINT NOT NULL DEFAULT 0,
    container_format TEXT NOT NULL DEFAULT '',
    index_number     INT,
    community_rating DOUBLE PRECISION,
    official_rating  TEXT NOT NULL DEFAULT '',
    premiere_date    TIMESTAMPTZ,
    year             INT,
    provider_ids     JSONB NOT NULL DEFAULT '{}',
    genres           JSONB NOT NULL DEFAULT '[]',
    studios          JSONB NOT NULL DEFAULT '[]',
    tags             JSONB NOT NULL DEFAULT '[]',
    added_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_media_items_library_id ON media_items(library_id);
CREATE INDEX IF NOT EXISTS idx_media_items_parent_id ON media_items(parent_id);
CREATE INDEX IF NOT EXISTS idx_media_items_type ON media_items(type);
CREATE INDEX IF NOT EXISTS idx_media_items_file_path ON media_items(file_path);

CREATE TABLE IF NOT EXISTS media_streams (
    id             TEXT PRIMARY KEY,
    media_item_id  TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    stream_index   INT NOT NULL,
    stream_type    TEXT NOT NULL,
    codec          TEXT NOT NULL DEFAULT '',
    codec_long_name TEXT NOT NULL DEFAULT '',
    profile        TEXT NOT NULL DEFAULT '',
    language       TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL DEFAULT '',
    is_default     BOOLEAN NOT NULL DEFAULT FALSE,
    is_forced      BOOLEAN NOT NULL DEFAULT FALSE,
    width          INT,
    height         INT,
    framerate      DOUBLE PRECISION,
    bit_depth      INT,
    color_space    TEXT,
    hdr_type       TEXT,
    channels       INT,
    channel_layout TEXT,
    sample_rate    INT,
    bitrate        BIGINT,
    is_external    BOOLEAN NOT NULL DEFAULT FALSE,
    external_path  TEXT
);
CREATE INDEX IF NOT EXISTS idx_media_streams_media_item_id ON media_streams(media_item_id);

CREATE TABLE IF NOT EXISTS people (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    provider_ids JSONB NOT NULL DEFAULT '{}',
    thumb_path  TEXT
);
CREATE INDEX IF NOT EXISTS idx_people_name ON people(name);

CREATE TABLE IF NOT EXISTS media_people (
    media_item_id  TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    person_id      TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role_type      TEXT NOT NULL,
    character_name TEXT,
    sort_order     INT NOT NULL DEFAULT 0,
    PRIMARY KEY (media_item_id, person_id, role_type)
);

CREATE TABLE IF NOT EXISTS artwork (
    id           TEXT PRIMARY KEY,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    person_id    TEXT REFERENCES people(id) ON DELETE SET NULL,
    type         TEXT NOT NULL,
    provider     TEXT NOT NULL DEFAULT '',
    source_url   TEXT,
    local_path   TEXT,
    width        INT,
    height       INT,
    language     TEXT,
    sort_order   INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_artwork_media_item_id ON artwork(media_item_id);

CREATE TABLE IF NOT EXISTS collections (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    overview    TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL,
    filter_rules JSONB NOT NULL DEFAULT '{}',
    sort_field  TEXT NOT NULL DEFAULT 'sort_title',
    sort_order  TEXT NOT NULL DEFAULT 'asc',
    poster_path TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS collection_items (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    sort_order    INT NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, media_item_id)
);

CREATE TABLE IF NOT EXISTS watch_progress (
    id            TEXT PRIMARY KEY,
    profile_id    TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    position_ms   BIGINT NOT NULL DEFAULT 0,
    completed     BOOLEAN NOT NULL DEFAULT FALSE,
    play_count    INT NOT NULL DEFAULT 0,
    last_played_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, media_item_id)
);
CREATE INDEX IF NOT EXISTS idx_watch_progress_profile_id ON watch_progress(profile_id);

CREATE TABLE IF NOT EXISTS play_sessions (
    id            TEXT PRIMARY KEY,
    profile_id    TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    device_name   TEXT NOT NULL DEFAULT '',
    device_type   TEXT NOT NULL DEFAULT '',
    client_ip     TEXT NOT NULL DEFAULT '',
    play_method   TEXT NOT NULL DEFAULT 'direct_play',
    video_codec   TEXT NOT NULL DEFAULT '',
    audio_codec   TEXT NOT NULL DEFAULT '',
    resolution    TEXT NOT NULL DEFAULT '',
    bandwidth_bps BIGINT NOT NULL DEFAULT 0,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at      TIMESTAMPTZ,
    duration_ms   BIGINT,
    position_ms   BIGINT
);
CREATE INDEX IF NOT EXISTS idx_play_sessions_profile_id ON play_sessions(profile_id);
CREATE INDEX IF NOT EXISTS idx_play_sessions_started_at ON play_sessions(started_at);

CREATE TABLE IF NOT EXISTS login_history (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_ip      TEXT NOT NULL DEFAULT '',
    user_agent     TEXT NOT NULL DEFAULT '',
    auth_method    TEXT NOT NULL DEFAULT 'local',
    success        BOOLEAN NOT NULL DEFAULT TRUE,
    failure_reason TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_login_history_user_id ON login_history(user_id);
CREATE INDEX IF NOT EXISTS idx_login_history_created_at ON login_history(created_at);

CREATE TABLE IF NOT EXISTS server_metrics (
    id                 TEXT PRIMARY KEY,
    cpu_percent        DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_bytes  BIGINT NOT NULL DEFAULT 0,
    memory_total_bytes BIGINT NOT NULL DEFAULT 0,
    active_streams     INT NOT NULL DEFAULT 0,
    active_transcodes  INT NOT NULL DEFAULT 0,
    bandwidth_bps      BIGINT NOT NULL DEFAULT 0,
    storage_used_bytes BIGINT NOT NULL DEFAULT 0,
    storage_total_bytes BIGINT NOT NULL DEFAULT 0,
    recorded_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_server_metrics_recorded_at ON server_metrics(recorded_at);

CREATE TABLE IF NOT EXISTS transcode_sessions (
    id              TEXT PRIMARY KEY,
    media_item_id   TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    input_codec     TEXT NOT NULL DEFAULT '',
    output_codec    TEXT NOT NULL DEFAULT '',
    hw_accel_type   TEXT NOT NULL DEFAULT 'none',
    preset          TEXT NOT NULL DEFAULT '',
    resolution_in   TEXT NOT NULL DEFAULT '',
    resolution_out  TEXT NOT NULL DEFAULT '',
    bitrate_in      BIGINT NOT NULL DEFAULT 0,
    bitrate_out     BIGINT NOT NULL DEFAULT 0,
    speed           DOUBLE PRECISION NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'queued',
    error_message   TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ,
    duration_ms     BIGINT
);
CREATE INDEX IF NOT EXISTS idx_transcode_sessions_status ON transcode_sessions(status);

CREATE TABLE IF NOT EXISTS plugins (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    version       TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    author        TEXT NOT NULL DEFAULT '',
    language      TEXT NOT NULL DEFAULT '',
    capabilities  JSONB NOT NULL DEFAULT '[]',
    permissions   JSONB NOT NULL DEFAULT '[]',
    config_schema JSONB NOT NULL DEFAULT '{}',
    config        JSONB NOT NULL DEFAULT '{}',
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    status        TEXT NOT NULL DEFAULT 'stopped',
    error_message TEXT NOT NULL DEFAULT '',
    installed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plugin_runtimes (
    id        TEXT PRIMARY KEY,
    language  TEXT NOT NULL,
    version   TEXT NOT NULL,
    status    TEXT NOT NULL DEFAULT 'tested',
    notes     TEXT NOT NULL DEFAULT '',
    tested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (language, version)
);

CREATE TABLE IF NOT EXISTS server_config (
    key       TEXT PRIMARY KEY,
    value     TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    type            TEXT NOT NULL,
    schedule        TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at     TIMESTAMPTZ,
    next_run_at     TIMESTAMPTZ,
    last_result     TEXT,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metadata_overrides (
    id           TEXT PRIMARY KEY,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    field_name   TEXT NOT NULL,
    custom_value JSONB NOT NULL,
    edited_by    TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (media_item_id, field_name)
);
CREATE INDEX IF NOT EXISTS idx_metadata_overrides_media_item_id ON metadata_overrides(media_item_id);

CREATE TABLE IF NOT EXISTS file_events (
    id           TEXT PRIMARY KEY,
    library_id   TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    event_type   TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    processed    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_file_events_library_id ON file_events(library_id);
CREATE INDEX IF NOT EXISTS idx_file_events_processed ON file_events(processed);

CREATE TABLE IF NOT EXISTS library_scans (
    id            TEXT PRIMARY KEY,
    library_id    TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'running',
    items_found   INT NOT NULL DEFAULT 0,
    items_added   INT NOT NULL DEFAULT 0,
    items_updated INT NOT NULL DEFAULT 0,
    items_removed INT NOT NULL DEFAULT 0,
    error_message TEXT,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_library_scans_library_id ON library_scans(library_id);

CREATE TABLE IF NOT EXISTS audit_log (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT,
    details       JSONB NOT NULL DEFAULT '{}',
    client_ip     TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);
