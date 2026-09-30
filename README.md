<p align="center">
  <img src="evey-brand/social/readme-hero.png" alt="Evey" width="800">
</p>

# Evey

Self-hosted media server for movies, TV shows, music, and photos.

Brand assets (logo, favicon, social images) live in [`evey-brand/`](evey-brand/) - see its [README](evey-brand/README.md) for usage rules.

## Install

One container, one file, no secret to invent. Save this as
`docker-compose.yml` with your media path, then `docker compose up -d`:

```yaml
services:
  evey:
    image: gravityz/evey:latest        # pin a tag here (e.g. 0.2.0-beta.1, sha-1a2b3c4)
    container_name: evey-server
    ports:
      - "9091:9091"
    volumes:
      - ./evey-data:/data              # database, JWT key, generated secrets, search index, logs, backups
      - evey-cache:/cache              # transcode + trickplay cache (safe to prune)
      - /path/to/your/media:/media     # read-write so in-place repairs work; append :ro to refuse them
    stop_grace_period: 60s
    restart: unless-stopped

volumes:
  evey-cache:
```

Open `http://<your-ip>:9091`, create your account, add a library. Postgres and
Meilisearch run inside the container and generate their own secrets on first
start (kept in `./evey-data/keys`, next to the JWT signing key - keep that
directory safe). Migrations run automatically. No `.env`.

A second media folder is one more line (`- /other/path:/media2`). Behind a
reverse proxy you already run, or with HTTPS, or with Postgres as its own
service: the three-line changes are in
[docs/installation.md](docs/installation.md). Installs from before the
embedded database keep working unchanged - `EVEY_DB_HOST` being set is what
keeps their own Postgres in charge.

> **ARM (Raspberry Pi, ARM NAS)?** Stable `vX.Y.Z` releases ship a multi-arch
> image (amd64 + arm64) - the same `docker pull` works. Hardware transcoding on
> ARM is Rockchip RKMPP only (best-effort, certified by the boot calibration);
> a Raspberry Pi or a Realtek NAS plays direct and transcodes in software.
> Details, PUID/PGID, an external
> Postgres or Meilisearch, and every other option are in
> [docs/installation.md](docs/installation.md).

## Update

```bash
docker compose pull && docker compose up -d
```

That is the whole procedure: the server backs the database up by itself
before applying a migration, and the container reports healthy once it is
serving. To go back, put the previous tag on the `image:` line and run
`docker compose up -d` again (`docker exec evey-server evey admin rollback`
tells you whether the database must be restored first, and how). Pin what you
run - a release tag (`0.2.0-beta.1`) or an immutable `sha-<short>` tag, not a
branch name, which moves with every push.

## HTTPS

Evey serves plain HTTP on 9091. Put your reverse proxy in front (Nginx Proxy
Manager, Traefik, Nginx, Caddy): [docs/reverse-proxy.md](docs/reverse-proxy.md)
has the settings for each, and a bundled Caddy block that obtains a Let's
Encrypt certificate for you if you have no proxy yet.

## Configuration

The default install has no `.env`: the compose file holds the media path and
the image tag, and everything else (transcoding, hardware acceleration,
libraries, plugins, API keys) is set in **Admin → Server config**. A `.env`
appears only to carry a secret shared between compose services (an external
Postgres or Meilisearch).

Full reference: [docs/configuration.md](docs/configuration.md)

## Volumes

| Volume | What's inside | Can you delete it? |
|--------|--------------|-------------------|
| `evey-data` | Database (`postgres/`), JWT key + generated secrets (`keys/`), logs, backups, plugins, search index | No - you lose your data and your signing key |
| `evey-cache` | Transcode + trickplay cache | Yes - regenerated on demand |
| `postgres-data` | Database, only when Postgres runs as its own service | No - you lose all data |

The embedded Meilisearch index lives under `evey-data` and is rebuilt from the
database on the next scan if lost. If you run the bundled Caddy reverse proxy,
its `caddy-data` volume holds the TLS certificates (re-requested if deleted).

## Apps

Android and Android TV apps connect to your server URL. Use **Quick Connect** to log in on TV - type the code on your phone instead of wrestling with a remote.

## Features

- Adaptive HLS transcoding with [hardware acceleration](docs/administration/hardware-acceleration.md) (VAAPI, NVENC, QSV)
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

You can extend Evey with plugins: themes, metadata providers, UI widgets. They run with multi-layer isolation (static analysis at install, UID drop, seccomp syscall filter, optional `LD_PRELOAD` guard). Each layer has known limits - **always review third-party plugins before installing them**. See [docs/SECURITY.md](docs/SECURITY.md) for the full model and its limits.

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
| [Installation](docs/installation.md) | The compose file, first launch, PUID/PGID |
| [Advanced deployments](docs/operations/advanced-deployments.md) | External database, ARM, image tags |
| [Configuration](docs/configuration.md) | All env variables |
| [Languages & Accessibility](docs/i18n-accessibility.md) | i18n + a11y: state & gaps |
| [Reverse Proxy](docs/reverse-proxy.md) | Caddy, Nginx, Traefik |
| [Playback](docs/playback/how-playback-is-decided.md) | How Direct Play, Remux and Transcode are chosen |
| [Hardware acceleration](docs/administration/hardware-acceleration.md) | GPU setup and calibration |
| [Backups and updates](docs/backups.md) | Schedule, restore, updating |
| [Notifications](docs/notifications.md) | SMTP & push |
| [SSO / OIDC](docs/oidc.md) | Single sign-on |
| [Plugins](docs/plugins.md) | Build your own |

## License

[GNU AGPLv3](LICENSE.md). Free software: use, modify and redistribute under the
same license, including over a network. For a commercial license under
different terms, contact the author.

Contributing means accepting [CLA.md](CLA.md) once - the dual license needs the
right to relicense. You keep the copyright on your work.
