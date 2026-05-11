# Evey

Self-hosted media server for movies, TV shows, music, and photos.

## Install

Create a directory for Evey (`mkdir evey && cd evey`), drop these two files in it, then `docker compose up -d`.

**`docker-compose.yml`** (paste as-is):
```yaml
services:
  evey:
    image: gravityz/evey:${EVEY_VERSION:-latest}
    container_name: evey
    ports:
      - "9091:9091"
    environment:
      EVEY_DB_HOST: postgres
      EVEY_MEILI_URL: http://meilisearch:7700
      EVEY_JWT_PRIVATE_KEY_PATH: /data/jwt.key
      EVEY_TRANSCODING_CACHE_DIR: /cache/transcode
      EVEY_TRICKPLAY_CACHE_DIR: /data/trickplay
      EVEY_LOG_DIR: /data/logs
      EVEY_BACKUP_DIR: /data/backups
    env_file: .env
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - ${MEDIA_DIR}:/media:ro
    depends_on:
      postgres: { condition: service_healthy }
      meilisearch: { condition: service_healthy }
    restart: unless-stopped

  postgres:
    image: postgres:16-alpine
    container_name: evey-postgres
    environment:
      POSTGRES_DB: ${EVEY_DB_NAME:-evey}
      POSTGRES_USER: ${EVEY_DB_USER:-evey}
      POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
    volumes:
      - ./postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped

  meilisearch:
    image: getmeili/meilisearch:v1.12
    container_name: evey-meilisearch
    environment:
      MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - ./meili-data:/meili_data
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped

volumes:
  evey-cache:
```

**`.env`** (edit the three values):
```env
EVEY_DB_PASSWORD=<strong-password>
MEILI_MASTER_KEY=<run: openssl rand -hex 16>
MEDIA_DIR=/path/to/your/media
# EVEY_VERSION=latest   # pin a specific tag (e.g. v0.1.0, beta-0.0.1)
```

Then:
```bash
docker compose up -d
```

Open `http://<your-ip>:9091`, create your account, add a library. The image serves both the API and the web UI on the same port. Migrations run automatically. A JWT signing key is generated on first start in `./evey-data` — keep that directory safe.

## Update

```bash
docker compose pull && docker compose up -d
```

Pin a specific version by setting `EVEY_VERSION` in `.env` (e.g. `v0.1.0-beta.1`, `beta-0.0.1`).

## HTTPS

Evey doesn't handle HTTPS itself. Put a reverse proxy in front (Caddy, Nginx, Traefik). See [docs/reverse-proxy.md](docs/reverse-proxy.md) for ready-to-use configs.

## Configuration

Everything goes in `.env`. You only need three things to start:

```env
EVEY_DB_PASSWORD=<strong-password>
MEILI_MASTER_KEY=<run: openssl rand -hex 16>
MEDIA_DIR=/path/to/your/media
```

Full reference: [docs/configuration.md](docs/configuration.md)

## Volumes

| Volume | What's inside | Can you delete it? |
|--------|--------------|-------------------|
| `postgres-data` | Database | No — you lose all data |
| `evey-data` | JWT key, logs, backups | No — you lose your signing key |
| `evey-plugins` | Installed plugins | No — you lose your plugins |
| `meili-data` | Search index | Yes — rebuilt on next scan |
| `evey-cache` | Transcode + trickplay cache | Yes — regenerated on demand |
| `caddy-data` | TLS certificates | Depends — Caddy re-requests them |

## Apps

Android and Android TV apps connect to your server URL. Use **Quick Connect** to log in on TV — type the code on your phone instead of wrestling with a remote.

## Features

- Adaptive HLS transcoding with [hardware acceleration](docs/transcoding.md) (VAAPI, NVENC, QSV)
- Android & Android TV apps (Expo/React Native)
- Full-text search powered by Meilisearch
- Multi-language metadata (TMDB/OMDb) + 6 UI languages (EN, FR, DE, ES, ZH, HI)
- [SSO/OIDC](docs/oidc.md) — Keycloak, Authentik, Google, anything OIDC-compliant
- [Email notifications](docs/notifications.md) + password recovery
- [Automatic backups](docs/backups.md)
- [Plugins & themes](docs/plugins.md) — multi-layer isolation (static analysis, UID drop, seccomp syscall filter, optional LD_PRELOAD)
- Multi-profile with parental controls
- SyncPlay — watch together in sync
- Webhooks for external integrations
- Trickplay (seek thumbnails)

## Plugins & Themes

You can extend Evey with plugins: themes, metadata providers, UI widgets. They run with multi-layer isolation (static analysis at install, UID drop, seccomp syscall filter, optional `LD_PRELOAD` guard). Each layer has known limits — **always review third-party plugins before installing them**. See [docs/plugins.md](docs/plugins.md#security-model) for the full model and caveats.

Quick CSS theme:
```yaml
# plugin.yaml
name: my-theme
version: 1.0.0
language: css
capabilities: [ui.theme]
entrypoint: theme.css
```
```css
/* theme.css */
:root { --bg: #0a0a0a; --accent: #e11d48; --text: #fafafa; }
```

ZIP it, upload in Admin > Plugins. Full spec and examples: [docs/plugins.md](docs/plugins.md)

## Docs

| | |
|---|---|
| [Configuration](docs/configuration.md) | All env variables |
| [Reverse Proxy](docs/reverse-proxy.md) | Caddy, Nginx, Traefik |
| [Transcoding](docs/transcoding.md) | Hardware acceleration |
| [Backups](docs/backups.md) | Schedule & restore |
| [Notifications](docs/notifications.md) | SMTP & push |
| [SSO / OIDC](docs/oidc.md) | Single sign-on |
| [Plugins](docs/plugins.md) | Build your own |

## License

MIT
