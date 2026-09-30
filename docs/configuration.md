# Configuration reference

Two places, one rule. **What must exist before the server starts** (secrets,
storage, the public address, reverse-proxy facts) goes in the compose file.
**What an administrator tunes afterwards** (languages, transcoding limits,
hardware acceleration, schedules, plugin limits) lives in the admin panel, is
stored in the database, survives a container update, and mostly applies
without a restart. For a setting that exists in both places, an environment
variable only seeds the first start: the value saved in the panel wins.

The variables, by topic:

| Page | Covers |
| --- | --- |
| [Server and network](environment/server.md) | Public address, ports, reverse proxy, remote access, request limits, container user |
| [Database and search](environment/database-search.md) | Embedded or external Postgres and Meilisearch |
| [Accounts and sign-in](environment/accounts.md) | Session lifetimes, signing key, single sign-on |
| [Libraries and metadata](environment/libraries.md) | Scanning, metadata keys, intro detection |
| [Transcoding and hardware](environment/transcoding.md) | Graphics card, capacity, quality limits, cache, calibration clip |
| [Playback delivery and downloads](environment/playback-delivery.md) | Stream packaging switches, prepared downloads, player skip buttons |
| [Background work](environment/background.md) | Background passes and their pacing |
| [Plugins and sandbox](environment/plugins.md) | Sandbox mode, separate user, resource limits, signatures |
| [E-mail and notifications](environment/notifications.md) | SMTP, browser push keys |
| [Backups, logs and monitoring](environment/logs-monitoring.md) | Backups, log, history retention, metrics and tracing |

## Secrets

The embedded Postgres and Meilisearch generate their secrets on the first
start, in `./evey-data/keys` (`postgres.password`, `meili.key`, mode 0600). The
server reads them through `EVEY_DB_PASSWORD_FILE` and `MEILI_MASTER_KEY_FILE`,
set in the image, which is also why `evey admin` commands need no password.
Set `EVEY_DB_PASSWORD` or `MEILI_MASTER_KEY` only for an external database or
search engine; an explicit value wins over the file.

A TMDB key is built in. `EVEY_TMDB_API_KEY` seeds your own; the key set in
**Admin → Server Config → API Keys** wins over it.

## Variables you do not need

Lines often carried over from older setups, and why they change nothing:


| Line | Why it is unnecessary |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | The image already sets these to `/data/logs`, `/cache/transcode`, `/data/trickplay`. Repeating them changes nothing. |
| `EVEY_FORWARDED_PROTO` | Deprecated - derived from the scheme of `EVEY_SERVER_BASE_URL`. Setting it only logs a deprecation warning. |
| `EVEY_KEYFRAME_BACKFILL=on` | Anything other than `off` enables it, including absent. |
| `LIBVA_DRIVER_NAME` | libva picks the driver from the GPU's PCI id. Force it only if your card needs a driver libva does not choose (verify with `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` picks the strongest confinement your host supports. Set it explicitly only when you want a *fail-closed* guarantee - `landlock` refuses to run a plugin it cannot confine, where `auto` degrades with a warning. |
| a `group_add` for `/dev/dri/card0` | Evey only ever opens `renderD128`. Check with `ls -ln /dev/dri` and add that group alone. |


## File ownership (PUID and PGID)

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

```env title=".env"
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

## Variables the server does not read

Some variables appear in compose files without being read by the server:

- `EVEY_HTTP_PORT`, `EVEY_DOMAIN`, `MEDIA_DIR`: used by the repository's own
  `docker-compose.yml` (published port, the bundled Caddy's domain, the media
  path), not by Evey. `EVEY_VERSION` is the image tag there; the server only
  shows it on `/healthz` when its binary carries no version.
- `EVEY_WEB_DIR` (`/srv`, where the web interface files are), and
  `EVEY_DB_PASSWORD_FILE`, `MEILI_MASTER_KEY_FILE`, `EVEY_IMAGE_REPO`,
  `EVEY_IMAGE_RUNTIMES`: match the image's layout. Changing them is not
  supported.
- `EVEY_SUPERVISED`, `EVEY_COMPANION_PIDS`, `EVEY_RESTART_EXIT_CODE`,
  `EVEY_LAUNCHER_BIN`, `EVEY_PLUGIN_LAUNCHER`: set by the entrypoint and the
  plugin launcher for their own use.

<!-- sources
panel precedence: cmd/evey/main.go - loadOverride
variables: internal/config/config.go ; docker/entrypoint.sh ; Dockerfile ; docker-compose.yml
-->
