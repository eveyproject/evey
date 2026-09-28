# Installation

Evey runs as **one container**: the image serves the API and the web UI on one
port and runs Postgres and Meilisearch inside it, as the non-root server user,
with their secrets generated on first boot. The compose file in section 1 is
the whole install. It needs no `.env`, no `--privileged`, and no memory or CPU
setting.

!!! warning "Not the repository's docker-compose.yml"

    The `docker-compose.yml` at the root of the Git repository is **not** that
    file. It is the operator layout (section 7): Postgres as a sibling service,
    a bundled Caddy, and a `.env` it refuses to start without. Copy the block
    below, not the repository's file.

To run Evey outside Docker, see [deployment-bare-metal.md](deployment-bare-metal.md).

## Prerequisites

- Docker + Docker Compose v2 (`docker compose`, not the old `docker-compose`).
- A place to store data on a persistent disk (the compose below uses bind mounts
  in the working directory).
- **Recommended:** a reverse proxy for HTTPS - Evey serves plain HTTP only. See
  [reverse-proxy.md](reverse-proxy.md).

## 1. The compose file

Save this as `docker-compose.yml`, with your media path on the last volume
line. Secrets land in `./evey-data/keys` on first boot; nothing to invent.

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. The image. Pin a tag here (`0.2.0-beta.1`, or an immutable `sha-1a2b3c4`) so two pulls a day apart run the same code - see [Updating Evey](#4-updating-evey).
2. The API and the web interface, on one port, in plain HTTP. For HTTPS put a [reverse proxy](reverse-proxy.md) in front and uncomment `EVEY_SERVER_BASE_URL` below with the public address: e-mail links, OIDC redirects and cast URLs are built from it.
3. Everything Evey owns: database, JWT key, generated secrets, search index, logs, backups, plugins. This is the folder to back up.
4. Transcode and trickplay cache. Safe to prune: it refills on demand.
5. Your library, left of the colon. Read-write on purpose: the seek-index repair rebuilds files in place. Append `:ro` for a read-only library and accept that those repairs fail. More folders go in as `/media2`, `/media3`...
6. On `docker stop` the embedded database is stopped cleanly after the server; a large catalogue needs more than Docker's 10 s default.

!!! tip "Keep the environment minimal"

    Only the secrets you bring yourself, the database connection when it is
    external, storage paths and reverse-proxy facts belong in the compose file (or
    in a `.env` for the secrets). Almost everything else - server name, transcode
    concurrency, rate limits, log level, scan and watcher intervals, backup
    schedule, API keys - is **editable at runtime from Admin → Server Config** and
    stays out of the compose.

!!! info "Who owns the files: PUID and PGID"

    Unset by default: the entrypoint adopts the owner of your `/media` mounts, so
    in-place repairs can write. To choose, set `PUID` and `PGID` under
    `environment:` (your `id -u` and `id -g`). The container starts as root only to
    apply that mapping, then drops privileges - the server itself never runs as
    root.

???+ note "On a NAS (Synology Container Manager, QNAP Container Station)"

    - Create the data folder **before** the first start (`mkdir evey-data` next
      to the compose, or from File Station): those UIs refuse a bind mount whose
      folder does not exist, unlike the Docker CLI. Nothing else to pre-create -
      the entrypoint makes `plugins/`, `keys/`, `postgres/` itself.
    - Set `PUID`/`PGID` to the DSM user that owns the media (`id <user>` over
      SSH; on Synology that is typically `1026`-ish and group `100` = `users`)
      rather than relying on the automatic detection: a DSM share is often owned
      by `root`, which the entrypoint never adopts.
    - Intel NAS with Quick Sync: add the `devices:` and `group_add:` lines from
      `docker-compose.gpu.yml` (the render group's GID from `getent group render`
      or `stat -c %g /dev/dri/renderD128`).
    - No `.env` is needed - the `.env`-gated `docker-compose.yml` at the root of
      the repository is the operator layout, not this one.

## 2. First launch

```bash
docker compose up -d
docker compose logs -f evey
```

On first boot the server:

1. **Generates the JWT signing key** automatically at `EVEY_JWT_PRIVATE_KEY_PATH`
   (`/data/jwt.key`, inside `./evey-data`). You do not create it by hand. Keep
   `evey-data` safe - losing the key logs everyone out.
2. **Runs database migrations automatically** (embedded, applied at startup; no
   separate migrate step). See [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

Then open `http://<your-ip>:9091`. With no admin yet, you land on the **setup
wizard** (`/setup`): create the first admin account. After that, log in and
**add a library** (Admin → Libraries), pointing it at a path under `/media`
(the folder you mounted there). The first scan starts automatically.
The wizard, the libraries, inviting people and what to check before sharing
the address are walked through in [First steps](guide/first-steps.md).

## 3. Common variations

**Variations, each a few lines on that file:**

- *A second media folder*: add `- /other/path:/media2` under `volumes:`
  (`/media3`, ... likewise). Mount them read-write, like the first one.
- *Behind a reverse proxy you already run* (Nginx Proxy Manager, Traefik, ...
  on a Docker network): remove `ports:`, join the proxy's network and tell the
  server its public URL and which peer to trust:
  ```yaml
  services:
    evey:
      environment:
        EVEY_SERVER_BASE_URL: https://media.example.com
        EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
  networks:
    default:
      name: proxy-net        # the network your proxy is on
      external: true
  ```
  The proxy then targets `evey-server:9091`. Settings per proxy:
  [reverse-proxy.md](reverse-proxy.md).
- *HTTPS with no proxy yet*: add the bundled Caddy service from
  [reverse-proxy.md](reverse-proxy.md) (automatic Let's Encrypt certificate).
- *Postgres or Meilisearch as their own services*: section 7.

**HTTPS and reverse proxies.** Evey serves plain HTTP on 9091. With no proxy yet, the bundled Caddy service
in [reverse-proxy.md](reverse-proxy.md) obtains a Let's Encrypt certificate
for you (ports 80/443 must reach the host). With your own proxy (Nginx,
Traefik, Nginx Proxy Manager): set `EVEY_SERVER_BASE_URL` to your public
`https://` URL (that scheme is what switches on Secure cookies and HSTS) and
trust the proxy with `EVEY_TRUSTED_PROXIES`.
Streaming endpoints need buffering disabled and no timeout - the ready-made
configs handle this. Full setups: [reverse-proxy.md](reverse-proxy.md).

## 4. Updating Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey      # until "healthy" (a migration on a large catalogue takes minutes)
```

Nothing else is required. The new image applies pending migrations when it
boots and dumps the database first (`./evey-data/backups/pre-migration-*`),
the container's health check turns green once it serves, and `up -d` restarts
only the `evey` service - a sibling database or proxy is never touched.

**Going back.** Put the previous tag on the `image:` line (or in
`EVEY_VERSION` if your compose interpolates it) and run `docker compose up -d`
again. If the newer version migrated the schema,
restore its pre-migration dump first - `docker exec evey-server evey admin
rollback` prints the exact steps, with the dump's path.

**Verifying what runs.** `docker exec evey-server evey version` prints the
version and commit of the running binary (`/healthz` reports the same), so
"which code is this?" never depends on a tag.

> Operators working from a checkout of the repository have the same procedure
> scripted: `scripts/evey-update.sh` (dry-run by default; state, pre-update
> dump, pull, health wait, commit check, rollback line) and
> `scripts/evey-rollback.sh`. They are conveniences for the repository's own
> deployments, not something an install needs.

**Pin what you run.** Put a release tag (`0.2.0-beta.1`) or an immutable
`sha-<short>` tag on the `image:` line; a branch tag such as
`beta-0.0.1` moves with every push, so two pulls a day apart can run different
code and "roll back to `beta-0.0.1`" goes nowhere. The script prints the tag it
launched so you can pin it.

**Image tags** (`gravityz/evey`):

- Release tags `vX.Y.Z` publish `X.Y.Z`, `X.Y`, `X`, and `latest`. `latest`
  always means the most recent semver **release**.
- Branch builds publish the branch name (e.g. `beta-0.0.1`) plus `sha-<short>`.
- **Runtime variants** - same app, different plugin runtimes bundled:
  - no suffix (default / "lean") - Python 3 only.
  - `-nodejs` - adds Node for JS plugins.
  - `-full` - adds PHP + Ruby + Lua too.
  Pick `-nodejs` / `-full` only if your plugins need those runtimes; otherwise
  the default is smallest. Set e.g. `EVEY_VERSION=latest-nodejs`.
- **GPU flavours** - the published tags ship every accelerator family (Intel
  Gen9 to current, AMD), which is about 900 MB of Intel OpenCL compilers alone.
  A self-built image can keep just its host's family with
  `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 to Gen11, the
  UHD 6xx), `GPU=intel` (Gen12 and newer), `GPU=amd`, or `GPU=none`. The image
  label `org.evey.gpu` records the choice. NVIDIA needs no flavour: no NVIDIA
  library is baked into any image, the NVIDIA Container Toolkit injects the
  host driver at run time (`--gpus all` or a compose device reservation), so
  NVENC works with every flavour, `none` included. On a box with both an Intel
  iGPU and an NVIDIA card, calibration's `auto` mode picks VAAPI first; pin
  the hardware mode to `nvenc` in the admin transcoding panel to use the card.

## 5. Backups & recovery

Enable scheduled DB backups (`EVEY_BACKUP_INTERVAL_HOURS`, stored in
`EVEY_BACKUP_DIR` → `/data/backups`, inside `evey-data`) or trigger one from the
admin panel. Backups cover all user data, not your media files. Details:
[backups.md](backups.md).

For lockout / lost-admin / restore situations, the image ships an `evey admin`
CLI (`reset-password`, `rotate-jwt-key`, `restore-backup`):
[runbooks/recovery.md](runbooks/recovery.md). Migration safety:
[runbooks/migration-runbook.md](runbooks/migration-runbook.md).

## 6. Troubleshooting

First checks when something is wrong:

```bash
docker compose ps                       # is every service Up / healthy?
docker compose logs --tail=200 evey     # server logs (migration errors, config validation)
```

- **evey won't start / config error.** With an external database the server
  refuses to boot if `EVEY_DB_PASSWORD` is still the default `evey`; with an
  external Meilisearch, if `MEILI_MASTER_KEY` is empty or a placeholder (a short
  non-default key only warns). Read the logged validation message and fix `.env`.
- **Embedded DB refuses to start.** The `evey:` lines at the top of the log say
  why: a cluster from another major (see the upgrade section above), a data
  directory that is not a Postgres cluster, or a `postgres:`-prefixed error
  from Postgres itself (disk full, permissions on `./evey-data/postgres`, a
  network-share filesystem).
- **Restart loop with `permission denied` on `/cache/...` in the log.** The
  `evey-cache` volume belongs to another uid than the one the server runs as
  (`PUID`, or the adopted media owner). Images from before 2026-09-17 only
  re-owned it when `./evey-data` changed owner too, so a data folder
  pre-created as `PUID` (the NAS case above) left the cache behind. Update the
  image; on an older one, `docker run --rm -v evey_evey-cache:/c alpine chown
  -R <PUID>:<PGID> /c` once (the volume name is `<project>_evey-cache`).
- **External DB not reachable.** Confirm Postgres is healthy and the password
  matches:
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **Search broken / empty.** Check Meilisearch health, then trigger a reindex -
  from the admin search settings, or `docker exec evey-server evey admin reindex-search`.
  The index is rebuilt from Postgres, so `meili-data` is safe to wipe.
- **Health endpoint:** `curl -sf http://localhost:9091/healthz && echo OK`.

### Which volumes are safe to delete

| Volume | Contents | Safe to delete? |
|--------|----------|-----------------|
| `evey-data` | Database (`postgres/`), JWT key + generated secrets (`keys/`), logs, backups, plugins, search index (`meili/`) | No - data, JWT key + plugins lost. `meili/` alone: yes, rebuilt with `evey admin reindex-search` |
| `postgres-data` | Database (external layout only) | No - all data lost |
| `evey-cache` | Transcode + trickplay cache | Yes - regenerated on demand |

## 7. Operator layout: external Postgres and Meilisearch

**How the embedded database behaves.** The cluster lives in
`./evey-data/postgres` (Postgres 17, `initdb` on first boot, owned by the
server user). It listens on the container's loopback only and requires the
generated password over TCP, so a plugin dropped to another uid cannot open it.
`docker stop` reaches it as an orderly fast shutdown after the server has
closed its connections. The data directory must sit on a real filesystem
(ext4, btrfs, xfs, ...) - not on a CIFS/NFS share. Backups (`evey admin`,
the admin panel, the scheduled `pg_dump`) work exactly as with an external
database. To open a SQL shell:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### External Postgres

Operators who want the database lifecycle in their own hands (a managed
Postgres, pinned major version, separate volume, dump+restore on their terms)
run it as a sibling service: add the service and point the server at it -
**`EVEY_DB_HOST` being set is what turns the embedded database off**, which is why every install from before the embedded database keeps
working untouched:

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... as above, plus:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # internal network, no TLS
        depends_on:
          postgres: { condition: service_healthy }

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
    ```

=== ".env"

    ```env
    EVEY_DB_PASSWORD=<strong-password>   # must NOT be the default 'evey' - the server refuses to start otherwise
    ```

**External Meilisearch, same idea.** A separate search container keeps its
own lifecycle and volume (and lets you `docker compose restart meilisearch` to
reset just the index). Turn the embedded one off, point the server at it, and
share the key - an explicit `MEILI_MASTER_KEY` always wins over the generated
`./evey-data/keys/meili.key`:

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... plus:
        environment:
          EVEY_EMBEDDED_MEILI: "false"
          EVEY_MEILI_URL: http://meilisearch:7700
        depends_on:
          meilisearch: { condition: service_healthy }

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
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    MEILI_MASTER_KEY=<run: openssl rand -hex 16>
    ```

Give that container a non-root `user:` and a `mem_limit` if you can - the
official image runs as root and its resident set (memory-mapped index) looks
large but is mostly reclaimable page cache. The index is rebuilt from Postgres
(`evey admin reindex-search`), so `meili-data` is expendable either way.

Only these env vars are infra/secret and **belong in compose/.env** - they are
never admin-editable: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY`, `EVEY_DB_HOST`,
`EVEY_DB_USER`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_EMBEDDED_POSTGRES`,
`EVEY_EMBEDDED_MEILI`, `EVEY_MEILI_URL`, the `*_DIR` paths,
`EVEY_JWT_PRIVATE_KEY_PATH`, `EVEY_SERVER_BASE_URL`,
`EVEY_TRUSTED_PROXIES`, and the plugin master switch
`EVEY_PLUGINS_ENABLED`. (`EVEY_PLUGINS_SANDBOX_MODE`, `EVEY_PLUGINS_UID_SEPARATION`
and the resource caps may be set here as initial defaults, but are admin-editable
at runtime from Admin → Plugins.) Full reference: [configuration.md](configuration.md).

### Moving an existing install onto the embedded database

An install that runs Postgres as a sibling service (the layout every release
before the embedded database used) can move to the single container: the
data is a dump away, everything else in `./evey-data` (JWT key, plugins,
backups, trickplay) is picked up as-is. The embedded database imports a dump
found at `./evey-data/postgres-import.sql.gz` **once, into the cluster it
creates on its first boot**, before the server runs - so the server's own
migrations then apply on top, as after any update.

```bash
# 1. dump the current database (plain SQL, gzipped - what the import expects)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. write the new compose (keep a copy of the old one): the single-service
#    file from section 1, with your variations (extra media folders, the
#    proxy block) and whatever you had in `environment:` (hardware
#    transcoding: `devices: /dev/dri`, `group_add`, LIBVA_DRIVER_NAME).

# 3. first boot: initdb, import, migrations, then the server
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. search index: an external Meilisearch's index is not carried over, rebuild it
docker exec evey-server evey admin reindex-search
```

What is NOT needed anymore: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` and the
`EVEY_DB_HOST` / `EVEY_MEILI_URL` wiring (the secrets are generated), the
`postgres` and `meilisearch` services, and API keys seeded from `.env` (they
already live in the dumped `server_config`). Keep `./postgres-data` and
`./meili-data` until you have checked the server; the old compose still runs
them if you need to go back. A dump left next to an already-initialised
cluster is reported and ignored, never merged; a failed import discards the
new cluster so the next boot retries from the dump.

### Postgres major upgrade (embedded database)

The image pins the embedded Postgres major (17 today). A newer image shipping a
newer major **refuses to open** the old cluster with a message naming this
section - it never silently runs a data directory it cannot read. The way
across is a dump with the old image and a restore with the new one:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # keep it until the restore is verified
# set the new EVEY_VERSION in .env, then:
docker compose up -d                              # initialises an empty cluster on the new major
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

Operators who prefer `pg_upgrade` or a pinned major for years use the
[external Postgres layout](#7-operator-layout-external-postgres-and-meilisearch).

## 8. ARM hosts (x86-64 and ARM64)

Stable releases (`vX.Y.Z` tags) publish a **multi-arch** image for both
`linux/amd64` and `linux/arm64`; `docker pull`/`compose up` picks the right one
automatically, no flag needed. Pre-release tags (`-alpha`/`-beta`) and per-branch
builds are **amd64-only** - pin a stable `vX.Y.Z` in `EVEY_VERSION` for an ARM
host (a Raspberry Pi, an ARM NAS, an Ampere VPS).

Two things to know on ARM:

- **Hardware transcoding on ARM: Rockchip only, and only what the calibration
  proves.** Evey wires four accel backends - Intel QSV, VAAPI, NVENC, and
  Rockchip **RKMPP** (RK3588 / RK3576 / RK3568-class SoCs, through the arm64
  jellyfin-ffmpeg's `h264_rkmpp` / `hevc_rkmpp` encoders, `drm_prime` decode,
  RGA scaling and an OpenCL tone-map on the Mali ICD when present). QSV and the
  Intel VAAPI drivers are x86-only (deliberately not installed on the arm64
  image); NVENC is NVIDIA. To use RKMPP, map the MPP service node and the DRM
  render node into the container (`/dev/mpp_service`, `/dev/dri`, and `/dev/rga`
  where the kernel exposes it); Evey detects it, runs a real encode probe, then
  benchmarks and route-proves each shape on your board before offering it -
  nothing is assumed from the SoC name, and a shape that does not prove stays on
  the CPU. RKMPP has not been validated by the project on real hardware yet: treat
  it as best-effort and check **Admin → Transcoding → Encode calibration** for what was
  certified. The Raspberry Pi **V4L2 M2M** encoder is **not** wired: on a Pi or
  a Synology-class ARM (Realtek) NAS there is no hardware encode path, so ARM =
  **direct play** (the client plays the file as-is, near-zero cost) plus
  **software transcode** on the CPU. Capacity is calibrated, not guessed: a weak
  core keeps the 720p software floor and admits one encode at a time; you may
  still lower `transcoding.max_video_sessions` (Admin → Transcoding) and lean
  on direct play.
- **HDR tone-mapping is on the fast path on ARM too.** The bundled arm64
  jellyfin-ffmpeg ships the SIMD `tonemapx` filter (NEON), just like the amd64
  build ships its AVX2 variant - so HDR→SDR tone-mapping is hardware-SIMD, not the
  slow zscale chain. (Evey probes `tonemapx` at runtime and falls back to zscale
  only if a build ever lacks it.)

If you build the image yourself for an unpublished arch, `docker buildx build
--platform linux/arm64` works out of the box - the Dockerfile guards the x86-only
bits behind `TARGETARCH`.

## 9. Plugin sandbox

Plugins are third-party code running as separate processes. The official image
contains everything needed to confine them - no `--privileged`, no extra Docker
security flag - and `EVEY_PLUGINS_SANDBOX_MODE=auto` (the default) picks the
strongest backend the host allows, Landlock on any modern kernel. The server
itself never runs as root. The layer-by-layer detail (seccomp, uid separation,
resource caps) is in [SECURITY.md](SECURITY.md); this page only tells you what
to check.

After the first start, verify the sandbox actually in force in **Admin → Plugins
→ Security Dashboard** (host sandbox card): `auto` should show `landlock` on a
modern kernel; `basic` means the host can't isolate plugin filesystem access -
see the deployment guide below.

Which setup for your machine (old NAS, bare-metal, what to enable/disable, the risks in plain words): [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Full threat model and per-layer detail: [plugins/security-model.md](plugins/security-model.md).
