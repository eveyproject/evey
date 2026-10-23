# Troubleshooting

Something does not work? Find what you see in the table below. The middle
column says where to look first; the last one leads to the fix. If that is
not enough, the logs and `evey admin doctor` (further down) say more.

## Where to look, by symptom

| What you see | Look first | Then |
| --- | --- | --- |
| Nothing answers, not even at home | Is Evey running? `docker compose ps`, then the last lines of its output: `docker compose logs --tail=200 evey` | [The server does not start](#the-server-does-not-start) below; [The server does not answer](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| The app says **Unable to reach server** or **Connection failed** | The address typed in the app | [Connect the apps](../getting-started/apps.md#the-server-address-to-enter); away from home only: [Remote access and HTTPS](../administration/remote-access.md) |
| Everyone is signed out, or nobody can sign in | **Admin → Remote Access**, the trusted proxies; then the logs | [Nobody can sign in](#nobody-can-sign-in) below |
| A film shows no picture, will not start, or stops | **Stream info** in the player, then **Admin → Media Health** for that title | [Playback problems](#playback-problems) below |
| It stutters or keeps loading | **Admin → Streaming Quality** (which titles and devices wait) and **Admin → System Health** (is the server too busy) | [Transcoding and server capacity](../playback/transcoding.md) |
| Wrong poster, wrong title, or a film is not recognised | **Edit Metadata** on the title | [Fix a wrong or missing title](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| Posters missing or broken | **Admin → Artworks Cache** | [Library and posters](#library-and-posters) below |
| New files do not show up | The scan result on the library's row in **Admin → Libraries** | [Library and posters](#library-and-posters) below |
| E-mails do not arrive | **Send test email** in **Admin → Server Config → Email / SMTP** | [E-mails do not arrive](#e-mails-do-not-arrive) below |
| A plugin fails | Its **Logs**, on its card in **Admin → Plugins** | [A plugin crashed](../runbooks/incident-runbook.md#a-plugin-crashed) |
| A cast froze or went silent | **Admin → Now Playing**: is the playback still listed? | [A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| The disk fills up | **Admin → Transcode Cache** | [The cache disk is full](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## The logs

The log is Evey's diary: every error is written there, with the time.

**The easy way: Admin → Logs & Debug.** It shows the log with a search box,
a **Live** view that follows new lines, and a **Where** filter for one area
(player and conversion, subtitles, intro detection, Chromecast, sign-in...).
When someone reports a problem, look here first, around the time they give
you.

??? info "Where the log files are"

    - The files are in `./evey-data/logs/`, next to your compose file
      (`/data/logs/evey-YYYY-MM-DD.log` inside the container). One file per
      day, in JSON, plus an hour-suffixed file (`evey-YYYY-MM-DD-HH.log`) when a
      file passes 50 MB (`EVEY_LOG_MAX_SIZE_MB`), kept 90 days by default
      (`EVEY_LOG_RETENTION_DAYS`).
    - `docker compose logs evey` shows something else: what the container
      prints while it starts (the start-up script's `evey:` lines, the
      database, the banner). Read it when the server does not come up. Once
      it runs, the log is in the files and the panel.

## `evey admin doctor`

One command checks the whole server. It prints one line per check, `ok`,
`WARN` or `FAIL`, with what to do:

```bash
docker exec evey-server evey admin doctor
```

??? info "What it checks"

    The database and its schema, the signing key, the search index (and
    whether it matches the database), the media folders (including whether
    Evey can write in each library), the cache and log folders, `ffmpeg` and
    `mkvgo`, the trusted proxies, remote access, and free disk space.
    `--repair` applies only the safe fixes (creating missing folders and
    giving them the right owner). It exits with an error code when a check
    fails. The other commands are in [Command line](../runbooks/cli.md).

## The server does not start

| What `docker compose logs evey` says | What it means |
| --- | --- |
| A configuration validation message | With an external database, the server refuses the default password `evey`; with an external Meilisearch, an empty or placeholder key. Fix the value named in the message. |
| Lines starting with `evey:` about Postgres | The embedded database refused to start: a cluster from another Postgres major version ([Postgres major upgrade](advanced-deployments.md#move-to-a-newer-postgres-major)), a data directory that is not a Postgres cluster, a full disk, or `./evey-data/postgres` on a network share. |
| `permission denied` on `/cache/...`, in a restart loop | The `evey-cache` volume belongs to another user than the one the server runs as. Update the image; on an image from before 2026-09-17, run once `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

A lost administrator password, a compromised signing key or a broken
database: [Recovery](../runbooks/recovery.md).

## Nobody can sign in

Two usual causes:

- **The database does not answer.** Evey checks every sign-in against it, and
  refuses when it cannot. The log says so.
- **A reverse proxy is missing from the trusted proxies** (**Admin → Remote
  Access**). Every visitor then seems to come from the proxy's address, and
  they all share one limit on sign-in attempts.

The step-by-step procedure:
[Every login returns 401](../runbooks/incident-runbook.md#every-login-returns-401).

!!! note
    A lost or replaced signing key alone signs nobody out: the apps renew
    their tokens. Ending every session on purpose is `evey admin
    rotate-jwt-key`, see [Recovery](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens).

The server answers at home but not elsewhere, or the apps say that remote
access is disabled: [Remote access and HTTPS](../administration/remote-access.md).
**Admin → Remote Access** says in red when the server is reachable from the
internet while remote access is off.

### SSO sign-in fails

The sign-in page names the reason:

- **domain not allowed**: the address is outside the card's allowed e-mail
  domains;
- **no account matches**: sign-up is off; turn on **Create accounts for new SSO
  users** or create the account;
- **account disabled**;
- **account exists but could not be linked**: the provider did not confirm the
  e-mail, linking is off, or the account is linked to another identity;
- **unavailable**: the provider cannot be reached; the card's **Status** and
  **Test connection** show the error.

An error page shown by the provider itself usually means the redirect URI
registered there is not the one the card shows. See [OIDC / SSO](../oidc.md).

## E-mails do not arrive

**Send test email** in **Admin → Server Config → Email / SMTP** sends a message
to your own address with the saved settings and shows the mail server's answer
when it fails. A TLS mode that does not match the port (`starttls` with 587,
`ssl` with 465) is the usual mistake. A host saved in the card replaces every
`EVEY_SMTP_*` variable. Notification sends that fail are logged as
`email: send notification`. See [Notifications](../notifications.md).

## Playback problems

Start with **Stream info** in the player. It says how the film is sent (as
it is, copied into another format, or converted), track by track, and why.
A title that is converted when you expected it to play as it is: see
[How playback is decided](../playback/how-playback-is-decided.md).

- **One title** stops, keeps loading, or is slow to jump to another moment:
  open **Admin → Media Health**. The usual causes are a file without an
  index for jumping, a damaged stretch, or tracks stored in large blocks.
  Most of these can be repaired. See [Media health](../runbooks/media-health.md).
- **Several titles, when several people watch**: the server may be at the
  limit of what it can convert. **Admin → System Health** shows how busy it
  is. See [Transcoding and server capacity](../playback/transcoding.md).
- Transcoded playback fails outright:
  [Transcoding fails](../runbooks/incident-runbook.md#transcoded-playback-fails).
- It buffers and the logs do not say why:
  [the diagnostic procedure](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why).
- No sound in a browser only, with a Dolby Digital (AC-3/E-AC-3) track: a
  reverse proxy that sets its own Content-Security-Policy may block the
  decoder the web player uses. See [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Sound out of sync on one title: **Admin → Media Health** detects audio that
  starts after the picture and tracks that disagree with each other.
- Subtitles do not appear: [Subtitles](../playback/subtitles.md) says which
  formats the players draw and which ones the server has to burn into the
  picture.
- **Subtitles are being prepared…** stays for minutes: the file has no index
  for that track and the server reads it through, once. See
  [The first time a subtitle is shown](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).
- Transcoded titles refuse to start or stall while titles already played
  keep working, or administrators receive **Subtitle cache full**: the disk
  is below its free-space margin. See
  [The transcoding cache](../playback/transcoding.md#the-transcoding-cache).

## Library and posters

Posters and backdrops are copied to the server during scans, into
`./evey-data/artworks`, and missing ones are retried every hour. **Admin →
Artworks Cache** shows how many still have no local copy; **Download missing
artworks** fetches them now, and **Purge corrupt variants** removes resized
posters a failed resize left empty, which are then rebuilt on demand.

A poster or a title that belongs to another film is a metadata match, not an
artwork problem: [Fix a wrong or missing title](../administration/libraries.md#fix-a-wrong-or-missing-title).

A new file normally shows up within a few minutes (see
[How new files are picked up](../administration/libraries.md#how-new-files-are-picked-up)).
If it does not, press **Scan** on the library's row in **Admin → Libraries**
and read the result.

A scan ends with **Partial scan - these folders could not be read:**
followed by paths: Evey cannot read those folders (permissions, a network
share that dropped). The rest of the library is scanned, and the titles under
an unreadable folder are kept rather than removed. Check the mount and the
owner (`PUID`/`PGID`, see [Install](../installation.md#common-variations)),
then scan again.

Search returns nothing: rebuild the search index from the database with
`docker exec evey-server evey admin reindex-search`. Nothing is lost: the
index can always be rebuilt.

## Reporting a problem

Write to **contact@eveyproject.org**. A report that can be acted on says:

- what you did, what you expected, and what happened instead;
- when it happened, to the minute: the logs are searched by time;
- the device, and the app: **Settings → About → App version** in the Android
  app, the browser and its version for the web app;
- the version of the server: `docker exec evey-server evey version`;
- for a playback problem, the title and a screenshot of **Stream info**
  (see [Reading Stream info](../playback/how-playback-is-decided.md#reading-stream-info)):
  it names the method chosen and its reasons, which is most of the diagnosis.

An administrator can add the log lines around that moment (**Admin → Logs &
Debug**, see [The logs](#the-logs)) and the report of
[`evey admin doctor`](#evey-admin-doctor). Logs name accounts and network
addresses: read them before sending.

A security flaw is not reported in a public place: write to
**security@eveyproject.org**, and give it time to be fixed before talking
about it.

<!-- sources
symptom table: i18n mobile.serverConnect.unreachable / connectionFailed ; i18n media.rematchMetadata, admin.library.refreshMetadataTooltip
logs: internal/pkg/logger/logger.go - New (file handler only outside DevMode), rotatingWriter.rotate (evey-YYYY-MM-DD.log), StartCleanup (90 days) ; internal/adapters/driving/http/admin_handler_logs.go - currentLogPath ; docker-compose.yml EVEY_LOG_DIR=/data/logs, ./evey-data:/data ; i18n admin.logs.where.*, admin.logs.live
docker compose logs: docker/entrypoint.sh (evey: lines), cmd/evey/banner.go
doctor: cmd/evey/admin_commands.go - doctor ; cmd/evey/admin_ops.go - checkResult names (database, schema, jwt.key, meilisearch, search index, media <root>, trusted proxies, remote access, disk), checkTool ffmpeg/mkvgo
sso: internal/adapters/driving/http/auth_handler.go /login?oidc_error=<reason>, i18n auth.login.ssoError.* ; smtp: internal/adapters/driving/http/smtp_test_handler.go (test send), internal/adapters/driven/email/notification_emailer.go (log)
partial scan: internal/app/library/scan_service.go - scanDirectories (unreadable root = failed path, protected from pruning) ; i18n admin.libraries.scanPartial
posters: internal/app/metadata/command_service_artwork.go (hourly retry) ; i18n admin.artworks.* (/data/artworks, purge variants)
rotate-jwt-key ends sessions: cmd/evey/admin.go - endAllSessions
startup failures: docker/entrypoint.sh ; cache ownership: docker/entrypoint.sh (re-own /cache)
remote access warning: docs/adr/0026-remote-access-inferred-from-a-published-url.md
reporting: docs/privacy.md (contact@eveyproject.org) ; docs/SECURITY.md (security@eveyproject.org) ; app version: frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx, i18n mobile.settings.appVersion ; server version: cmd/evey/cli.go
new files row: i18n admin.libraries.scan, scanPartial ; docs/administration/libraries.md (How new files are picked up) ; e-mail row: i18n admin.serverConfig.sendTestEmail
-->
