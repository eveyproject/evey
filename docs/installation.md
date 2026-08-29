# Installation & Operations

Production install for self-hosters running the prebuilt `gravityz/evey` image
with `docker compose` (e.g. on a NAS). The container runs **non-root** and needs
**no `--privileged`, `mem_limit`, `cpus`, or cgroup settings**
(see [adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md)).

For the bare two-file quick-start see the [README](../README.md). This page
covers the same install plus updates, backups, and day-2 operations. To run
Evey **outside Docker** (systemd / bare metal), see
[deployment-bare-metal.md](deployment-bare-metal.md).

## Prerequisites

- Docker + Docker Compose v2 (`docker compose`, not the old `docker-compose`).
- A place to store data on a persistent disk (the compose below uses bind mounts
  in the working directory).
- **Recommended:** a reverse proxy for HTTPS - Evey serves plain HTTP only. See
  [reverse-proxy.md](reverse-proxy.md).

### Architecture (x86-64 and ARM64)

Stable releases (`vX.Y.Z` tags) publish a **multi-arch** image for both
`linux/amd64` and `linux/arm64`; `docker pull`/`compose up` picks the right one
automatically, no flag needed. Pre-release tags (`-alpha`/`-beta`) and per-branch
builds are **amd64-only** - pin a stable `vX.Y.Z` in `EVEY_VERSION` for an ARM
host (a Raspberry Pi, an ARM NAS, an Ampere VPS).

Two things to know on ARM:

- **No hardware transcoding on a Synology-class ARM (Realtek) NAS.** Evey wires
  exactly three accel backends - Intel QSV, VAAPI, and NVENC. QSV and the Intel
  VAAPI drivers are x86-only (deliberately not installed on the arm64 image);
  NVENC is NVIDIA. Evey does **not** wire the ARM-native video encoders (the
  Raspberry Pi V4L2 M2M / Rockchip RKMPP VPUs), so on an ARM board there is no
  hardware encode path. In practice ARM = **direct play** (the client plays the
  file as-is, near-zero cost) plus **software transcode** on the CPU. On a weak
  ARM core, keep `transcoding.max_video_sessions` low (1-2, in Admin → Server
  Config) and lean on direct play.
- **HDR tone-mapping is on the fast path on ARM too.** The bundled arm64
  jellyfin-ffmpeg ships the SIMD `tonemapx` filter (NEON), just like the amd64
  build ships its AVX2 variant - so HDR→SDR tone-mapping is hardware-SIMD, not the
  slow zscale chain. (Evey probes `tonemapx` at runtime and falls back to zscale
  only if a build ever lacks it.)

If you build the image yourself for an unpublished arch, `docker buildx build
--platform linux/arm64` works out of the box - the Dockerfile guards the x86-only
bits behind `TARGETARCH`.

## 1. docker-compose.yml

**Default layout: one service.** The evey image serves the API + web UI on one
port **and** runs both Postgres and Meilisearch embedded - spawned by the
entrypoint as the non-root server user, loopback-only, their secrets generated
on first boot into `./evey-data/keys`. Let the image write the files:

```bash
mkdir evey && cd evey
docker run --rm -v "$PWD":/out gravityz/evey init --media /path/to/your/media
```

`evey init` writes one `docker-compose.yml` (owned by you, not root) - no
`.env`. Flags: `--port 9091` (host port), `--domain media.example.com` (adds
the Caddy reverse proxy with automatic HTTPS and a `Caddyfile`),
`--behind-proxy <network>` (you already run a proxy that owns TLS - Caddy
serves plain `:80` on that network, see [reverse-proxy.md](reverse-proxy.md)),
`--external-postgres` / `--external-meili` (the operator layout below; the
shared secrets then go to a `.env`), `--version 0.2.0-beta.1` (pin a tag),
`--force` (overwrite). Or paste the equivalent by hand:

```yaml
services:
  evey:
    image: gravityz/evey:latest        # pin a tag here (e.g. 0.2.0-beta.1, sha-1a2b3c4)
    container_name: evey-server
    ports:
      - "9091:9091"
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091   # e-mail links, OIDC redirects, cast/device URLs
    volumes:
      - ./evey-data:/data              # database, JWT key, generated secrets, search index, logs, backups
      - evey-cache:/cache              # transcode + trickplay cache (safe to prune)
      # Read-write so the seek-index repair rebuilds files in place; append
      # `:ro` for a read-only library and accept that those repairs fail.
      - /path/to/your/media:/media
    # The embedded database is stopped cleanly AFTER the server on `docker
    # stop`; a large catalogue needs more than Docker's 10s default.
    stop_grace_period: 60s
    restart: unless-stopped

volumes:
  evey-cache:
```

Keep the environment **minimal** - only secrets you bring yourself, the DB
connection when external, storage paths, and reverse-proxy facts belong in the
compose file (or in a `.env` for the secrets). Almost everything else (server
name, transcode concurrency, rate limits, log level, scan/watcher intervals,
backup schedule, API keys) is **editable at runtime from Admin → Server
Config** and should NOT be stuffed into the compose.
`PUID`/`PGID`: unset by default → the entrypoint adopts the owner of your
`/media` mounts so in-place repairs work; override with `id -u`/`id -g`. The
container starts as root only to apply the mapping, then drops privileges -
the server never runs as root.

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

### Operator layout: external Postgres

Operators who want the database lifecycle in their own hands (a managed
Postgres, pinned major version, separate volume, dump+restore on their terms)
run it as a sibling service. `evey init --external-postgres` writes this
layout with a generated password; by hand, add the service and point the
server at it - **`EVEY_DB_HOST` being set is what turns the embedded database
off**, which is why every install from before the embedded database keeps
working untouched:

```yaml
services:
  evey:
    # ... as above, plus:
    environment:
      EVEY_DB_HOST: postgres
      EVEY_DB_SSLMODE: disable          # internal Docker network; the bundled Postgres has no TLS
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

