# Troubleshooting

Start with the symptom. Each entry says what to check first and where the
detailed procedure is.

## First checks

Two commands answer most questions about a server that misbehaves:

```bash
docker compose logs --tail=200 evey
docker exec evey-server evey admin doctor
```

`evey admin doctor` checks the database and its schema, the signing key, the
search index, the media mounts (including whether the server can write to each
library), the cache directories, ffmpeg, remote access and free disk space. It
prints one line per check with a remedy, and exits non-zero when one fails.

In the admin panel, **Logs & Debug** shows the same logs, searchable and
filtered by area (playback, transcoding, scan...).

## The server does not start

| What the log says | What it means |
| --- | --- |
| A configuration validation message | With an external database, the server refuses the default password `evey`; with an external Meilisearch, an empty or placeholder key. Fix the value named in the message. |
| Lines starting with `evey:` about Postgres | The embedded database refused to start: a cluster from another Postgres major version ([Postgres major upgrade](advanced-deployments.md#postgres-major-upgrade)), a data directory that is not a Postgres cluster, a full disk, or `./evey-data/postgres` on a network share. |
| `permission denied` on `/cache/...`, in a restart loop | The `evey-cache` volume belongs to another user than the one the server runs as. Update the image; on an image from before 2026-09-17, run once `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

Recovering a lost administrator password, a compromised signing key or a
broken database: [Recovery](../runbooks/recovery.md).

## It cannot be reached from outside the home

The server answers at home but not elsewhere, or the apps show a message about
remote access being disabled: see
[Remote access and HTTPS](../administration/remote-access.md). The admin page
**Remote Access** says in red when the server is reachable from the internet
while remote access is off.

Nothing answers at all, even at home: [Server not reachable](../runbooks/incident-runbook.md#1-server-not-reachable).

## Everyone is signed out, or every sign-in fails

[All logins return 401](../runbooks/incident-runbook.md#2-all-logins-return-401).
The usual cause is a lost or replaced signing key in `./evey-data`.

## A title plays transcoded when you expected Direct Play

Open **Stream info** in the player: it names the method and the reasons. See
[How playback is decided](../playback/how-playback-is-decided.md) to read them.

## Playback stops, buffers or will not start

- One title only: open **Admin → Media Health**. A file without a seek index,
  a damaged stretch or tracks stored in blocks explains stalls and slow seeks,
  and most of these are repaired in place. See [Media health](../runbooks/media-health.md).
- Several titles, when several people watch: the server may be at its
  transcoding capacity. See
  [Transcoding and server capacity](../playback/transcoding.md).
- Transcoded playback fails outright:
  [Transcoding fails](../runbooks/incident-runbook.md#3-transcoding-fails--videos-wont-play-transcoded).
- It buffers and the logs do not say why:
  [the diagnostic procedure](../runbooks/incident-runbook.md#3c-it-buffers--it-cut-out-and-the-logs-do-not-say-why).
- **Admin → Streaming Quality** shows, for real viewers over the last day or
  week, how often playback waited and which titles and devices are affected.

## No sound, or sound out of sync

- In a browser only, with a Dolby Digital (AC-3/E-AC-3) track: a reverse proxy
  that sets its own Content-Security-Policy may block the decoder the web
  player uses. See [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Out of sync on one title: **Admin → Media Health** detects audio that starts
  after the picture and tracks that disagree with each other.

## Subtitles do not appear

See [Subtitles](../playback/subtitles.md): which formats the players draw, and
which ones the server has to burn into the picture.

## Posters are missing or broken

Posters and backdrops are copied to the server during scans, into
`./evey-data/artworks`, and missing ones are retried every hour. **Admin →
Artworks Cache** shows how many still have no local copy; **Download missing
artworks** fetches them now, and **Purge corrupt variants** removes resized
posters a failed resize left empty, which are then rebuilt on demand.

## Search returns nothing

Rebuild the index from the database:
`docker exec evey-server evey admin reindex-search`. The index can always be
rebuilt; nothing is lost.

## A cast session froze or went silent

[A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).

## A plugin stopped working

[Plugin crashed](../runbooks/incident-runbook.md#7-plugin-crashed). Its own
logs are on its card in **Admin → Plugins**.

## The disk fills up

The transcoding cache is the usual cause:
[Transcode cache disk full](../runbooks/incident-runbook.md#6-transcode-cache-disk-full).
It can be cleared from **Admin → Transcode Cache** at any time.

<!-- sources
posters: internal/app/metadata/command_service_artwork.go (hourly retry) ; i18n admin.artworks.* (/data/artworks, purge variants)
doctor: cmd/evey/admin_commands.go - doctor
startup failures: previous docs/installation.md (Troubleshooting) ; docker/entrypoint.sh
remote access warning: docs/adr/0026-remote-access-inferred-from-a-published-url.md
-->
