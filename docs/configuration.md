# Configuration

All configuration is in `.env`. Copy `.env.example` to `.env` and edit.

## Must Set

```env
EVEY_DB_PASSWORD=<strong-password>
MEILI_MASTER_KEY=<run: openssl rand -hex 16>
MEDIA_DIR=/path/to/your/media
```

## Recommended

```env
EVEY_SERVER_BASE_URL=https://media.example.com  # your public URL
EVEY_TMDB_API_KEY=<your-tmdb-key>               # better metadata (free at themoviedb.org)
```

## All Variables

### Server
```env
EVEY_SERVER_PORT=9090                  # gRPC listen port (default 9090); HTTP API binds at SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose-only: host port exposed by Docker for the HTTP gateway (not read by the server binary)
EVEY_SERVER_NAME=Evey                  # display name
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### Database
```env
EVEY_DB_USER=evey
EVEY_DB_PASSWORD=changeme
EVEY_DB_NAME=evey
EVEY_DB_SSLMODE=prefer                # default (works with the bundled Postgres); or disable | require | verify-full
POSTGRES_DATA_DIR=~/.evey/postgres-data
```

Migrations run automatically on startup.

### Authentication
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 days
EVEY_RATELIMIT_LOGIN_MAX=10            # per IP per minute
```

### Libraries
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # fallback polling interval when inotify misses events (default 2m); admin-editable via Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
EVEY_MKV_FASTPATH=0                    # 0/off (default) = probe every file with ffprobe; 1 = read .mkv metadata in-process (mkvgo, ~35x faster on re-scan/repair/NAS). ffprobe stays the authority and is used for any file the fast-path can't map field-equivalent. Restart to apply.
```

`EVEY_MKV_FASTPATH` only affects the library scan prober; music keeps ffprobe.
See [adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md).

### Transcoding
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

See [transcoding.md](transcoding.md) for hardware setup.

### Playback
```env
EVEY_PLAYBACK_MAX_STREAMS=0            # 0 = unlimited
```

### Plugins (sandbox & limits)
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic — auto picks the strongest your host supports
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off — run plugins as a separate user to wall off secrets (auto = on only on hosts without Landlock); incompatible with no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # max RAM per plugin (MiB); admin-editable via Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # max processes/threads per plugin; admin-editable
EVEY_PLUGINS_DISK_MAX_MB=100           # max install-dir size per plugin (MiB; 0 = off); admin-editable
EVEY_PLUGINS_CPU_NICE=10               # scheduling nice 0–19 (higher = lower priority); admin-editable
EVEY_PLUGINS_ENABLED=true              # master switch for the plugin subsystem
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce — require a cosign signature on plugin ZIPs
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # PEM public-key path (required when policy != off)
```

Per-scenario guide: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Full model: [SECURITY.md](SECURITY.md).

### Email (SMTP)
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

See [notifications.md](notifications.md).

### SSO (OIDC)
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # comma-separated, empty = all
```

See [oidc.md](oidc.md).

### Backups
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = disabled, 24 = daily
EVEY_BACKUP_RETENTION_DAYS=30
```

See [backups.md](backups.md).

### Reverse Proxy
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_FORWARDED_PROTO=true
EVEY_DOMAIN=media.example.com          # only for bundled Caddy
```

See [reverse-proxy.md](reverse-proxy.md).

### Intro detection
```env
EVEY_INTRO_TIDB_API_KEY=                # optional Bearer token for TheIntroDB (500 lookups/day vs 100 anonymous)
EVEY_INTRO_FFMPEG_THREADS=1             # threads per ffmpeg process used by intro detection (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # serve "preview" (next-episode reel) segments to the player so a "Skip Preview" button appears
```

`api.tidb_key`, `intro_providers.enabled`, `intro_providers.tidb_submit_enabled`,
and `intro_providers.skip_preview_enabled` can also be configured via
`/admin/server-config` (DB-first, env fallback). Restart the server after toggling
`intro_providers.enabled`; the skip-preview toggle applies live (no restart).

See [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md).

### Logging
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_RETENTION_DAYS=90
```
