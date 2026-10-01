# Monitoring and logs

Where to see what the server is doing, from the most immediate to the most
aggregated. For a symptom to investigate, start with
[Troubleshooting](troubleshooting.md).

## What is playing now

**Admin → Now Playing** lists every stream in progress: who, what, on which
device, and how, track by track (copied, transcoded, and the backend encoding
it). An administrator can **Stop** a session or **Send message** to the
viewer.

The dashboard's **Who's Online** card lists who is connected right now, by
profile, with how long ago each app was last heard from. A Chromecast whose
television keeps Evey's receiver loaded stays listed after its playback has
ended and left **Now Playing**; **Stop this Chromecast** sends it a stop
command, which the television applies within 30 seconds.

**Admin → System Health** shows the processor, memory, bandwidth and storage,
and how much of the transcoding capacity is in use.

<figure class="evey-shot" markdown="span">
  ![Admin → System Health on the demo server](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Admin → System Health</strong>, with the background work running under it.</figcaption>
</figure>

## Logs

**Admin → Logs & Debug** searches the server's logs and follows them live,
filtered by area (playback, transcoding, scan, subtitles...). It is the first
place to look when a user reports a problem. The same logs are available with
`docker compose logs evey`.

## How playback actually went

**Admin → Streaming Quality** is built from what the players report while they
play (buffer level, rebuffering, throughput, in the CMCD format): how often
viewers waited, the titles that stall most, and the kinds of device that
struggle, over the last day or week.

The server also acts on it. When one kind of source keeps making viewers wait,
it lowers the maximum quality for that kind of source for a while, on top of
the calibrated ceiling, then lifts the limit again on its own. The active
limits are listed under **Automatic quality limits**, with the reason and the
expiry; **Reset** drops one immediately.

## History and statistics

- **Admin → Analytics**: plays today and this week, active users, the most
  watched titles of the last 30 days.
- **Admin → Activity**: the history of plays and of administrative actions,
  and the background work in progress.

How long each kind of history is kept is set in **Admin → Server Config →
Server Settings** (play sessions, audit log, server metrics, streaming
quality). A change applies at the next daily cleanup.

## Command line

`evey admin doctor` checks the whole server in one report; the other
diagnostic commands are listed in [Command line](../runbooks/cli.md).

<!-- sources
who's online: frontend/apps/web/src/features/admin/DashboardOnline.tsx (presence heartbeat, profile names, idle, SendCastReceiverCommand) ; i18n admin.online.*
labels: i18n admin.sessions.*, admin.systemHealth.*, admin.logs.*, admin.qoe.*, admin.analytics.*, admin.activity.*, admin.serverSettings.*RetentionDays, retentionSweepHint
automatic limits: i18n admin.qoe.overrides.* ; docs/adr/0018-cmcd-qoe-pipeline.md
-->
