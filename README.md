# Evey

Self-hosted media server for movies, TV shows, music, and photos.

## Install

```bash
cp .env.example .env
# Edit .env: set EVEY_DB_PASSWORD, MEILI_MASTER_KEY, and MEDIA_DIR
docker compose up -d
```

Open `http://<your-ip>:9091`, create your admin account, add a library. Done.

## Update

```bash
docker compose pull && docker compose up -d
```

## HTTPS

Evey doesn't handle HTTPS. Put a reverse proxy in front. The bundled Caddy option:

```bash
# Add EVEY_DOMAIN=media.example.com to .env
docker compose --profile caddy up -d
```

Or use your own Nginx/Traefik/etc. See [docs/reverse-proxy.md](docs/reverse-proxy.md).

## Apps

Android and Android TV apps connect to your server URL. Use **Quick Connect** to log in on TV without typing passwords.

## Configuration

Everything is in `.env`. The essentials:

| Variable | What it does |
|----------|--------------|
| `EVEY_DB_PASSWORD` | Database password |
| `MEILI_MASTER_KEY` | Search engine key (generate: `openssl rand -hex 16`) |
| `MEDIA_DIR` | Path to your media on the host |
| `EVEY_SERVER_BASE_URL` | Your public URL (for links, SSO, password reset) |

Full reference: [docs/configuration.md](docs/configuration.md)

## Features

- Adaptive HLS transcoding with optional [hardware acceleration](docs/transcoding.md) (VAAPI, NVENC, QSV)
- [SSO/OIDC](docs/oidc.md) with any provider (Keycloak, Authentik, Google...)
- [Email notifications](docs/notifications.md) for new media, with password recovery
- [Automatic backups](docs/backups.md) with configurable retention
- [Plugin system](docs/plugins.md) for themes, metadata providers, and widgets
- Multi-profile with parental controls
- SyncPlay (watch together)

## Plugins & Themes

Evey supports plugins for themes, custom metadata providers, and UI widgets. Plugins run sandboxed with seccomp and process isolation.

- Install plugins from the admin panel (ZIP upload)
- Create your own: see [docs/plugins.md](docs/plugins.md) and [examples](examples/)

Quick theme example:
```yaml
# plugin.yaml
name: my-dark-theme
version: 1.0.0
language: css
capabilities: [ui.theme]
entrypoint: theme.css
```

```css
/* theme.css */
:root {
  --bg: #0a0a0a;
  --surface: #1a1a1a;
  --accent: #e11d48;
  --text: #fafafa;
}
```

ZIP it, upload in Admin > Plugins. See [docs/plugins.md](docs/plugins.md) for the full spec.

## Docs

- [Configuration](docs/configuration.md)
- [Reverse Proxy](docs/reverse-proxy.md) (Caddy, Nginx, Traefik, NPM)
- [Transcoding](docs/transcoding.md)
- [Backups](docs/backups.md)
- [Notifications](docs/notifications.md)
- [SSO / OIDC](docs/oidc.md)
- [Plugins](docs/plugins.md)

## Health Check

```
GET /healthz
{"status":"ok","db":"ok","meilisearch":"ok","version":"0.1.0","uptime_seconds":3600}
```

## License

MIT
