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
- **Recommended:** a reverse proxy for HTTPS — Evey serves plain HTTP only. See
  [reverse-proxy.md](reverse-proxy.md).

## 1. docker-compose.yml

Three services: the Evey server (serves both the API and the web UI on one port),
Postgres, and Meilisearch. Keep the env **minimal** — only secrets, the DB/search
connection, storage paths, the JWT key path, and reverse-proxy vars belong here.
Almost everything else (server name, transcode concurrency, rate limits, log
level, scan/watcher intervals, backup schedule) is **editable at runtime from
Admin → Server Config** and should NOT be stuffed into the compose.

```yaml
services:
  evey:
    image: gravityz/evey:${EVEY_VERSION:-latest}
    container_name: evey
    ports:
      - "9091:9091"
    environment:
      EVEY_DB_HOST: postgres
      EVEY_DB_SSLMODE: disable          # internal Docker network; the bundled Postgres has no TLS
      EVEY_MEILI_URL: http://meilisearch:7700
      EVEY_JWT_PRIVATE_KEY_PATH: /data/jwt.key
      EVEY_TRANSCODING_CACHE_DIR: /cache/transcode
      EVEY_TRICKPLAY_CACHE_DIR: /cache/trickplay
      EVEY_LOG_DIR: /data/logs
      EVEY_BACKUP_DIR: /data/backups
      # Tell Evey its public URL + trust the proxy when you put one in front:
      # EVEY_SERVER_BASE_URL: https://media.example.com
      # EVEY_TRUSTED_PROXIES: 172.16.0.0/12
      # EVEY_FORWARDED_PROTO: "true"
    env_file: .env
    volumes:
      - ./evey-data:/data        # JWT key, logs, backups, plugins, avatars
      - evey-cache:/cache        # transcode + trickplay cache (safe to prune)
      - ${MEDIA_DIR}:/media:ro    # your media, read-only
    depends_on:
      postgres: { condition: service_healthy }
      meilisearch: { condition: service_healthy }
    restart: unless-stopped

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
      timeout: 3s
      retries: 5
    restart: unless-stopped

volumes:
  evey-cache:
```

`.env` (edit these three; everything else has a sane default):

```env
EVEY_DB_PASSWORD=<strong-password>          # must NOT be the default 'evey' — server refuses to start otherwise
MEILI_MASTER_KEY=<run: openssl rand -hex 16>  # >= 16 chars; required
MEDIA_DIR=/path/to/your/media
# EVEY_VERSION=latest                        # pin a tag (e.g. v0.1.0, beta-0.0.1)
```

Only these env vars are infra/secret and **belong in compose/.env** — they are
never admin-editable: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY`, `EVEY_DB_HOST`,
`EVEY_DB_USER`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_MEILI_URL`, the `*_DIR`
paths, `EVEY_JWT_PRIVATE_KEY_PATH`, `EVEY_SERVER_BASE_URL`,
`EVEY_TRUSTED_PROXIES` / `EVEY_FORWARDED_PROTO`, and the plugin master switch
`EVEY_PLUGINS_ENABLED`. (`EVEY_PLUGINS_SANDBOX_MODE`, `EVEY_PLUGINS_UID_SEPARATION`
and the resource caps may be set here as initial defaults, but are admin-editable
at runtime from Admin → Plugins.) Full reference: [configuration.md](configuration.md).

> **Embedded Meilisearch alternative.** The image can also run Meilisearch
> *inside* the evey container (`EVEY_EMBEDDED_MEILI=true`), in which case you drop
> the `meilisearch` service and its `EVEY_MEILI_URL`. The standalone service above
> is the clearer default for ops (separate lifecycle, separate volume). Either
> way `MEILI_MASTER_KEY` is required.

### Plugin sandbox & hardening (what the image ships)

Plugins are third-party code that run as separate processes. The official image
is hardened so this stays the recommended containerised posture — **no
`--privileged`, no extra Docker security flags**:

- **Sandbox defaults to Landlock.** `EVEY_PLUGINS_SANDBOX_MODE=auto` (the
  default) resolves to **`landlock` whenever the kernel supports it**, which is
  the case on a default modern Docker host. `auto` resolves in the order
  **landlock → bubblewrap → basic**, picking the strongest backend the host
  allows. Landlock is the right posture for the container: it needs no
  privilege, runs under Docker's default seccomp + AppArmor with zero host
  configuration, applies a per-path filesystem allowlist (plugins see only their
  own directory, never `/data`), and is fail-closed — once applied it cannot be
  relaxed. (bubblewrap needs the `mount` syscall, which Docker's default seccomp
  blocks, so `auto` only picks it on bare metal; basic is a last-resort
  fallback.)
