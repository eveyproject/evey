# Incident Runbook

First-response playbook for common Evey outages. Each section: **symptom → diagnose → fix**.

Containers: `evey-server`, `evey-postgres`, `evey-meilisearch`, `evey-caddy` (prod), `evey-web` + `evey-mailcatcher` (dev override). A default install has **only `evey-server`**: Postgres and Meilisearch run inside it - replace `docker exec evey-postgres psql -U ...` below with `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'`, and their logs are the `postgres:` / Meili lines in `docker logs evey-server`.

Default HTTP port: `9091` (`EVEY_HTTP_PORT`). Health endpoint: `/healthz`.

## 1. Server not reachable

**Symptom**: clients fail to connect; `curl http://localhost:9091/healthz` times out or returns connection refused.

**Diagnose**:
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # port actually listening?
curl -v http://localhost:9091/healthz
```

**Common causes + fix**:
- Container exited → `docker logs evey-server` for the panic, fix, `docker compose up -d evey`.
- Healthcheck failing because DB or Meili is down → see sections 4 and 5.
- Port clash on host → change `EVEY_HTTP_PORT` or free the port.
- Caddy in front not routing → `docker logs evey-caddy`, verify `EVEY_DOMAIN` and that `evey` healthcheck is green (Caddy depends on it).

## 2. All logins return 401

**Symptom**: every user - including admins who logged in yesterday - gets 401 from `/api/v1/auth/*`.

**Diagnose**:
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Look for the epoch line first.** Every authenticated request re-checks the
user's token epoch against the database (ADR-0019), and that check fails
CLOSED. So a database that is slow, locked or unreachable makes every request
in the server return the same 401 an expired token returns - which reads
exactly like a JWT problem and is not one. The server says so explicitly:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

If that line is present, stop reading this section and go to §1 (database).
`/healthz` can still be green here: it only pings the connection, it does not
run the epoch query. The Prometheus counter is
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Common causes + fix**:
- **JWT key missing or changed**: `EVEY_JWT_PRIVATE_KEY_PATH` (default `/data/jwt.key`) not readable, or was rotated. Every token signed by the old key is now invalid - there is no mitigation beyond restoring the old key from backup or forcing every user to re-login. Confirm the `evey-data` volume is mounted correctly at `/data`.
- **Rate limit tripping every request**: check `EVEY_RATELIMIT_LOGIN_MAX` (default `10` per IP per minute) and `EVEY_RATELIMIT_GLOBAL_MAX` (default `300`). Reverse proxy forwarding all traffic as a single IP will trip this fast - set `EVEY_TRUSTED_PROXIES` and `EVEY_FORWARDED_PROTO=true`, restart. Dev override already sets `1000` / `10000`.
- Clock skew between server and clients → `date -u` on the host.

## 3. Transcoding fails / videos won't play transcoded

**Symptom**: playback works for direct play, fails when a transcode is required. Player errors, server logs show ffmpeg exit codes.

**Diagnose**:
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**Common causes + fix**:
- **ffmpeg binary missing** (custom image without it): rebuild from the official `Dockerfile`.
- **Cache dir not writable**: `EVEY_TRANSCODING_CACHE_DIR` defaults to `/data/transcode` in code but compose maps it to `/cache/transcode` via the `evey-cache` named volume. `EVEY_TRICKPLAY_CACHE_DIR` defaults to `/cache/trickplay`. Verify writability:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Cache full** → see section 6.
- **HW accel misconfigured**: set `EVEY_TRANSCODING_HW_ACCEL_ENABLED=false` to force software, restart, confirm the regression is HW-related before re-enabling.
- **Too many concurrent transcodes**: `EVEY_TRANSCODING_MAX_CONCURRENT` (default `2`). Raise cautiously if the box has cores.

### 3b. `stale_rendition` 410s / player refetches its master mid-play

**Symptom**: player briefly stalls then recovers; logs show
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
and requests answered `410 Gone` with `X-Evey-Error: stale_rendition`.

**What it means**: this is a *protection firing*, not the failure itself. Every
manifest URI carries the decision hash it was built under (`fp=`). A 410 means
a player presented a URL from a session whose ABR ladder has since been
reshaped - serving it would hand the decoder bytes from a different rendition
(the "rainbow screen" incident class, ADR 0022 §1b). The refusal forces a clean
master refetch instead.

**Diagnose**: the WARN carries `sealed_fp` vs `current_fp` and the media id.
- A **burst right after a deploy/restart or an admin recalibration** is
  expected: live sessions outlived a capability change. No action; players
  resync within one poll.
- A **steady drip during normal operation** means something keeps reshaping
  ladders under live streams - look for capability flapping
  (`docker logs evey-server | grep -iE 'calibrat|capability'`), an admission
  shaper churning under load, or a CMCD cap override oscillating.
- Players that **loop on 410 without recovering** are holding a cached master
  (old app build, aggressive proxy caching of `.m3u8`). Verify the proxy does
  not cache manifests (`Cache-Control: no-cache` must pass through).

## 4. Database connection refused / errors

**Symptom**: server logs contain `dial tcp ... connect: connection refused`, `pq: ...`, or queries time out.

**Diagnose**:
```sh
docker ps --filter name=evey-postgres
docker inspect --format='{{.State.Health.Status}}' evey-postgres
docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
docker logs --tail=200 evey-postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"
```

**Common causes + fix**:
- Postgres container down → `docker compose up -d postgres`, watch logs for corruption/oom.
- Wrong credentials → server env must match `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` used by the `postgres` service. Defaults: `evey` / `evey` / `evey`.
- Pool exhausted (`too many connections` or long waits) → check `EVEY_DB_MAX_OPEN_CONNS` (default `50`). Kill stuck sessions:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- Disk full on `postgres-data/` → `df -h`, free space, restart.

## 5. Meilisearch down

**Symptom**: search returns degraded results, logs show `meilisearch unavailable, using SQL search fallback`.

**Behavior**: Evey has a resilient fallback - search keeps working via SQL (`cmd/evey/wire.go` wires a resilient search adapter that logs `meilisearch unavailable, using SQL search fallback` when Meili is unreachable). No user action needed to stay online; only ranking quality degrades.

**Note on deployment mode**: The default `docker-compose.yml` runs Meilisearch **embedded inside the `evey-server` container** (`EVEY_EMBEDDED_MEILI: "true"`). A standalone `evey-meilisearch` container only exists in the dev override (`docker-compose.override.yml`). In embedded mode, "Meilisearch down" means the evey-server process is unhealthy - there is no separate container to restart.

**Diagnose + fix (embedded, default)**:
```sh
docker ps --filter name=evey-server
curl -sf http://localhost:7700/health          # port forwarded from inside evey-server
docker logs --tail=200 evey-server | grep -i meili
```
- Server unhealthy → `docker compose restart evey-server`.
- Index corruption → stop evey-server, backup `./evey-data/meili-data/`, wipe, restart, trigger a full re-index from the admin panel.

**Diagnose + fix (standalone dev override only)**:
```sh
docker ps --filter name=evey-meilisearch
curl -sf http://localhost:7700/health
docker logs --tail=200 evey-meilisearch
```
- Container down → `docker compose up -d meilisearch`.
- `MEILI_MASTER_KEY` mismatch → server logs show auth failures. Align the env on both services and restart.
- Index corruption → stop Meili, backup `./meili-data/`, wipe, restart, trigger a full re-index from the admin panel.

## 6. Transcode cache disk full

**Symptom**: transcodes fail with `no space left on device`; container healthcheck flaps.

**Diagnose**:
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose-mangled name may vary
```

**Fix**:
- Safe purge - the cache is regenerable:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
- Trickplay is more expensive to regenerate; only purge if necessary:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/* /data/trickplay/*'
  ```
- Lower `EVEY_TRANSCODING_CACHE_MAX_GB` (default `50`) if the host volume is smaller than that.

## 7. Plugin crashed

**Symptom**: a plugin stops responding; server logs show panics from plugin runner.

**Diagnose**:
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Look for: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Fix**:
- Try a surgical restart first, no container downtime: `POST /api/v1/admin/plugins/<id>/enable`. This works even when the plugin is already `enabled` but stuck in `Error` status - `StartPlugin`'s guard relaunches it as soon as the status is not `Running`. Prefer this over restarting the whole server, especially when SSH access to the host is not readily available.
- If the surgical restart does not clear it, disable the offending plugin from the admin panel instead.
- If the server itself is unstable, restart: `docker compose restart evey`.
- Inspect plugin files at `./evey-data/plugins/` (host) == `/data/plugins` (container). Remove the broken plugin zip/dir, restart.
- Send the panic trace + plugin id to the plugin author.

## 8. WSL git corruption (dev host)

**Symptom**: on a WSL2 dev host, after a hard Windows reboot or WSL crash, git commands fail: `loose object XX is corrupt`, `object file is empty`, `git fetch` errors.

**Why**: git writes objects in two phases; a WSL kill between fopen and fwrite leaves zero-byte object files.

**Fix** (from `memory/a project note`):
```sh
# 1. Identify zero-byte objects (use absolute find if rtk intercepts the alias)
/usr/bin/find .git/objects -type f -size 0

# 2. Quarantine (do not delete)
mkdir -p /tmp/evey-corrupt-backup
/usr/bin/find .git/objects -type f -size 0 \
  -exec mv {} /tmp/evey-corrupt-backup/ \;

# 3. Check what broke
git fsck

# 4. Prune dangling worktree refs if needed
git update-ref -d refs/heads/<dead-branch>
rm -rf .git/worktrees/<dir> .claude/worktrees/<dir>
git worktree prune

# 5. If a commit poisoned HEAD: reset, expire reflog, gc, re-add, re-commit
git reset HEAD~1
git reflog expire --expire=now --all && git gc --prune=now

# 6. Verify
git fsck && git fetch origin
```

If the files on disk in a worktree are intact but the commit is toast, `cp` them into the main repo and re-commit there.

## Kill a compromised session / device

Symptom: admin sees an unexpected device in **Now Playing** (admin panel), or a user reports an unknown device in their session list.

1. Identify the `session_id`:
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     SELECT id, user_id, profile_id, device, ip_address, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"
   ```
2. Kill from admin UI: **Admin → Activity → Now Playing → ⋯ → Kill session**, or via gRPC `AdminSessionService/KillSession` with the `session_id`. The session is terminated 5s later so the player can receive the stop command.
3. If the session is unresponsive, force-delete the HLS session (directory is `<cacheDir>/<mediaItemID>`, default `/cache/transcode/<mediaItemID>`):
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. If the refresh token should also be revoked (suspected password compromise):
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     DELETE FROM refresh_tokens WHERE user_id = '<user_id>';"
   ```
   All devices for that user will need to log in again. For full lockdown, reset the password via `evey admin reset-password --user <username>`.

## Unblock a stuck library scan

Symptom: a library scan shows "in progress" for hours but no file progress in logs.

1. Check the `scan_service.scanning` map indirectly - if the process is hung, CPU is idle and recent logs show no `scan_service`:
   ```sh
   docker compose logs --tail=500 evey-server | grep scan_service
   ```
2. A scan lock is **in-memory** (no DB row), so a server restart clears it:
   ```sh
   docker compose restart evey-server
   ```
3. If a specific file is causing repeated panics (see stack traces), quarantine it by moving it out of the library path, then re-scan.
4. If inotify watches are exhausted (kernel limit), bump:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## General diagnostic cheatsheet

```sh
# Status of everything
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Tail every container
docker compose logs --tail=100 -f

# Server config dump
docker exec evey-server env | grep ^EVEY_ | sort

# Postgres size / locks
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"

# Host disk
df -h . ./postgres-data ./meili-data ./evey-data
```
