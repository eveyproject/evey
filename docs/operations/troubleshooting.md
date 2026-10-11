# Troubleshooting

Something is wrong and you want to know where to look first. Find your
symptom in the table, open the page or the panel section it names, and
come back here for the logs and the recovery procedures when that is not
enough.

## Where to look, by symptom

| Symptom | Look first | Then |
| --- | --- | --- |
| Nothing answers, not even at home | `docker compose ps`, then `docker compose logs --tail=200 evey` | [The server does not start](#the-server-does-not-start) below; [Server not reachable](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| The app says **Unable to reach server** or **Connection failed** | The address typed in the app: [Apps and devices](../getting-started/apps.md#the-server-address-to-enter) | Away from home only: [Remote access and HTTPS](../administration/remote-access.md) |
| Everyone is signed out, every sign-in fails | **Admin → Remote Access → Trusted proxies**, and the database in the logs | [Nobody can sign in](#nobody-can-sign-in) below |
| No picture, playback will not start, or it stops | **Stream info** in the player, then **Admin → Media Health** for that title | [Playback problems](#playback-problems) below |
| It stutters or keeps buffering | **Admin → Streaming Quality** (which titles and devices wait) and **Admin → System Health** (is the server at capacity) | [Transcoding and server capacity](../playback/transcoding.md) |
| Wrong poster, wrong title, a film that is not recognised | **Edit Metadata** or **Re-match metadata** on the title | [Fix a wrong or missing title](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| Posters missing or broken | **Admin → Artworks Cache** | [Library and posters](#library-and-posters) below |
| A plugin fails | Its **Logs** on its card in **Admin → Plugins** | [Plugin crashed](../runbooks/incident-runbook.md#a-plugin-crashed) |
| A cast froze or went silent | **Admin → Now Playing** (is the session still there) | [A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| The disk fills up | **Admin → Transcode Cache** | [Transcode cache disk full](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## The logs

The server writes its log to `/data/logs/evey-YYYY-MM-DD.log` inside the
container, which is `./evey-data/logs/` next to your compose file. One file
per day, in JSON, kept 90 days by default (`EVEY_LOG_RETENTION_DAYS`).

**Admin → Logs & Debug** reads the same files: full-text search, a **Live**
tail, and a **Where** filter that narrows to one area (player and transcode,
seek index, subtitles, intro detection, Chromecast, auth, infrastructure...).
When a viewer reports a problem, this is the first place to look.

`docker compose logs evey` shows what the container prints while it starts:
the start-up script's `evey:` lines, the embedded database and the banner. It
is the place to read when the server does not come up; once it runs, the
application log is in the file and the panel.

## `evey admin doctor`

One command checks the whole server and prints one line per check, `ok`,
`WARN` or `FAIL`, with a remedy:

```bash
docker exec evey-server evey admin doctor
```

It covers the database and its schema, the signing key, the search index and
its drift against the database, the media mounts (including whether the server
can write to each library), the cache and log directories, `ffmpeg` and
`mkvgo`, the trusted proxies, remote access and free disk space. `--repair`
applies only the safe fixes (creating and re-owning missing writable
directories). It exits non-zero when a check fails. The other commands are in
[Command line](../runbooks/cli.md).

## The server does not start

| What `docker compose logs evey` says | What it means |
| --- | --- |
| A configuration validation message | With an external database, the server refuses the default password `evey`; with an external Meilisearch, an empty or placeholder key. Fix the value named in the message. |
| Lines starting with `evey:` about Postgres | The embedded database refused to start: a cluster from another Postgres major version ([Postgres major upgrade](advanced-deployments.md#move-to-a-newer-postgres-major)), a data directory that is not a Postgres cluster, a full disk, or `./evey-data/postgres` on a network share. |
| `permission denied` on `/cache/...`, in a restart loop | The `evey-cache` volume belongs to another user than the one the server runs as. Update the image; on an image from before 2026-09-17, run once `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

A lost administrator password, a compromised signing key or a broken
database: [Recovery](../runbooks/recovery.md).

## Nobody can sign in

Every request checks the account against the database and is refused when it
cannot answer; the log says so. The other usual cause is a reverse proxy
missing from **Admin → Remote Access → Trusted proxies**: every visitor then
shares the proxy's address and one sign-in rate limit. The procedure is
[All logins return 401](../runbooks/incident-runbook.md#every-login-returns-401).

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

**Stream info** in the player names the playback method (sent as is, copied
or re-encoded, track by track) and the reasons. A title that transcodes when
you expected Direct Play is explained there: see
[How playback is decided](../playback/how-playback-is-decided.md).

- One title stops, buffers or seeks slowly: open **Admin → Media Health**. A
  file without a seek index, a damaged stretch or tracks stored in blocks
  explains it, and most of these are repaired in place. See
  [Media health](../runbooks/media-health.md).
- Several titles, when several people watch: the server may be at its
  transcoding capacity. **Admin → System Health** shows how much of it is in
  use. See [Transcoding and server capacity](../playback/transcoding.md).
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

A scan ends with **Partial scan - these folders could not be read:**
followed by paths: Evey cannot read those folders (permissions, a network
share that dropped). The rest of the library is scanned, and the titles under
an unreadable folder are kept rather than removed. Check the mount and the
owner (`PUID`/`PGID`, see [Install](../installation.md#common-variations)),
then scan again.

Search returns nothing: rebuild the index from the database with
`docker exec evey-server evey admin reindex-search`. The index can always be
rebuilt; nothing is lost.

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
-->