- **Server runs non-root** (UID 1000, `USER evey` in the image) and is marked
  **non-dumpable** (`PR_SET_DUMPABLE=0`) so a co-resident plugin can't scrape
  the server's `/proc/<pid>/{mem,environ}`.
- **Kernel seccomp on every plugin** — a denylist (ptrace, mount, unshare,
  kernel-module, kexec, setuid-family, keyring, … syscalls) is installed right
  before the plugin `execve`, so it constrains even static/compiled plugins.
  Non-network plugins additionally get `socket(AF_INET/AF_INET6)` denied.
- **Plugin UID separation without root.** On a host without Landlock, plugins can
  run under the unprivileged `evey-plugin` user (UID 1001) so they can't read the
  server's secrets (e.g. `jwt.key`) by plain file permissions. The **launcher**
  binary (not the server) carries a single `cap_setuid` file capability and drops
  the plugin itself — **not root, no `--privileged`** — so the server stays fully
  unprivileged. Controlled by `EVEY_PLUGINS_UID_SEPARATION` (default `auto`: on
  only where Landlock is absent, since under Landlock secrets are already walled
  off); best-effort, and incompatible with `no-new-privileges`.
- **Per-plugin resource caps** (memory / PID / disk / CPU nice) enforced by a
  userspace monitor — **no cgroup / `mem_limit` / `cpus` settings needed** — and
  all **admin-editable at runtime** from Admin → Plugins.

After the first start, verify the sandbox actually in force in **Admin → Plugins
→ Security Dashboard** (host sandbox card): `auto` should show `landlock` on a
modern kernel; `basic` means the host can't isolate plugin filesystem access —
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
   `evey-data` safe — losing the key logs everyone out.
2. **Runs database migrations automatically** (embedded, applied at startup; no
   separate migrate step). See [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

Then open `http://<your-ip>:9091`. With no admin yet, you land on the **setup
wizard** (`/setup`): create the first admin account. After that, log in and
**add a library** (Admin → Libraries), pointing it at a path under `/media`
(your `MEDIA_DIR`, mounted read-only). The first scan starts automatically.

## 3. Reverse proxy (HTTPS)

Evey serves plain HTTP on 9091. Terminate TLS with a proxy in front (Caddy,
Nginx, Traefik, Nginx Proxy Manager). Set `EVEY_SERVER_BASE_URL` to your public
URL and trust the proxy with `EVEY_TRUSTED_PROXIES` + `EVEY_FORWARDED_PROTO=true`.
Streaming endpoints need buffering disabled and no timeout — the ready-made
configs handle this. Full setups: [reverse-proxy.md](reverse-proxy.md).

## 4. Updating Evey

```bash
# Back up first (see below), then:
docker compose pull && docker compose up -d
```

The new image applies any pending migrations automatically when it boots — watch
`docker compose logs -f evey` until `/healthz` returns 200. Pin a known-good
version with `EVEY_VERSION` in `.env` instead of tracking `latest`.

**Image tags** (`gravityz/evey`):

- Release tags `vX.Y.Z` publish `X.Y.Z`, `X.Y`, `X`, and `latest`. `latest`
  always means the most recent semver **release**.
- Branch builds publish the branch name (e.g. `beta-0.0.1`) plus `sha-<short>`.
- **Runtime variants** — same app, different plugin runtimes bundled:
  - no suffix (default / "lean") — Python 3 only.
  - `-nodejs` — adds Node for JS plugins.
  - `-full` — adds PHP + Ruby + Lua too.
  Pick `-nodejs` / `-full` only if your plugins need those runtimes; otherwise
  the default is smallest. Set e.g. `EVEY_VERSION=latest-nodejs`.

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

- **evey won't start / config error.** The server refuses to boot if
  `EVEY_DB_PASSWORD` is still the default `evey`, or `MEILI_MASTER_KEY` is empty
  or a default placeholder (a short non-default key only warns). Read the logged
  validation message and fix `.env`.
- **DB not reachable.** Confirm Postgres is healthy and the password matches:
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **Search broken / empty.** Check Meilisearch health, then trigger a reindex —
  from the admin search settings, or `docker exec evey evey admin reindex-search`.
  The index is rebuilt from Postgres, so `meili-data` is safe to wipe.
- **Health endpoint:** `curl -sf http://localhost:9091/healthz && echo OK`.

### Which volumes are safe to delete

| Volume | Contents | Safe to delete? |
|--------|----------|-----------------|
| `postgres-data` | Database | No — all data lost |
| `evey-data` | JWT key, logs, backups, plugins | No — JWT key + plugins lost |
| `meili-data` | Search index | Yes — rebuild from Postgres with `evey admin reindex-search` |
| `evey-cache` | Transcode + trickplay cache | Yes — regenerated on demand |
