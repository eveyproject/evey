# Evey

A self-hosted media server for movies, TV shows, music, and photos.

## Quick Start

```bash
# 1. Get the compose file and config template
curl -O https://raw.githubusercontent.com/gravity-zero/evey/master/docker-compose.yml
curl -O https://raw.githubusercontent.com/gravity-zero/evey/master/.env.example
cp .env.example .env

# 2. Edit .env — at minimum:
#    EVEY_DB_PASSWORD=<strong-password>
#    MEILI_MASTER_KEY=<random-32-char-string>
#    MEDIA_DIR=/path/to/your/media

# 3. Start
docker compose up -d

# 4. Open http://<your-ip>:9091 and create your admin account
```

That's it. Evey pulls from Docker Hub, starts Postgres and Meilisearch, and is ready in ~30 seconds.

## Requirements

- Docker Engine 24+ and Docker Compose v2+
- 2 GB RAM minimum (4 GB+ recommended for transcoding)
- A media library on disk

## What You Get

| Service | Port | Description |
|---------|------|-------------|
| **evey** | 9091 | REST API, streaming, transcoding |
| **postgres** | internal | Database |
| **meilisearch** | internal | Full-text search |

Port 9091 is the only port you need to expose. Add a reverse proxy for HTTPS.

## HTTPS & Reverse Proxy

Evey does not handle HTTPS. Use a reverse proxy in front of it. See [docs/reverse-proxy.md](docs/reverse-proxy.md) for ready-to-use configs:

- [Caddy](docs/reverse-proxy.md#caddy) (easiest, auto HTTPS)
- [Nginx](docs/reverse-proxy.md#nginx)
- [Traefik](docs/reverse-proxy.md#traefik)
- [Nginx Proxy Manager](docs/reverse-proxy.md#nginx-proxy-manager)

**Bundled Caddy** (optional):
```bash
# Set your domain
echo 'EVEY_DOMAIN=media.example.com' >> .env

# Start with Caddy
docker compose --profile caddy up -d
```

## First Setup

1. Open your Evey URL
2. Create your admin account (username, password, email)
3. Go to **Admin > Libraries** and add a media folder
4. Wait for the scan to finish
5. Start watching

## Updating

```bash
docker compose pull
docker compose up -d
```

To pin a specific version:
```env
EVEY_VERSION=v0.1.0-beta.1
```

## Health Check

```bash
curl http://localhost:9091/healthz
# {"status":"ok","db":"ok","meilisearch":"ok","version":"0.1.0","uptime_seconds":3600}
```

## Mobile & TV Apps

Evey has apps for Android and Android TV. Connect by entering your server URL.

**Quick Connect**: display a code on your TV, enter it on your phone — no need to type passwords with a remote.

## Documentation

| Topic | Link |
|-------|------|
| Configuration reference | [docs/configuration.md](docs/configuration.md) |
| Reverse proxy setup | [docs/reverse-proxy.md](docs/reverse-proxy.md) |
| Hardware transcoding | [docs/transcoding.md](docs/transcoding.md) |
| Backups | [docs/backups.md](docs/backups.md) |
| Email notifications | [docs/notifications.md](docs/notifications.md) |
| SSO / OIDC | [docs/oidc.md](docs/oidc.md) |
| Plugins | [docs/plugins.md](docs/plugins.md) |

## License

MIT
