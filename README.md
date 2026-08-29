<p align="center">
  <img src="evey-brand/social/readme-hero.png" alt="Evey" width="800">
</p>

# Evey

Self-hosted media server for movies, TV shows, music, and photos.

Brand assets (logo, favicon, social images) live in [`evey-brand/`](evey-brand/) - see its [README](evey-brand/README.md) for usage rules.

## Install

One container, no secret to invent. Let the image write the compose file:

```bash
mkdir evey && cd evey
docker run --rm -v "$PWD":/out gravityz/evey init --media /path/to/your/media
docker compose up -d
```

Open `http://<your-ip>:9091`, create your account, add a library. Postgres and
Meilisearch run inside the container and generate their own secrets on first
start (kept in `./evey-data/keys`, next to the JWT signing key - keep that
directory safe). Migrations run automatically.

Prefer to paste the file yourself? This is the whole of it:

**`docker-compose.yml`**
```yaml
services:
  evey:
    image: gravityz/evey:${EVEY_VERSION:-latest}
    container_name: evey-server
    ports:
      - "9091:9091"
    env_file: .env
    volumes:
      - ./evey-data:/data     # database, JWT key, generated secrets, search index, logs, backups
      - evey-cache:/cache     # transcode + trickplay cache (safe to prune)
      - ${MEDIA_DIR}:/media   # read-write so in-place repairs work; append :ro to refuse them
    stop_grace_period: 60s
    restart: unless-stopped

volumes:
  evey-cache:
```

**`.env`**
```env
MEDIA_DIR=/path/to/your/media
# EVEY_VERSION=latest   # pin a specific tag (e.g. v0.1.0, beta-0.0.1)
```

`evey init --domain media.example.com` adds the bundled Caddy reverse proxy
with automatic HTTPS; `--external-postgres` keeps the database as a separate
compose service (the layout earlier releases used - existing installs keep
working unchanged, `EVEY_DB_HOST` is what selects it).

> **ARM (Raspberry Pi, ARM NAS)?** Stable `vX.Y.Z` releases ship a multi-arch
> image (amd64 + arm64) - the same `docker pull` works, but ARM has no hardware
> transcoding (direct-play + software only). Details, PUID/PGID, an external
> Postgres or Meilisearch, and every other option are in
> [docs/installation.md](docs/installation.md).

## Update

```bash
docker compose pull && docker compose up -d
```

That is the whole procedure: the server backs the database up by itself
before applying a migration, and the container reports healthy once it is
serving. To go back, set `EVEY_VERSION` in `.env` to the previous tag and run
`docker compose up -d` again (`docker exec evey-server evey admin rollback`
tells you whether the database must be restored first, and how). Pin what you
run with `EVEY_VERSION` - a release tag (`0.2.0-beta.1`) or an immutable
`sha-<short>` tag, not a branch name, which moves with every push.

## HTTPS

Evey serves plain HTTP on 9091. `evey init --domain media.example.com` writes a
Caddy service that obtains a Let's Encrypt certificate for you; with your own
proxy (Nginx, Traefik, Nginx Proxy Manager) see
[docs/reverse-proxy.md](docs/reverse-proxy.md) for ready-to-use configs.

## Configuration

`.env` holds only what must exist before the server boots. With the default
install that is one line - `MEDIA_DIR` - everything else (transcoding, hardware
acceleration, libraries, plugins, API keys) is set in **Admin → Server config**.

Full reference: [docs/configuration.md](docs/configuration.md)

## Volumes

| Volume | What's inside | Can you delete it? |
|--------|--------------|-------------------|
| `evey-data` | Database (`postgres/`), JWT key + generated secrets (`keys/`), logs, backups, plugins, search index | No - you lose your data and your signing key |
| `evey-cache` | Transcode + trickplay cache | Yes - regenerated on demand |
| `postgres-data` | Database, only with `--external-postgres` | No - you lose all data |

The embedded Meilisearch index lives under `evey-data` and is rebuilt from the
database on the next scan if lost. If you run the bundled Caddy reverse proxy,
its `caddy-data` volume holds the TLS certificates (re-requested if deleted).

## Apps

Android and Android TV apps connect to your server URL. Use **Quick Connect** to log in on TV - type the code on your phone instead of wrestling with a remote.

## Features

- Adaptive HLS transcoding with [hardware acceleration](docs/transcoding.md) (VAAPI, NVENC, QSV)
- Android & Android TV apps (Expo/React Native)
- Full-text search powered by Meilisearch
- Multi-language metadata (TMDB/OMDb) + 6 UI languages (EN, FR, DE, ES, ZH, HI)
- [SSO/OIDC](docs/oidc.md) - Keycloak, Authentik, Google, anything OIDC-compliant
- [Email notifications](docs/notifications.md) + password recovery
- [Automatic backups](docs/backups.md)
- [Plugins & themes](docs/plugins.md) - multi-layer isolation (static analysis, UID drop, seccomp syscall filter, optional LD_PRELOAD)
- Multi-profile with parental controls
- SyncPlay - watch together in sync
- Webhooks for external integrations
- Trickplay (seek thumbnails)

## Plugins & Themes

You can extend Evey with plugins: themes, metadata providers, UI widgets. They run with multi-layer isolation (static analysis at install, UID drop, seccomp syscall filter, optional `LD_PRELOAD` guard). Each layer has known limits - **always review third-party plugins before installing them**. See [docs/plugins/security-model.md](docs/plugins/security-model.md) for the full model and caveats.

Quick CSS theme:
```yaml
# plugin.yaml
name: my-theme
kind: theme
version: 1.0.0
```
```css
/* theme.css */
:root { --bg: #0a0a0a; --accent: #e11d48; --text: #fafafa; }
```

ZIP it, upload in Admin > Plugins. Full spec and examples: [docs/plugins.md](docs/plugins.md)

## Languages & Accessibility

The UI ships in 6 languages (EN, FR, DE, ES, ZH, HI), with English as the source
and fallback; media metadata language is configured separately. The interface
honors `prefers-reduced-motion`, labels its controls, and the player is
keyboard-driven - but Evey has **not** been formally audited for accessibility
and isn't screen-reader tested end to end. For the honest details (how i18n
works, how to add a language, and the a11y gaps that remain), see
[docs/i18n-accessibility.md](docs/i18n-accessibility.md).

## Docs

| | |
|---|---|
| [Installation & Operations](docs/installation.md) | Full setup, ARM, PUID/PGID, updating |
| [Configuration](docs/configuration.md) | All env variables |
| [Languages & Accessibility](docs/i18n-accessibility.md) | i18n + a11y: state & gaps |
| [Reverse Proxy](docs/reverse-proxy.md) | Caddy, Nginx, Traefik |
| [Transcoding](docs/transcoding.md) | Hardware acceleration |
| [Backups](docs/backups.md) | Schedule & restore |
| [Notifications](docs/notifications.md) | SMTP & push |
| [SSO / OIDC](docs/oidc.md) | Single sign-on |
| [Plugins](docs/plugins.md) | Build your own |

## License

MIT
