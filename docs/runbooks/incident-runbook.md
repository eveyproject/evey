# Incident Runbook

First-response playbook for common Evey outages. Each section: **symptom → diagnose → fix**.

Containers: `evey-server`, `evey-postgres`, `evey-meilisearch`, `evey-caddy` (prod), `evey-web` + `evey-mailcatcher` (dev override). A default install has **only `evey-server`**: Postgres and Meilisearch run inside it - replace `docker exec evey-postgres psql -U ...` below with `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'`, and their logs are the `postgres:` / Meili lines in `docker logs evey-server`.

Default HTTP port: `9091` (`EVEY_HTTP_PORT`). Health endpoint: `/healthz`.

**Logs.** Two copies of the same stream: the daily file `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` inside the image) and `docker logs evey-server`. The file is authoritative: the json-file driver rotates `docker logs`, and after a rotation the command can stop hours before the container did - a load test's `stop_detail` or a calibration verdict then looks missing when it is only rotated away. When they disagree, read the file, or as root the raw `/var/lib/docker/containers/<id>/*-json.log*` files. The `docker logs` one-liners below assume a recent, unrotated window.

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
- **Rate limit tripping every request**: check `EVEY_RATELIMIT_LOGIN_MAX` (default `10` per IP per minute) and `EVEY_RATELIMIT_GLOBAL_MAX` (default `300`). Reverse proxy forwarding all traffic as a single IP will trip this fast - set `EVEY_TRUSTED_PROXIES`, restart. Dev override already sets `1000` / `10000`.
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

### 3c. "It buffers / it cut out" and the logs do not say why

**Symptom**: a viewer reports stalls or dropouts on a transcoded title, and the
server logs show `hls-vod: segment timeout` with `err:"context canceled"` -
which means the CLIENT hung up, not that the server timed out.

**Diagnose**: the server already records every decision it made. Start there,
because it is free:

```sh
# Why a segment request reused an encoder or spawned one, and what it cost the
# player. Only logged when it costs something: a wait beyond one segment, or a
# spawn. `over_client_patience: true` is the smoking gun.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# The encoders themselves: spawns, kills and their reason.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Those lines say what the server *decided*. When you need what ffmpeg actually
*achieved* second by second - the frontier's real position over time - turn on
the per-encoder progress recording:

```sh
# ON. Applies to the NEXT encoder; no restart needed. $TOKEN is an admin JWT.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduce the problem, then read one session: out_time = content produced so
# far, speed = how far ahead of real time the encoder ran.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# OFF again when you are done.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Leave it off outside an investigation.** It is one file per encoder, a few MB
per session, written for every playback on the box - real disk and real writes
on a weak NAS. The file name carries the decision hash, the rung (`-vN-`) and
the start segment (`-sN-`), so it lines up with the JSON log above; a respawn at
the same position gets its own file rather than overwriting the previous run,
which is what makes encoder churn visible.

**Common causes + fix**:
- **Repeated short forward seeks**: each one may spawn a fresh encoder, and the
  per-hash cap (`MaxSessionsPerHash`, default 2) evicts the least recently used
  one. Expect `session killed reason:lru_cap` in the log. Normal in moderation.
- **The client is less patient than the server**: `clientSegmentPatience` (6 s)
  is what the seek-reach window is sized against; AndroidX Media3 abandons a
  segment at ~8 s while the server's `WaitBudgetDur` is 60 s. A `reach decision`
  line with `over_client_patience: true` means the server was about to ask for a
  wait no player would sit through.

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
  This also drops `photos/` (resized photo variants) and `track-art/` (a
  track's tag-embedded cover, extracted once and resized) under it - both are
  rebuilt on the next request. The admin panel's "Clear cache" button does
  the same purge.
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

## 8. A cast froze, went silent, or the phone keeps spinning

Everything below is readable from the admin Logs page (or
`GET /api/v1/admin/logs/search?q=<term>&limit=200`): no shell on the box is
needed. Times are UTC. Every beacon is a `player-client: playback event
reported` line with `type=` and a `details=` string. Credentials never appear
in what these routes return (bearers, JWTs and `token=`/`password=` values read
`[redacted]`), so a cast URL's token is not something a log search can recover -
grep for the media id, not the token.

1. **Was the receiver alive, and until when?** Search `cast_receiver_alive`.
   The receiver posts one every 30 s while it plays; `details` carries
   `buf=<behind>/<ahead>/<ranges>` (seconds buffered behind and ahead of the
   playhead) and `pos_sec`. Healthy playback shows `ahead` in the tens of
   seconds. **The last beacon's time is when the receiver died or was
   killed**: nothing after it means the television is no longer running the
   receiver, whatever the phone shows.
2. **Why did it stop?** Search `cast_receiver_finished`: `reason=INTERRUPTED`
   is a new LOAD replacing the running one (look for a `StartStream` a few
   seconds earlier), `reason=ERROR` carries the player error. No `finished`
   at all after the last `alive` = the receiver process disappeared (a
   Chromecast HD killed for memory on a 20+ Mbps copy master, most often).
3. **What did the phone see?** Search `cast_sender_stall`: the sender posts
   `state=playing frozen=<n>s pos=<p>` when the Cast SDK still says PLAYING
   but the position has not moved for 20 s, and `state=idle reason=<r>` when
   the receiver reported an idle reason. These lines are logged at WARN, so
   the admin Logs page's warning filter surfaces them. This is the sender's
   side of the same minute - compare its `pos` with the receiver's last
   `pos_sec`.
4. **Did the session cascade?** Search `refresh rejected`: a refresh token
   "not found" logs the app out and back in (saved credentials), which
   re-issues the stream - the line now says the token's `jti`, `issued` and
   `age`, which tells a stale copy from a forged one. Search `StartStream`
   to see re-issues; the sender no longer re-loads the receiver on a re-issued
   URL for the same title.
5. **Was the stream too big for the device?** Search `advertises a fallback
   rung` (the copy master's peak bitrate) and `capped master served`: a
   session that started on a 20+ Mbps copy master and fell to a capped
   1080p rung after a few seconds is the floor rescue doing its job; a
   re-LOAD that starts again on the copy master is the pattern that kills a
   Chromecast HD.
6. **Server-side starvation?** Search `on-demand resource refused`: a burst
   of `context canceled` refusals on consecutive segments is a scrub (each
   request cancelled by the next), not a server fault; `served` lines with
   long durations are.

If steps 1-3 show the receiver alive and the sender silent, the problem is
in the phone's Cast session (the SDK, the network to the TV); if the
receiver's beacons stop first, the television is where to look (memory,
codec, the stream's bitrate).

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
