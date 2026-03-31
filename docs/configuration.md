# Configuration Reference

All configuration is done via environment variables in `.env`.

## Required

| Variable | Example | Description |
|----------|---------|-------------|
| `EVEY_DB_PASSWORD` | `s3cureP@ss` | PostgreSQL password |
| `MEILI_MASTER_KEY` | `aBcDeFgH12345678` | Meilisearch master key (min 16 chars). Generate with `openssl rand -hex 16` |
| `MEDIA_DIR` | `/mnt/media` | Host path to your media files. Mounted read-only inside the container |

## Server

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | Public URL of your instance. Used for generating links, OIDC callbacks, and password reset emails |
| `EVEY_SERVER_NAME` | `Evey` | Server display name |
| `EVEY_HTTP_PORT` | `9091` | HTTP API port |
| `EVEY_SERVER_PORT` | `9090` | gRPC port (internal, plugins only) |
| `EVEY_VERSION` | `latest` | Docker image tag to use |

## Database

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_DB_USER` | `evey` | PostgreSQL username |
| `EVEY_DB_PASSWORD` | `evey` | PostgreSQL password |
| `EVEY_DB_NAME` | `evey` | Database name |
| `EVEY_DB_SSLMODE` | `disable` | SSL mode (`disable`, `require`, `verify-full`) |
| `POSTGRES_DATA_DIR` | `~/.evey/postgres-data` | Host path for database storage |

## Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_JWT_ACCESS_TTL` | `15m` | Access token lifetime |
| `EVEY_JWT_REFRESH_TTL` | `168h` | Refresh token lifetime (7 days) |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Max login attempts per minute per IP |

## Transcoding

See [transcoding.md](transcoding.md) for hardware acceleration setup.

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Max simultaneous transcode jobs |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `50` | Max transcode cache size in GB |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | Enable hardware acceleration |

## Library

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_LIBRARY_SCAN_INTERVAL` | `6h` | Automatic rescan interval |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Watch for file changes in real time |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | Fallback scan interval when inotify fails |

## Playback

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_PLAYBACK_MAX_STREAMS` | `0` | Max concurrent streams per user (0 = unlimited) |

## SMTP (Email Notifications)

See [notifications.md](notifications.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_SMTP_HOST` | | SMTP server hostname. Empty = email disabled |
| `EVEY_SMTP_PORT` | `587` | SMTP port |
| `EVEY_SMTP_USERNAME` | | SMTP username |
| `EVEY_SMTP_PASSWORD` | | SMTP password |
| `EVEY_SMTP_FROM` | | Sender address (e.g. `noreply@example.com`) |
| `EVEY_SMTP_TLS` | `starttls` | TLS mode: `none`, `starttls`, `ssl` |

## OIDC / SSO

See [oidc.md](oidc.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_OIDC_ENABLED` | `false` | Enable OIDC authentication |
| `EVEY_OIDC_ISSUER` | | OIDC provider URL |
| `EVEY_OIDC_CLIENT_ID` | | Client ID |
| `EVEY_OIDC_CLIENT_SECRET` | | Client secret |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | | Comma-separated domain allowlist. Empty = all domains |

## Metadata Providers

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_TMDB_API_KEY` | | TMDB API key (recommended for movie/show metadata) |
| `EVEY_OMDB_API_KEY` | | OMDb API key |
| `EVEY_METADATA_LANGUAGE` | `en` | Preferred metadata language (ISO 639-1) |

## Backups

See [backups.md](backups.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_BACKUP_DIR` | `/data/backups` | Backup storage directory |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` | Auto-backup interval (0 = disabled) |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Delete backups older than this |

## Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `EVEY_LOG_DIR` | `/data/logs` | Log directory |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Max log file size before rotation |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Delete logs older than this |

## Reverse Proxy

See [reverse-proxy.md](reverse-proxy.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `EVEY_TRUSTED_PROXIES` | `127.0.0.1/32,::1/128` | CIDR list of trusted proxy IPs |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | Header containing the real client IP |
| `EVEY_FORWARDED_PROTO` | `false` | Trust `X-Forwarded-Proto` for HTTPS detection |
| `EVEY_DOMAIN` | `localhost` | Domain for the bundled Caddy reverse proxy |
