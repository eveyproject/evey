# Configuration

Two places, one rule. **What must exist before the server boots** - secrets,
container wiring, reverse-proxy facts - lives in the compose file. **What an
operator tunes afterwards** - log level, language, transcoding limits, hardware
acceleration, watcher, plugin sandbox and quotas - lives in **Admin > Server
config**, is stored in the database, survives a container recreate, and mostly
applies without a restart.

Setting one of those in the environment only seeds a first boot. That is why
this page does not list every `EVEY_*` the binary accepts: the rest are
first-boot seeds and escape hatches. `internal/config/config.go` is the
exhaustive reference if you need one.

## 1. What you must set yourself

Nothing, for the default install: the media path and the image tag are on the
compose file itself, and there is no `.env`. The
repository's own `docker-compose.yml` reads `MEDIA_DIR` from `.env` instead:

```env
MEDIA_DIR=/path/to/your/media
```

Either way, Postgres and Meilisearch run embedded in
the container and generate their own secrets on first boot, stored under
`./evey-data/keys` (`postgres.password`, `meili.key`, 0600). The server reads
them through `EVEY_DB_PASSWORD_FILE` / `MEILI_MASTER_KEY_FILE` (baked into the
image), which is also why `docker exec evey-server evey admin ...` needs no
password on the command line. Set `EVEY_DB_PASSWORD` / `MEILI_MASTER_KEY`
explicitly only when you bring your own database or search engine - an explicit
value always wins over the file.


```env
EVEY_SERVER_BASE_URL=https://media.example.com  # your public URL
EVEY_TMDB_API_KEY=<your-tmdb-key>               # better metadata (free at themoviedb.org)
```

## 2. A published instance, in two variables

Behind a reverse proxy, with hardware transcoding, the whole environment is two
variables. Everything else people commonly carry in a compose file is either
already in the image or a default:

```yaml
services:
  evey:
    image: gravityz/evey:latest
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 10.0.0.5/32      # your reverse proxy, /32 if you know its address
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - /srv/media:/media
    devices:
      - /dev/dri:/dev/dri                     # hardware transcoding
    group_add:
      - "991"                                 # the group owning /dev/dri/renderD128 on YOUR host
```

Lines worth **not** copying from older setups:

| Line | Why it is unnecessary |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | The image already sets these to `/data/logs`, `/cache/transcode`, `/data/trickplay`. Repeating them changes nothing. |
| `EVEY_FORWARDED_PROTO` | Deprecated - derived from the scheme of `EVEY_SERVER_BASE_URL`. Setting it only logs a deprecation warning. |
| `EVEY_KEYFRAME_BACKFILL=on` | Anything other than `off` enables it, including absent. |
| `LIBVA_DRIVER_NAME` | libva picks the driver from the GPU's PCI id. Force it only if your card needs a driver libva does not choose (verify with `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` picks the strongest confinement your host supports. Set it explicitly only when you want a *fail-closed* guarantee - `landlock` refuses to run a plugin it cannot confine, where `auto` degrades with a warning. |
| a `group_add` for `/dev/dri/card0` | Evey only ever opens `renderD128`. Check with `ls -ln /dev/dri` and add that group alone. |

## 3. The two settings the admin panel does not replace

Both of these have a look-alike in **Admin → Remote Access**, and neither
makes the environment variable redundant. This trips people up, so it is worth
being explicit.

The reason they differ is where each one lives. The remote-access block
(enable/disable, external URL, LAN ranges, remote bitrate cap) is **persisted in
the database** - its environment variables seed the first boot and are ignored
afterwards. `EVEY_SERVER_BASE_URL` and `EVEY_TRUSTED_PROXIES` are the opposite:
they are read from the environment at every boot and **nothing in the panel ever
overwrites them**. So a value that looks like a duplicate in the panel is, in
these two cases, a different setting that happens to mention an address.

**`EVEY_SERVER_BASE_URL` is not replaced by "External URL".** The panel's value
feeds the URLs the server *builds*. But whether the server trusts
`X-Forwarded-Proto` - and therefore whether it believes it is served over HTTPS -
is decided **at boot, from the environment variable only**. Drop the variable and
it falls back to `http://localhost:9091`: `Secure` cookies, redirects and every
absolute link degrade to `http://`, and share/download URLs point at localhost.
The panel value is read too late to help.

**`EVEY_TRUSTED_PROXIES` is not replaced by "LAN networks".** They are two
stages, not two names for one thing:

