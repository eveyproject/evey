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
EVEY_HTTP_PORT=9091                    # HTTP API port
EVEY_SERVER_NAME=Evey                  # display name
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### Database
```env
EVEY_DB_USER=evey
EVEY_DB_PASSWORD=changeme
EVEY_DB_NAME=evey
EVEY_DB_SSLMODE=disable               # disable, require, verify-full
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
EVEY_LIBRARY_SCAN_INTERVAL=6h          # rescan interval
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
```

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

### Logging
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_RETENTION_DAYS=90
```
