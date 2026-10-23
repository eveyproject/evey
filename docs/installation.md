# Install

This page is for the person who will run Evey for the household. You need a
computer or a NAS that stays on, with Docker installed. It takes three steps:

1. Save one file, the compose file.
2. Start Evey with one command.
3. Open Evey in a browser.

Then the [First setup](getting-started/first-setup.md) wizard takes over.

Evey runs as a single container. Its database and search engine are inside
it, so there is nothing else to install, no `.env` file to write, and no
memory or CPU setting.

!!! warning "Use the file on this page, not the one in the Git repository"

    The `docker-compose.yml` at the root of the Git repository is a different
    setup, for developers and advanced installs: it needs a `.env` file and
    runs extra services. It is described in
    [Advanced deployments](operations/advanced-deployments.md).

## What you need

- **A computer or NAS that stays on**, with an x86-64 processor (Intel or
  AMD). ARM64 machines need a release tag, which does not exist yet: see
  [ARM hosts](operations/advanced-deployments.md#arm-hosts).
- **Docker with Compose v2.** The command is `docker compose`, with a space,
  not the older `docker-compose`.
- **Your media folder**: where your films, series, music or photos are.
- **Some disk space for Evey's own data** (database, artwork, search index).
  The compose file below keeps it in a folder named `evey-data`, next to the
  file.

Evey answers in plain HTTP on your home network. Watching from outside your
home needs HTTPS, which you can add later:
[Remote access and HTTPS](administration/remote-access.md).

## Step 1: Save the compose file { #the-compose-file }

Create a folder for Evey, for example `evey`, and save this file in it as
`docker-compose.yml`. Change one line: the path to your media (the line
marked 5). For example, if your films are in `/srv/media`, the line becomes
`- /srv/media:/media`.

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:latest # (1)!
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

1. The version of Evey. `latest` is the newest release. To stay on a given
   version, use a release tag such as `0.1.0`, or a `sha-<commit>` tag: see
   [Backups and updates](backups.md#updating-evey).
2. The port. The number on the left is the one you will type in the browser
   and in the apps. Keep `9091` unless another program already uses it.
3. Evey's own data: database, keys, generated passwords, search index,
   artwork, logs, backups, plugins. **This is the folder to back up.**
4. A cache of converted video, resized photos and covers. It can be deleted
   at any time and fills up again by itself.
5. Your media. Replace `/path/to/your/media` with the real folder. Inside
   Evey, this folder is called `/media`. It is writable on purpose, so that
   [Media health](runbooks/media-health.md) can repair a file when you ask.
   Add `:ro` at the end to make it read-only: Evey then cannot write there,
   and repairs fail.
6. Gives the database time to stop cleanly when the container stops.
   Docker's default of 10 seconds is short for a large collection.

## Step 2: Start Evey { #first-launch }

In a terminal, in the folder that holds the file:

```bash
docker compose up -d
```

Evey starts in the background. To watch what it is doing, run
`docker compose logs -f evey`; press `Ctrl+C` to stop watching, Evey keeps
running.

On the first start, Evey creates its passwords and its database in
`evey-data`. You have nothing to prepare.

## Step 3: Open Evey in a browser { #the-address-to-open-next }

The address is `http://`, then the address of the machine running Docker,
then `:9091`:

| Where your browser is | Address to type |
| --- | --- |
| On the machine running Docker | `http://localhost:9091` |
| On another computer or a phone at home | `http://` + the IP address of the machine running Docker + `:9091`, for example `http://192.168.1.20:9091` |
| Outside your home | Not yet. This needs HTTPS and a public address: see [Remote access and HTTPS](administration/remote-access.md). |

To find the IP address of the machine running Docker, look in your router's
list of connected devices, or run `ip a` on Linux, `ipconfig` on Windows,
`ifconfig` on macOS. If you changed the port in step 1, use the number on the
left of the `ports:` line instead of `9091`.

The first time, the address opens the setup wizard. Continue with
[First setup](getting-started/first-setup.md#1-the-setup-wizard). Once the
wizard is done, the same address opens the sign-in page.

!!! tip "Write this address down"

    Everyone at home will need it. The Android and Android TV apps ask for
    exactly this address on their first screen, with `http://` and the port:
    [Connect the apps](getting-started/apps.md#the-server-address-to-enter).

## Where your data lives { #where-the-data-lives }

| Location | What it holds | Can you delete it? |
| --- | --- | --- |
| `./evey-data` | Database, keys, generated passwords, search index, timeline thumbnails, artwork, logs, backups, plugins | **No.** Losing it loses every account, watch history and setting. The only exception is `meili/` inside it, the search index: the command `evey admin reindex-search` rebuilds it from the database. |
| `evey-cache` volume | Converted video, resized photos and covers | Yes. It fills up again by itself. |
| Your media folders | Your files | Evey never deletes them. Removing a title in Evey removes it from the library, not from the disk. |

Evey writes into your media folders in three cases only:

- a repair you start from **Media Health** (the original is kept as a
  backup);
- a subtitle downloaded from OpenSubtitles (saved in a `.subtitles` folder
  next to the film);
- a small test file, deleted right after, that checks Evey still
  notices new files.

## Common changes { #common-variations }

Each of these is a few lines in the same file.

**A second media folder.** Add `- /other/path:/media2` under `volumes:`. A
third one is `/media3`, and so on.

**Another port.** Change only the number on the left, for example
`"8080:9091"`. Then type `:8080` in the browser and in the apps.

**Who owns the files.** By default, Evey adopts the owner of your media
folder, so that repairs can write. To choose yourself, add `PUID` and `PGID`
under `environment:`, with the numbers given by `id -u` and `id -g`. The
container starts as root only to apply this; Evey itself never runs as root.

**A graphics card for video conversion.** Give the container access to the
card: see [Hardware acceleration](administration/hardware-acceleration.md).

**You already run a reverse proxy** (a program that puts HTTPS in front of
your services): see [Remote access and HTTPS](administration/remote-access.md).

??? note "On a NAS (Synology Container Manager, QNAP Container Station)"

    - Create the `evey-data` folder before the first start. These interfaces
      refuse to mount a folder that does not exist, unlike the Docker command
      line. Nothing else needs creating.
    - Set `PUID` and `PGID` to the NAS user that owns the media (`id <user>`
      over SSH; on Synology usually around `1026`, group `100`). NAS shares
      are often owned by `root`, which Evey never adopts on its own.
    - Intel NAS with Quick Sync: add the `devices:` and `group_add:` lines
      described in [Hardware acceleration](administration/hardware-acceleration.md).

## What Evey sets up by itself

You do not configure these:

- **How much video the server can convert at once.** It measures the machine
  at its first quiet moment.
- **The graphics card.** Evey detects the card you gave the container and
  checks it with a real conversion.
- **How each device plays a file.** Decided for each playback: see
  [How playback is decided](playback/how-playback-is-decided.md).
- **Access from outside your home.** Off until you give the server a public
  address.

Settings that you may change later (server name, schedules, limits, API
keys) are in the admin panel, not in the compose file. The few environment
variables that exist are listed in the
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
ARM needs a release tag, none exists yet: docs/operations/advanced-deployments.md (ARM hosts) ; git tag (none)
probe file removed right after: internal/app/library/watcher_probe.go - probeOnce
apps link label: docs/getting-started/apps.md (Connect the apps)
-->
