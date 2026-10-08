# Install

Evey runs as one container: the image serves the API and the web interface on
one port, and runs its own database (Postgres) and search engine (Meilisearch)
inside it. This page is for the person who runs Docker at home or on a NAS:
copy the compose file, start it, open **`http://<server-address>:9091`** in a
browser, and continue with the setup wizard. No `.env`, no privileged mode,
no memory or CPU setting.

!!! warning "Not the repository's docker-compose.yml"

    The `docker-compose.yml` at the root of the Git repository is a different
    layout (Postgres as a separate service, a bundled Caddy, a required `.env`).
    Use the file on this page. The other layout is described in
    [Advanced deployments](operations/advanced-deployments.md).

## Requirements

- Docker with Compose v2 (`docker compose`, not the older `docker-compose`).
- A persistent disk for Evey's data (the compose file below keeps it in
  `./evey-data`, next to the file).
- An x86-64 host, or ARM64 with a release tag: see
  [ARM hosts](operations/advanced-deployments.md#arm-hosts).

Evey serves plain HTTP. To reach it from outside your home over HTTPS, put a
reverse proxy in front of it later: see
[Remote access and HTTPS](administration/remote-access.md).

## The compose file

Save this as `docker-compose.yml` and replace the media path on the last volume
line.

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:beta-0.0.1 # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. Evey is in beta and has no numbered release yet, so there is no `latest`
   tag: `beta-0.0.1` follows the development branch and moves with every
   change. To run the same code from one pull to the next, put an immutable
   `sha-<commit>` tag here instead: see
   [Backups and updates](backups.md#updating-evey).
2. The API and the web interface, on one port, in plain HTTP. The number on
   the left is the port you will type in the browser and in the apps; keep
   `9091` unless it is already taken on the host.
3. Everything Evey owns: database, signing key, generated passwords, search
   index, timeline thumbnails, artwork, logs, backups, plugins. This is the
   folder to back up.
4. Transcoding cache: segments, resized photos and covers. It can be deleted
   at any time and refills on demand.
5. Your media. Mounted read-write on purpose: the repairs of
   [Media health](runbooks/media-health.md) rewrite files in place. Append
   `:ro` for a read-only library and those repairs will fail. More folders are
   mounted as `/media2`, `/media3`...
6. Leaves the embedded database time to stop cleanly on `docker stop`. Docker's
   default of 10 seconds is short for a large catalogue.

## First launch

```bash
docker compose up -d
docker compose logs -f evey
```

On the first start the server generates its signing key and passwords in
`./evey-data`, creates the database and applies its migrations. Nothing has to
be prepared by hand.

## The address to open next

Open **`http://<server-address>:9091`** in a browser on the same network:

| Where the browser is | Address to type |
| --- | --- |
| On the machine running Docker | `http://localhost:9091` |
| Another computer or a phone at home | `http://` + the IP of the Docker host on your home network + `:9091`, for example `http://192.168.1.20:9091` |
| Outside your home | Not yet: this needs HTTPS and a public address, see [Remote access and HTTPS](administration/remote-access.md) |

The IP of the Docker host is in your router's list of devices, or on the host
itself (`ip a` on Linux, `ipconfig` on Windows, `ifconfig` on macOS). The
port is the left-hand number of the `ports:` line in the compose file.

A server with no administrator opens on the setup wizard: continue with
[First setup](getting-started/first-setup.md#1-the-setup-wizard). Once the
administrator exists, the same address opens on the sign-in page.

!!! tip "The apps ask for the same address"

    The Android and Android TV apps ask for the server address on their first
    screen. It is exactly this address, with `http://` and the port:
    [The server address to enter](getting-started/apps.md#the-server-address-to-enter).

## Where the data lives

| Location | Contains | Can be deleted? |
| --- | --- | --- |
| `./evey-data` | Database, signing key, generated passwords, search index, timeline thumbnails, artwork, logs, backups, plugins | No. Losing it loses every account, watch history and setting. Only `meili/` inside it can go: the search index is rebuilt from the database with `evey admin reindex-search`. |
| `evey-cache` volume | Transcoded segments, resized photos and covers | Yes. Regenerated on demand. |
| Your media folders | Your files | Evey does not delete them; removing a title in Evey removes it from the library, not from the disk. It writes there in three cases: a repair run from Media health (the original is kept as a backup), subtitles downloaded from OpenSubtitles (a `.subtitles` folder next to the film), and a probe file of a few seconds that checks the folder watcher still receives events. |

## Common variations

Each of these is a few lines added to the same file.

**A second media folder.** Add `- /other/path:/media2` under `volumes:`.

**Another port on the host.** Change the left-hand number only, for example
`"8080:9091"`, and type `:8080` in the browser and the apps instead.

**File ownership.** By default the container adopts the owner of your `/media`
mount, so that repairs can write. To choose it yourself, set `PUID` and `PGID`
under `environment:` (the output of `id -u` and `id -g`). The container starts
as root only to apply that mapping; the server itself never runs as root.

**A graphics card for transcoding.** Map the device into the container: see
[Hardware acceleration](administration/hardware-acceleration.md).

**Behind a reverse proxy you already run.** See
[Remote access and HTTPS](administration/remote-access.md).

??? note "On a NAS (Synology Container Manager, QNAP Container Station)"

    - Create the `evey-data` folder before the first start. These interfaces
      refuse a bind mount whose folder does not exist, unlike the Docker
      command line. Nothing else needs creating.
    - Set `PUID` and `PGID` to the NAS user that owns the media (`id <user>`
      over SSH; on Synology typically around `1026`, group `100`). A NAS share
      is often owned by `root`, which the automatic detection never adopts.
    - Intel NAS with Quick Sync: add the `devices:` and `group_add:` lines
      described in [Hardware acceleration](administration/hardware-acceleration.md).

## What you do not need to configure

Transcoding capacity, the graphics card, the playback method for each device
and remote access are decided by the server: it measures the machine on its
first quiet moment, detects the hardware you mapped, and opens remote access
when you give it a public address. Settings that can change later (server
name, schedules, limits, API keys) are in the admin panel, not in the compose
file. The environment variables that do exist are listed in the
[Configuration reference](configuration.md).

<!-- sources
image name and tags: .github/workflows/ci.yml (docker.io/eveyproject/evey, latest gated on semver tags)
http port = grpc port + 1: cmd/evey/main.go - httpPort ; internal/config/config.go - EVEY_SERVER_PORT default 9090
repository compose is another layout: docker-compose.yml (EVEY_DB_HOST postgres, env_file .env, caddy service)
first boot: docker/entrypoint.sh ; internal/config/config.go
wizard or sign-in on the root address: frontend/apps/web/src/routes/index.tsx ; frontend/apps/web/src/routes/setup.tsx
apps ask for the same address: frontend/apps/mobile/app/server-connect.tsx ; frontend/apps/tv/app/server-connect.tsx
volumes: docs/backups.md ; cmd/evey (evey admin reindex-search)
writes into media folders: internal/app/subtitle/service.go (.subtitles) ; internal/app/library/watcher_probe.go - probeOnce ; runbooks/media-health.md
delete = library only: internal/app/library/command_service.go - DeleteMediaItem ; internal/app/music/command_service.go - DeleteTrack
remote access inference: docs/adr/0026-remote-access-inferred-from-a-published-url.md
calibration at first quiet moment: internal/app/transcoding/calibration_service.go
-->