```env
EVEY_DB_PASSWORD=<strong-password>   # must NOT be the default 'evey' - the server refuses to start otherwise
```

**External Meilisearch, same idea.** A separate search container keeps its
own lifecycle and volume (and lets you `docker compose restart meilisearch` to
reset just the index). Turn the embedded one off, point the server at it, and
share the key - an explicit `MEILI_MASTER_KEY` always wins over the generated
`./evey-data/keys/meili.key`:

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
`EVEY_TRUSTED_PROXIES` / `EVEY_FORWARDED_PROTO`, and the plugin master switch
`EVEY_PLUGINS_ENABLED`. (`EVEY_PLUGINS_SANDBOX_MODE`, `EVEY_PLUGINS_UID_SEPARATION`
and the resource caps may be set here as initial defaults, but are admin-editable
at runtime from Admin → Plugins.) Full reference: [configuration.md](configuration.md).

### Plugin sandbox & hardening (what the image ships)

Plugins are third-party code that run as separate processes. The official image
is hardened so this stays the recommended containerised posture - **no
`--privileged`, no extra Docker security flags**:

- **Sandbox defaults to Landlock.** `EVEY_PLUGINS_SANDBOX_MODE=auto` (the
  default) resolves to **`landlock` whenever the kernel supports it**, which is
  the case on a default modern Docker host. `auto` resolves in the order
  **landlock → bubblewrap → basic**, picking the strongest backend the host
  allows. Landlock is the right posture for the container: it needs no
  privilege, runs under Docker's default seccomp + AppArmor with zero host
  configuration, applies a per-path filesystem allowlist (plugins see only their
  own directory, never `/data`), and is fail-closed - once applied it cannot be
  relaxed. (bubblewrap needs the `mount` syscall, which Docker's default seccomp
  blocks, so `auto` only picks it on bare metal; basic is a last-resort
  fallback.)
- **Server runs non-root** (UID 1000, `USER evey` in the image) and is marked
  **non-dumpable** (`PR_SET_DUMPABLE=0`) so a co-resident plugin can't scrape
  the server's `/proc/<pid>/{mem,environ}`.
- **Kernel seccomp on every plugin** - a denylist (ptrace, mount, unshare,
  kernel-module, kexec, setuid-family, keyring, … syscalls) is installed right
  before the plugin `execve`, so it constrains even static/compiled plugins.
  Non-network plugins additionally get `socket(AF_INET/AF_INET6)` denied.
- **Plugin UID separation without root.** On a host without Landlock, plugins can
  run under the unprivileged `evey-plugin` user (UID 1001) so they can't read the
  server's secrets (e.g. `jwt.key`) by plain file permissions. The **launcher**
  binary (not the server) carries a single `cap_setuid` file capability and drops
  the plugin itself - **not root, no `--privileged`** - so the server stays fully
  unprivileged. Controlled by `EVEY_PLUGINS_UID_SEPARATION` (default `auto`: on
  only where Landlock is absent, since under Landlock secrets are already walled
  off); best-effort, and incompatible with `no-new-privileges`.
- **Per-plugin resource caps** (memory / PID / disk / CPU nice) enforced by a
  userspace monitor - **no cgroup / `mem_limit` / `cpus` settings needed** - and
  all **admin-editable at runtime** from Admin → Plugins.

After the first start, verify the sandbox actually in force in **Admin → Plugins
→ Security Dashboard** (host sandbox card): `auto` should show `landlock` on a
modern kernel; `basic` means the host can't isolate plugin filesystem access -
see the deployment guide below.

Which setup for your machine (old NAS, bare-metal, what to enable/disable, the risks in plain words): [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Full threat model and per-layer detail: [plugins/security-model.md](plugins/security-model.md).
You can keep the default `auto` (recommended) or pin `EVEY_PLUGINS_SANDBOX_MODE:
landlock` in compose to be explicit and fail-closed (`landlock` refuses to start
a plugin if Landlock can't be enforced). Both give the same posture on a modern
kernel.

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
(your `MEDIA_DIR`, mounted read-write). The first scan starts automatically.

The compose file above mounts it read-write on purpose: the seek-index repair
rebuilds files in place, and a `:ro` mount makes those repairs fail. Add `:ro`
only if you accept losing them.

## 3. Reverse proxy (HTTPS)

Evey serves plain HTTP on 9091. `evey init --domain media.example.com` adds a
Caddy service that obtains a Let's Encrypt certificate and sets the proxy facts
in `.env` for you (ports 80/443 must reach the host). With your own proxy
(Nginx, Traefik, Nginx Proxy Manager): set `EVEY_SERVER_BASE_URL` to your public
URL and trust the proxy with `EVEY_TRUSTED_PROXIES` + `EVEY_FORWARDED_PROTO=true`.
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

# 2. write the new compose (keep a copy of the old one), e.g. behind an existing proxy:
docker run --rm -v "$PWD":/out gravityz/evey init --force \
  --media /mnt/disk_1/movies --media /mnt/disk_2/movies \
  --domain media.example.com --behind-proxy proxy-net
#    then re-add by hand what init does not know: hardware transcoding
#    (`devices: /dev/dri`, `group_add`, LIBVA_DRIVER_NAME - see transcoding.md)
#    and any EVEY_* you had set in `environment:`.

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
[external Postgres layout](#operator-layout-external-postgres).

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
docker compose ps                       # are all three services Up / healthy?
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
