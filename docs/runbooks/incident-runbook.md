# Incident runbook

First response for the common Evey outages, for the operator on call. Find
the section that matches the symptom, run its checks, apply the fix that
matches what you found. Every section has the same shape: **Symptom**,
**Diagnose**, **Fix**.

## Before you start

The commands target the standard install: one container, `evey-server`,
with Postgres and Meilisearch inside it. Their lines appear in the same log
as the server's (`postgres:` prefix for the database). The repository's
`docker-compose.yml` adds `evey-postgres` and `evey-caddy` containers; see
[Advanced deployments](../operations/advanced-deployments.md).

**Ports.** The HTTP API listens on `EVEY_SERVER_PORT` + 1, `9091` by default;
`/healthz` answers there. The compose `ports:` line is what publishes it.

**SQL shell** on the embedded database:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

**Logs.** The same stream exists twice: the daily file
`./evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` inside
the image) and `docker logs evey-server`. The file is authoritative: Docker's
json-file driver rotates `docker logs`, and after a rotation the command can
stop hours before the container did. When the two disagree, read the file.
The admin panel's **Logs & Debug** page searches the same stream
(`GET /api/v1/admin/logs/search?q=<term>&limit=200`); bearers, JWTs and
`token=` / `password=` values read `[redacted]` there, so search for a media
id, never for a token.

## The server does not answer

**Symptom**: clients fail to connect; `curl http://localhost:9091/healthz`
times out or is refused.

**Diagnose**:

```sh
docker ps --filter name=evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
docker logs --tail=200 evey-server
ss -ltnp | grep 9091                # is the port listening at all?
curl -v http://localhost:9091/healthz
```

**Fix**:

- Container exited: the panic is at the end of `docker logs evey-server`.
  Fix the cause, then `docker compose up -d evey`.