1. `EVEY_TRUSTED_PROXIES` decides **whose `X-Forwarded-For` is believed**, and
   therefore which address a client is given.
2. LAN networks then classify **that** address as local or remote.

The second operates on the output of the first. Without the first, every client
arrives with the proxy's address and they all classify identically - which is
exactly how the remote-access gate stayed dormant for four months (see
`internal/config/remote_access_resolve.go`). Loopback is always trusted and
cannot be removed; your entry is *added* to it.

## 4. File ownership (PUID/PGID)

**Default: automatic.** When `PUID`/`PGID` are not set, the entrypoint looks
at the owner of your `/media*` mounts at startup and runs the server as that
user, so the features that *write into* your library (the seek-index repair
rebuilds files in place) work out of the box - no `chown`, no configuration.
The startup log tells you what was picked:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Media mounts owned by **different users** are handled best-effort: the
server adopts the user owning the most mounts and joins the other mounts'
groups as supplementary groups, so repairs on those mounts work when their
files are group-writable (the startup warning lists which mounts may still
refuse writes). Auto-detection only steps aside entirely (falls back to
`1000:1000` with a warning) when every media mount is owned by root or by
the reserved uid `1001` - chown your media to a regular user, or set
`PUID`/`PGID` explicitly:

```env
PUID=1026   # run the server as this user id  (host: `id -u <media-owner>`)
PGID=100    # ...and this group id
```

Explicit values always win over detection.

How it works: the container starts as root just long enough to re-map its
internal user (and re-own Evey's own state directories `/data`, `/cache` -
**never** your media mounts), then drops privileges. Supplementary groups
granted via compose `group_add:` (e.g. your host's render group for
`/dev/dri` hardware transcoding) are preserved across the drop. The server
process itself never runs as root.

Zero-root alternative: if even that transient root is unacceptable to you,
set `user: "UID:GID"` on the service in your compose file instead. The
entrypoint detects the non-root start and skips the re-map (including the
media-owner detection) - you then own keeping `./evey-data` writable by that
UID yourself.

Notes:
- `PUID=1001` is refused: it is the internal `evey-plugin` user, kept
  distinct so plugins can never share the server's identity.
- Media is mounted read-write by default so repairs work natively; append
  `:ro` to the media volume if you prefer a read-only library (repairs will
  then fail with a clear error).

## 5. Variable reference

### Server
```env
EVEY_SERVER_PORT=9090                  # gRPC listen port (default 9090); HTTP API binds at SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose-only: host port exposed by Docker for the HTTP gateway (not read by the server binary)
EVEY_SERVER_NAME=Evey                  # display name
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### Database
```env
# Embedded (default when EVEY_DB_HOST is unset): Postgres 17 inside the container.
EVEY_EMBEDDED_POSTGRES=            # true | false; unset = embedded iff EVEY_DB_HOST is unset
EVEY_POSTGRES_DATA_DIR=/data/postgres   # cluster location inside the container (under ./evey-data)
EVEY_DB_USER=evey
EVEY_DB_NAME=evey
EVEY_DB_PASSWORD=                  # unset = generated on first boot into /data/keys/postgres.password
EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password   # read when EVEY_DB_PASSWORD is unset (image default)

# External: use these INSTEAD of the block above, not alongside it.
# Setting the host turns the embedded database off.
EVEY_DB_HOST=postgres              # the repository compose's sibling service is `postgres`
EVEY_DB_PORT=5432
EVEY_DB_PASSWORD=changeme
EVEY_DB_SSLMODE=prefer             # or disable | require | verify-full
```

The embedded Postgres listens on the container's loopback only and asks a
password over TCP; the generated one is what the server (and `evey admin`) use.
Its data directory must be on a real filesystem (ext4, btrfs, xfs, ...), not a
network share. Migrations run automatically on startup in both layouts.

### Authentication
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 days
EVEY_RATELIMIT_LOGIN_MAX=10            # per IP per minute
```

