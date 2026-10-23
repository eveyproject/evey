# Evey

Self-hosted media server for movies, TV shows, music and photos. One container, no `.env`, hardware transcoding included, plugins that run in a kernel sandbox.

- Website: https://eveyproject.org
- Licence: GNU AGPLv3. The source is published with the public beta.

## Quick start

Save this as `docker-compose.yml` with your media path, then `docker compose up -d`:

```yaml
services:
  evey:
    image: eveyproject/evey:latest
    container_name: evey-server
    ports:
      - "9091:9091"
    volumes:
      - ./evey-data:/data              # database, JWT key, generated secrets, search index, logs, backups
      - evey-cache:/cache              # transcode cache (safe to prune)
      - /path/to/your/media:/media     # read-write so in-place repairs work; append :ro to refuse them
    stop_grace_period: 60s
    restart: unless-stopped

volumes:
  evey-cache:
```

Open `http://<your-ip>:9091`, create your account, add a library. Postgres and Meilisearch run inside the container and generate their own secrets on first start. Migrations run automatically.

Run it as the user who owns your media (`user: "1000:1000"` or the `PUID` / `PGID` variables), so that scans can read the files and in-place repairs can write them.

## Hardware transcoding

One image covers Intel Quick Sync (VAAPI / QSV), NVIDIA (NVENC) and, on arm64, Rockchip (RKMPP, best-effort). Pass the device (`/dev/dri`, the NVIDIA runtime, or the Rockchip MPP node) and Evey detects, probes, benchmarks and calibrates the box at boot to decide what it can encode and how many streams it can serve. Nothing to buy, nothing to unlock. Other ARM boards (Raspberry Pi, Realtek NAS) have no hardware encoder: those play direct and transcode in software.

## Tags

| Tag | Meaning |
|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | Stable releases. `latest` is always the newest stable release. |
| `<branch>`, `sha-<short>` | Branch builds. `sha-` tags are immutable, pin one if you follow a branch. |

Variants, same app with more plugin runtimes: no suffix (Python only), `-nodejs` (adds Node), `-full` (adds PHP, Ruby and Lua).

Multi-arch (`amd64` + `arm64`) on release tags. Branch builds are `amd64` only.

## What is in the box

- Web app, Android app, Android TV app, a TV browser client under `/tv`, Chromecast receiver.
- Direct play, remux and transcode decided once per file, HLS with a cached, fingerprinted plan.
- MKV media health: a verdict per file, in-place repair of broken seek indexes.
- Profiles with a PIN-gated parental control enforced server-side.
- Quick Connect: sign in on the TV with a code from your phone.
- Plugins isolated with Landlock, seccomp and no direct network access.
- Server settings live in the admin panel and in `/data`, so they survive a recreate.

Beta software. Read the release notes before you upgrade, and keep a backup of `./evey-data`.
