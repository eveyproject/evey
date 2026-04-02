# Evey

Self-hosted media server for movies, TV shows, music, and photos.

## Install

```bash
cp .env.example .env
# Edit .env — set your database password, search key, and media path
docker compose up -d
```

Open `http://<your-ip>:9091`, create your account, add a library. That's it.

Migrations run automatically. A JWT signing key is generated on first start in the `evey-data` volume — keep that volume safe.

## Update

```bash
docker compose pull && docker compose up -d
```

Pin a specific version if you want:
```env
EVEY_VERSION=v0.1.0-beta.1
```

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
- [Plugins & themes](docs/plugins.md) — sandboxed with seccomp
- Multi-profile with parental controls
- SyncPlay — watch together in sync
- Webhooks for external integrations
- Trickplay (seek thumbnails)

## Plugins & Themes

You can extend Evey with plugins: themes, metadata providers, UI widgets. They run sandboxed.

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