### Libraries
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # fallback polling interval when inotify misses events (default 2m); admin-editable via Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
```

The library scan prober reads `.mkv`/`.webm` and `.mp4`/`.m4v` metadata in-process
(mkvgo, head-only - ~35x faster on re-scan/repair/NAS) with no configuration: it is
always on. ffprobe stays the authority and handles any file the fast-path cannot map
field-equivalent (HDR/Dolby Vision, unmapped codecs, …). See
[adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md).

### Transcoding
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

See [transcoding.md](transcoding.md) for hardware setup.

**Dolby audio on a Chromecast** (`EVEY_CAST_WASM_AUDIO_ENABLED`, default
`false`). Evey can hand a cast receiver the original AC-3/E-AC-3 track and let
it decode in WebAssembly on the TV, avoiding a transcode. It is **off for cast
receivers** because a low-end operator box was killed mid-film, twice, with that
path in play - and its decoder measured 3.13x real time there, so it gained
nothing anyway. With it off, a cast gets the ordinary muxed master with an AAC
rendition: sound from the first second, on every device. Browsers are NOT
affected - they keep the wasm decoder, which is validated there. Set it to
`true` to re-enable it for casts.

**Prefer the admin panel.** Concurrency, hardware acceleration, AV1, the cache
budget and the advanced limits (max concurrent HLS sessions, software and
software-HDR height ceilings, one-rung-at-a-time encoding, one hardware encoder
per rung) all live under **Admin > Transcoding configuration**. Values saved
there survive restarts and take precedence over the environment; the panel says
which ones only take effect after a restart.

**Background-worker flags** are set as rows under **Admin > Server
configuration** (the panel suggests the keys, and shows the matching environment
variable and expected value for each). All are read at boot, so they apply on
restart:

| Key | Value |
| --- | --- |
| `transcoding.keyframe_backfill` | `off` to disable the keyframe index backfill (default on) |
| `transcoding.keyframe_backfill_batch` | items per pass, > 0 |
| `transcoding.keyframe_backfill_pause_ms` | milliseconds between items, >= 0 |
| `transcoding.keyframe_backfill_idle_sec` | seconds between idle passes, > 0 |
| `transcoding.level_backfill` | `off` to disable the H.264/HEVC level backfill (default on) |
| `transcoding.level_backfill_batch` | items per pass, > 0 |
| `transcoding.level_backfill_pause_ms` | milliseconds between items, >= 0 |
| `transcoding.level_backfill_idle_sec` | seconds between idle passes, > 0 |
| `transcoding.pause_bg_on_playback` | `off` to keep background work running during playback (default on: it steps aside) |
| `transcoding.snapshot_fastpath` | `off` to force the exact ffmpeg extractor instead of the mkvgo keyframe fast path |

Each also has an environment equivalent (`EVEY_KEYFRAME_BACKFILL`,
`EVEY_LEVEL_BACKFILL_BATCH`, ...), but the panel row wins.

### Plugins (sandbox & limits)
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic - auto picks the strongest your host supports
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off - run plugins as a separate user to wall off secrets (auto = on only when the effective sandbox resolves to "basic", i.e. neither Landlock nor bubblewrap); incompatible with no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # max RAM per plugin (MiB); admin-editable via Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # max processes/threads per plugin; admin-editable
EVEY_PLUGINS_DISK_MAX_MB=100           # max install-dir size per plugin (MiB; 0 = off); admin-editable
EVEY_PLUGINS_CPU_NICE=10               # scheduling nice 0-19 (higher = lower priority); admin-editable
EVEY_PLUGINS_ENABLED=true              # master switch for the plugin subsystem
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce - require a cosign signature on plugin ZIPs
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # PEM public-key path (required when policy != off)
```

Per-scenario guide: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Full model: [SECURITY.md](SECURITY.md).

### Email (SMTP)
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

See [notifications.md](notifications.md).

### SSO (OIDC)
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # comma-separated, empty = all
```

See [oidc.md](oidc.md).

### Backups
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = disabled, 24 = daily
EVEY_BACKUP_RETENTION_DAYS=30
```

See [backups.md](backups.md).

### Reverse Proxy
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12     # added to the always-trusted loopback, not a replacement
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_DOMAIN=media.example.com          # only for bundled Caddy
```

HTTPS behaviour (Secure cookies, HSTS, trusting `X-Forwarded-Proto`) follows
the scheme of `EVEY_SERVER_BASE_URL`: an `https://` base URL turns it on.
`EVEY_FORWARDED_PROTO=true|false` is deprecated and only kept as an override.

See [reverse-proxy.md](reverse-proxy.md).