- Healthcheck red because the database or the search engine is down: see
  [Database errors](#database-errors) and
  [Search is degraded](#search-is-degraded).
- Port clash on the host: change the host side of the compose `ports:` line,
  or free the port.
- Caddy in front not routing (repository layout): `docker logs evey-caddy`,
  check `EVEY_DOMAIN`, and that the `evey` healthcheck is green, since Caddy
  waits for it.

## Every login returns 401

**Symptom**: every user, admins included, gets 401 from `/api/v1/auth/*`.

**Diagnose**:

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

Look for the epoch line first. Every authenticated request re-checks the
user's token epoch against the database, and that check fails closed: a
database that is slow, locked or unreachable makes every request return the
same 401 an expired token returns, which reads like a JWT problem and is not
one. The server says so:

```
auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key
```

If that line is present, go to [Database errors](#database-errors).
`/healthz` can stay green meanwhile: it pings the connection, it does not run
the epoch query. The Prometheus counter is
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Fix**:

- **A missing or changed JWT key is rarely the cause.** An unreadable key
  file (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key`) stops the server from
  starting; a missing one is regenerated. Access tokens signed by the old key
  are then refused, but the apps renew them with their refresh token, which
  is checked in the database, not against the key: users stay signed in.
  Confirm the `./evey-data` volume is mounted at `/data`; without it the
  database is missing too.
- **The rate limiter trips on every request**: `EVEY_RATELIMIT_LOGIN_MAX`
  (`10` per IP per minute) and `EVEY_RATELIMIT_GLOBAL_MAX` (`300`). A reverse
  proxy that forwards everything as a single address trips it fast: add the
  proxy to **Trusted proxies** in **Admin -> Remote Access** (applies without
  a restart; `EVEY_TRUSTED_PROXIES` is the fallback). The repository's
  `docker-compose.override.yml` raises both to `1000` / `10000` for
  development.
- **Clock skew** between the server and the clients: `date -u` on the host.

## Transcoded playback fails

**Symptom**: direct play works, playback that needs a transcode fails; the
player errors and the server log shows ffmpeg exit codes.

**Diagnose**:

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
docker exec evey-server touch /cache/transcode/.write-test && \
  docker exec evey-server rm /cache/transcode/.write-test
```

**Fix**:

- **ffmpeg missing** (a custom image): rebuild from the official
  `Dockerfile`.
- **Cache directory not writable**: the image sets
  `EVEY_TRANSCODING_CACHE_DIR=/cache/transcode` (the `evey-cache` volume) and
  `EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay`; a compose `environment:` entry
  overrides them. Outside the image the Go defaults are `/data/transcode` and
  `/cache/trickplay`.
- **Cache full**: see [The cache disk is full](#the-cache-disk-is-full).
- **Hardware acceleration misbehaving**: set **Hardware acceleration** to
  **Off (software only)** in **Admin -> Transcoding -> Encode calibration**
  (applies live and recalibrates) so every transcode runs on the processor;
  switch back to **Auto (best available)** once the hardware is fixed.
- **"The server is busy transcoding other streams"**: the live capacity is
  measured, not set; see [Sharing the
  capacity](../playback/transcoding.md#sharing-the-capacity).
  `EVEY_TRANSCODING_MAX_CONCURRENT` (**Max Concurrent** in **Admin ->
  Transcode Cache**) only covers transcode jobs outside live playback.

### It buffers or cuts out and the log does not say why

**Symptom**: a viewer reports stalls on a transcoded title; the log shows
`hls-vod: segment timeout` with `err:"context canceled"`, which means the
client hung up, not that the server timed out.

**Diagnose**: start with what the server already recorded, it is free:

```sh
# Why a segment request reused an encoder or spawned one, and what it cost
# the player. Logged only when it costs something. `over_client_patience:
# true` is the smoking gun.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# The encoders themselves: spawns, kills and their reason.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Those lines say what the server decided. To see what ffmpeg achieved second
by second, turn on the per-encoder progress log (`$TOKEN` is an admin JWT):

```sh
# ON. Applies to the next encoder, no restart.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduce, then read one session: out_time = content produced so far,
# speed = how far ahead of real time the encoder ran.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# OFF when done.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

!!! note
    Leave the progress log off outside an investigation: one file per
    encoder, a few MB per session, for every playback on the box. The file
    name carries the decision hash, the rung (`-vN-`) and the start segment
    (`-sN-`), so it lines up with the JSON log; a respawn at the same position
    gets its own file, which is what makes encoder churn visible.

**Fix**:

- **Repeated short forward seeks**: each one may spawn a fresh encoder, and
  the per-hash cap (`MaxSessionsPerHash`, 2 by default, 1 under the
  low-memory profile) evicts the least recently used one: `session killed
  reason:lru_cap` in the log. Normal in moderation.
- **The client is less patient than the server**: the seek-reach window is
  sized against `clientSegmentPatience` (6 s) while the server's own wait
  budget (`WaitBudgetDur`) is 60 s. A `reach decision` line with
  `over_client_patience: true` means the server was about to ask for a wait
  no player sits through.

### `stale_rendition` 410s, the player refetches its master mid-play

**Symptom**: the player stalls briefly then recovers; the log shows
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
and requests answered `410 Gone` with `X-Evey-Error: stale_rendition`.

This is a protection firing, not the failure itself. Every manifest URI
carries the decision hash it was built under (`fp=`). A 410 means a player
presented a URL from a session whose ABR ladder has since been reshaped;
serving it would hand the decoder bytes from another rendition. The refusal
forces a clean master refetch.

**Diagnose**: the WARN carries `sealed_fp`, `current_fp` and the media id.

- A **burst right after a deploy, a restart or a recalibration** is expected:
  live sessions outlived a capability change. Players resync within one poll.
- A **steady drip during normal operation** means something keeps reshaping
  ladders under live streams: look for capability flapping (`docker logs
  evey-server | grep -iE 'calibrat|capability'`), an admission shaper
  churning under load, or a CMCD cap override oscillating.
- Players that **loop on 410** hold a cached master (an old app build, a
  proxy caching `.m3u8`). The proxy must pass `Cache-Control: no-cache`
  through.

## Database errors

**Symptom**: the log contains `dial tcp ... connect: connection refused`,
`pq: ...`, or queries time out.

**Diagnose**:

```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**Fix**:

- **Disk full** under `./evey-data` (the cluster lives in
  `./evey-data/postgres`): free space, then `docker compose restart evey`.
- **Pool exhausted** (`too many connections`, long waits):
  `EVEY_DB_MAX_OPEN_CONNS` (default `25`). Kill stuck sessions:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Separate Postgres container** (repository layout): `docker ps --filter
  name=evey-postgres`, `docker logs --tail=200 evey-postgres`; the server's
  `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` must match the
  `postgres` service.

## Search is degraded

**Symptom**: search returns poorer results; the log shows `meilisearch
unavailable, using SQL search fallback`.

Search keeps working through SQL while Meilisearch is unreachable; only
ranking quality drops, nobody is offline. Meilisearch runs inside
`evey-server` (`EVEY_EMBEDDED_MEILI`, on by default) on the container's
loopback, port 7700.

**Diagnose**:

```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```

**Fix**:

- Unhealthy: `docker compose restart evey`.
- Index corrupted: stop the container, move `./evey-data/meili` aside, start
  again, then rebuild the index from the database:
  ```sh
  docker exec evey-server evey admin reindex-search
  ```
  The rebuild is a command, not a panel action.

## The cache disk is full

**Symptom**: transcodes fail with `no space left on device`; the healthcheck
flaps.

**Diagnose**:

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # the compose prefix may differ
```

**Fix**:

- Purge the transcode cache, it is regenerable:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  This also drops `photos/` (resized photo variants) and `track-art/`
  (covers extracted from audio tags) under it; both are rebuilt on the next
  request. **Clear All** in **Admin -> Transcode Cache** clears the same
  cache from the panel.
- Trickplay is more expensive to regenerate; purge it only if necessary:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- Lower the **Transcode cache budget (GB)** in **Admin -> Server Config ->
  Server Settings** (default `10`) if the host volume is tight. Whatever the
  budget, the sweeper keeps 2 GiB free on the cache volume.

## A plugin crashed

**Symptom**: a plugin stops responding; the log shows panics from the plugin
runner.

**Diagnose**:

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# look for: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Fix**:

1. Relaunch the plugin without touching the server: **Enable** it again in
   **Admin -> Plugins**, or `POST /api/v1/admin/plugins/<id>/enable`. This
   works when the plugin is enabled but stuck in `Error`: the start path only
   refuses a plugin that is already `Running` or `Initializing`.
2. If that does not clear it, **Disable** the plugin from the panel.
3. If the server itself is unstable: `docker compose restart evey`.
4. Plugin files live at `./evey-data/plugins/` on the host (`/data/plugins`
   in the container). Remove the broken plugin's directory, restart.
5. Send the panic trace and the plugin id to the plugin author.

## A cast froze, went silent, or the phone keeps spinning

Everything here is readable from **Admin -> Logs & Debug**; no shell on the
box is needed. Times are UTC. Every beacon is a `player-client: playback
event reported` line with `type=` and a `details=` string.

1. **Was the receiver alive, and until when?** Search `cast_receiver_alive`.
   The receiver posts one every 30 s while it plays; `details` carries
   `buf=<behind>/<ahead>/<ranges>` (seconds buffered around the playhead)
   and `pos_sec`. Healthy playback shows `ahead` in the tens of seconds. The
   last beacon's time is when the receiver died or was killed: nothing after
   it means the television is no longer running the receiver, whatever the
   phone shows.
2. **Why did it stop?** Search `cast_receiver_finished`: `reason=INTERRUPTED`
   is a new LOAD replacing the running one (look for a `StartStream` a few
   seconds earlier), `reason=ERROR` carries the player error. No `finished`
   after the last `alive` means the receiver process disappeared, most often
   a Chromecast HD killed for memory on a 20+ Mbps copy master.
3. **What did the phone see?** Search `cast_sender_stall`: the sender posts
   `state=playing frozen=<n>s pos=<p>` when the Cast SDK still says PLAYING
   but the position has not moved for 20 s, and `state=idle reason=<r>` when
   the receiver reported an idle reason. These are WARN lines, so the
   page's warning filter surfaces them. Compare its `pos` with the receiver's
   last `pos_sec`.
4. **Did the session cascade?** Search `refresh rejected`: a refresh token
   `not found` logs the app out and back in (saved credentials), which
   re-issues the stream; the line carries the token's `jti`, `issued` and
   `age`, which tells a stale copy from a forged one. Search `StartStream` to
   see the re-issues.
5. **Was the stream too big for the device?** Search `advertises a fallback
   rung` (the copy master's peak bitrate) and `plan fitted to the client's
   line` (the receiver's `maxKbps` measure or its `abr=floor` rescue; the
   transcode master that follows carries the reason `client_line_exceeded`
   or `abr_floor_requested`). A session that started on a 20+ Mbps copy
   master and fell to a fitted 1080p rung after a few seconds is the rescue
   doing its job; a re-LOAD that starts again on the copy master is the
   pattern that kills a Chromecast HD.
6. **Server-side starvation?** Search `on-demand resource refused`: a burst
   of `context canceled` refusals on consecutive segments is a scrub (each
   request cancelled by the next), not a server fault; `served` lines with
   long durations are.

If steps 1-3 show the receiver alive and the sender silent, the problem is
in the phone's Cast session (the SDK, the network to the TV). If the
receiver's beacons stop first, look at the television: memory, codec, the
stream's bitrate.

## Stop a session or a compromised device

**Symptom**: an unexpected device in **Admin -> Now Playing**, or a user
reports an unknown device in their session list.

1. Identify the session:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
       FROM play_sessions
      WHERE ended_at IS NULL
      ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Stop it from **Admin -> Now Playing**: the stop icon on the session
   (**Stop session**), then **Stop** to confirm; or
   `AdminSessionService/KillSession` over gRPC with the session id. The
   player receives a stop command first and the stream is ended 5 seconds
   later. This ends the playback only; the user stays signed in.
3. If the stream keeps running anyway, restart the container: every encoder
   stops with it.
   ```sh
   docker compose restart evey
   ```
4. If the account itself is compromised, **Admin -> Users -> Disable**: the
   refresh tokens are deleted and the access tokens revoked at once, so every
   device is signed out. Set a new password (`evey admin reset-password
   --user <email>`, see [Recovery](recovery.md#give-a-user-a-new-password)),
   then **Enable** the account again.

## A library scan is stuck

**Symptom**: a scan shows in progress for hours with no file progress in the
log.

1. Check whether the scan still logs anything; a hung scan shows idle CPU and
   no recent `scan:` line:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. The scan lock is in memory, so a restart clears it:
   ```sh
   docker compose restart evey
   ```
3. If one file causes repeated panics (stack traces name it), move it out of
   the library path, then scan again.
4. If the file watcher logs `watcher: inotify failed for path, will use
   periodic scan`, the kernel's watch limit is exhausted:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Cheatsheet

```sh
# Status of everything
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Follow every container
docker compose logs --tail=100 -f

# Effective configuration
docker exec evey-server env | grep ^EVEY_ | sort

# Database size
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Host disk
df -h ./evey-data
```

<!-- sources
ports: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND) ; cmd/evey/main.go - httpPort = Port+1 ; Dockerfile - HEALTHCHECK
logs: Dockerfile ENV EVEY_LOG_DIR ; internal/adapters/driving/http/admin_handler.go (GET /api/v1/admin/logs/search)
epoch fail-closed: internal/adapters/driving/http/auth_middleware.go, internal/adapters/driving/grpcapi/interceptors/auth.go ; internal/pkg/metrics/metrics.go - AuthFailuresTotal
rate limits: internal/config/config.go (EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_TRUSTED_PROXIES) ; docker-compose.override.yml
cache dirs: Dockerfile ENV EVEY_TRANSCODING_CACHE_DIR / EVEY_TRICKPLAY_CACHE_DIR ; docker-compose.yml ; internal/config/config.go defaults
hardware mode and calibration labels: i18n/en/admin.yaml - transcodeCalibration.hwModeLabel, hwModeOff, hwModeAuto, cardTitle
software producer: internal/app/transcoding/software_producer.go - clientSegmentPatience, WaitBudgetDur, MaxSessionsPerHash, "software: reach decision", over_client_patience ; internal/app/transcoding/software_producer_lifecycle.go - "software: session killed" ; cmd/evey/wire.go - low-memory profile, transcode.ffmpeg_progress_log, logs/ffmpeg
server-config route: proto/evey/v1/server_config.proto - SetServerConfigEntry (PUT /api/v1/admin/server-config/{key})
stale rendition: internal/adapters/driving/http/transcode_handler_vod.go - stale_rendition, sealed_fp
db pool: internal/config/config.go (EVEY_DB_MAX_OPEN_CONNS)
search fallback: cmd/evey/wire_infra.go ; cmd/evey/admin.go - adminReindexSearch ; docker/entrypoint.sh (EVEY_EMBEDDED_MEILI, MEILI_DATA_DIR)
cache budget: internal/config/config.go (EVEY_TRANSCODING_CACHE_MAX_GB) ; internal/app/transcoding/segment_cache_evict.go - DefaultMinFreeBytes ; internal/adapters/driving/http/server.go - track-art under PhotoCacheDir ; cmd/evey/http.go - PhotoCacheDir
plugins: internal/adapters/driven/pluginrunner/runner_start.go (panic/crash lines) ; internal/app/plugin/runtime_service.go - StartPlugin ; proto/evey/v1/admin_plugin.proto - EnablePlugin
cast beacons: internal/adapters/driving/http/player_event_handler.go ; internal/app/user/auth.go - "refresh rejected" ; internal/adapters/driving/http/transcode_ondemand.go ; internal/app/transcoding/resolver_line.go
sessions: internal/adapters/driven/postgres/migrations/000001_init.up.sql - play_sessions ; internal/adapters/driving/grpcapi/admin_audit.go - KillSession (5 s) ; internal/app/user/user_service.go - DisableUser
scan: internal/app/library/scan_service.go - scanning map ; internal/app/library/watcher.go - inotify warning
labels: i18n/en/admin.yaml - sidebar (logs, nowPlaying, remoteAccess, serverConfig, transcodeCache, users, plugins), nowPlaying.stopSession, transcodeCache.clearAll, transcodeCache.maxConcurrent, remoteAccess.trustedProxies, serverSettings.transcodeCacheMaxGB ; i18n/en/common.yaml - enable, disable
-->