### Intro detection
```env
EVEY_INTRO_TIDB_API_KEY=                # optional Bearer token for TheIntroDB (500 lookups/day vs 100 anonymous)
EVEY_INTRO_FFMPEG_THREADS=1             # threads per ffmpeg process used by intro detection (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # serve "preview" (next-episode reel) segments to the player so a "Skip Preview" button appears
EVEY_INTRO_AUTO_SKIP_ENABLED=false      # every player auto-skips detected intros (web/mobile read the flag in the segments response; cast receivers get intro segments gated by it)
```

`api.tidb_key`, `intro_providers.enabled`, `intro_providers.tidb_submit_enabled`,
`intro_providers.skip_preview_enabled` and `intro_providers.auto_skip_enabled`
can also be configured via
`/admin/server-config` (DB-first, env fallback). Restart the server after toggling
`intro_providers.enabled`; the skip-preview and intro auto-skip toggles apply live (no restart).

See [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md).

### Logging
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_DIR=/data/logs                # the image's default; the compose file maps it to ./evey-data/logs
EVEY_LOG_RETENTION_DAYS=90
```

The daily file `evey-YYYY-MM-DD.log` under `EVEY_LOG_DIR` is the authoritative
log. `docker logs evey-server` shows the same stream, but Docker's json-file
driver rotates it, and after a rotation the command can appear to stop hours
before the container did. When the two disagree, trust the file - or, as root,
the raw `/var/lib/docker/containers/<id>/*-json.log*` files. Outside the image
(a bare-metal install) the default is a `logs` directory relative to the working
directory, so point `EVEY_LOG_DIR` at a writable absolute path.

### Remote access (who may connect from outside your network)
```env
EVEY_REMOTE_ACCESS_ENABLED=                                              # unset = auto (see below); true/false to decide yourself
EVEY_REMOTE_ACCESS_EXTERNAL_URL=                                         # the public address, if it differs from EVEY_SERVER_BASE_URL
EVEY_REMOTE_ACCESS_LAN_NETWORKS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16  # CIDRs treated as LAN
EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS=0                                    # 0 = no cap for remote (WAN) sessions
```

> ⚠️ The gate protects the **HTTP** listener (`EVEY_SERVER_PORT` + 1). The native
> gRPC listener on `EVEY_SERVER_PORT` itself is an internal transport and is not
> gated, so it binds to **loopback only** (`EVEY_GRPC_BIND`, default `127.0.0.1`):
> its one client is the HTTP gateway inside the same process. Set
> `EVEY_GRPC_BIND=0.0.0.0` to expose it deliberately - boot then warns that the
> remote-access gate does not cover it, and firewalling it becomes your job.

**The gate.** When remote access is off, every client whose IP is not in
`EVEY_REMOTE_ACCESS_LAN_NETWORKS` is refused on every route with
`403 {"code":"remote_access_disabled"}`. It is a deployment choice ("this
instance is for my own network"), not a security layer - authentication is what
protects your library.

**Auto by default.** Leave `EVEY_REMOTE_ACCESS_ENABLED` unset and the server
decides at boot: if `EVEY_REMOTE_ACCESS_EXTERNAL_URL` or `EVEY_SERVER_BASE_URL`
is a publicly reachable address, remote access is **enabled** (a server you
published is meant to be reached); otherwise it stays LAN-only. The decision is
logged at startup, and `evey admin doctor` reports it.

Set the variable - or use **Admin → Remote Access** - to decide explicitly; an
explicit choice is never overridden by the inference. If you deliberately keep a
published server LAN-only, the boot log carries a warning saying so, because the
symptom otherwise reaching you is "it works on your phone but not on mine".

> Behind a reverse proxy this only works when the proxy forwards the real client
> IP and `EVEY_TRUSTED_PROXIES` covers the proxy. Otherwise every client looks
> like the proxy's own address - see [reverse-proxy.md](reverse-proxy.md).

When `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` is set, playback sessions whose
client IP is classified WAN are capped to that bitrate; the cap only ever
LOWERS the client/profile ceiling. Two safety rules to know:

- An RFC1918/private client IP is **never** capped, even if you forgot to
  list its range in `EVEY_REMOTE_ACCESS_LAN_NETWORKS` - the cap only lands on
  a positively public address.
- If you use unusual local ranges (CGNAT `100.64.0.0/10`, a VPN/Tailscale
  subnet) and do NOT want those viewers capped, list them explicitly in
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS`. Behind a reverse proxy, make sure it
  forwards the real client IP (`X-Forwarded-For`) or every session will look
  like the proxy's address.
