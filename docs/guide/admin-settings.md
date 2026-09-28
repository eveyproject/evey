# Administration

The admin panel (avatar menu, then **Admin**) is where an administrator runs
the server day to day. Nearly everything is stored in the database: it
survives a container update, and most of it applies at once. The panel says
when a setting only takes effect after a restart (`docker compose restart`).

This page follows the panel's menu and stops on the settings that matter. For
what must be decided before the server boots (public URL, trusted proxy,
file ownership), see [Configuration](../configuration.md).

## Dashboard and system

### Dashboard and Now Playing

The **Dashboard** shows the version, the machine and what is happening.
**Now Playing** lists every stream in progress: who, what, on which device,
and how - **Direct Play** (the file as is), **Remux** (repackaged, picture
untouched) or **Transcode** (converted) - track by track, with the engine and
the graphics card used. You can stop a session or send a message to the
viewer.

### Remote access

Who may connect from outside your home network.

| Setting | What it does |
| --- | --- |
| **Enable Remote Access** | Off: anyone whose address is not on your local network gets an error on every page. On: they can sign in like anyone else. |
| **External URL** | The public address, when it differs from the one the server was started with. |
| **LAN Networks** | The address ranges treated as "home". Private ranges are listed by default; add a VPN range (Tailscale, WireGuard) here if its viewers should count as home. |
| **Max Remote Bitrate** | A bitrate ceiling for viewers outside the home, in kbit/s (0 = none). A private address is never capped. |

When you leave it untouched, the server decides at start-up: remote access is
on if you gave it a public address, off otherwise. If the server is reachable
from the internet while remote access is off, the page warns you in red.
Behind a reverse proxy, this only works when the proxy passes on the real
client address - see [Reverse proxy](../reverse-proxy.md#real-client-ips-and-the-remote-access-gate).

Remote access is a choice about who may reach the server, not a protection:
sign-in is what protects the library.

### Server Config

The page groups the server's own settings in cards:

| Card | The settings worth knowing |
| --- | --- |
| **Language & Region** | Default interface language, the country whose age ratings apply, and the language of titles and summaries. |
| **Home feeds** | Which rows the home page shows, in which order, how many items each. Plugins can add rows. |
| **Email / SMTP** | Outgoing mail for invitations and password resets. See [Notifications](../notifications.md). |
| **Trickplay Thumbnails** | Generates the timeline preview images. On by default. |
| **Processing power** | How much of the processor background work may take together: **25 %** (gentle, default) or **50 %** (boost, for when nobody is watching). A heavy task (new media analysis, thumbnails, intro detection, complexity classification) can get its own share. Applies to the next item. Background work pauses while someone watches. |
| **Memory profile** | **Low** suspends idle plugins to save memory; **Auto** picks it on machines under 3 GB. |
| **Background work** | Pauses and paces the background passes (keyframe index, codec levels). Read at start-up: restart after a change. |
| **Artwork Downloads** | Keeps posters and backdrops on the server, converted to WebP, instead of loading them from the internet each time. |
| **Parser tokens** | Extra release tags and edition names the file-name parser should strip, when your files carry tags it does not know. |
| **API Keys** | TMDB (a shared key is built in), OMDb, OpenSubtitles, TheIntroDB. |
| **Intro Skip Providers** | Uses community databases (TheIntroDB, then IntroDB) for intro, recap and credits timings before running the server's own detection. Restart after switching it. Optionally sends the server's confident detections back to TheIntroDB. |
| **Server Settings** | Server name, sign-in and global rate limits, log level, file watcher and scan interval, **Auto-skip intros** (off by default: when on, every player skips detected intros on its own), **Skip preview segments**, how long history and statistics are kept, the **transcode cache budget** (10 GB by default), and the AV1 software ceiling. |
| **Updates** | Checks every 6 hours whether a newer image is published and tells you. Evey never updates itself. |

Below the cards, **Config entries** lists every raw key-value setting, for the
rare case a documented key has no card of its own.

### Transcoding

How the server converts video.

- **Encode calibration** shows what the server measured about itself: the
  highest resolution it can convert in real time, in software and with the
  graphics card, and which codecs the card handles. **Hardware
  acceleration** chooses the card (**Auto** takes the best one detected, **Off**
  forces the processor). **Recalibrate** after adding a graphics card or moving
  the container to another machine. Also here: **AV1 output** (hardware only),
  Intel's **low-power encoder**, and a **video bitrate ceiling**, with a
  separate one for viewers outside the home.
- **Live playback capacity** is the number of videos that may be converted at
  the same time. It is measured; you can only lower it. A stream that only
  converts the sound does not count, and a viewer changing quality keeps a
  single slot.
- **Background transcode jobs** limits queued conversion jobs, separately
  from live playback.
- **Advanced limits** caps sessions and resolutions on weak hardware. Read at
  start-up.
- **Load test** simulates real viewers to measure what the server sustains.
  Run it only when nobody is watching.

Setting up a graphics card in Docker: [Transcoding and hardware](../transcoding.md).

### Transcode cache and System health

**Transcode Cache** shows what conversions left on disk and clears it. After
an update that changes how videos are converted, clearing it makes sure nobody
is served an old conversion. **System Health** shows processor, memory,
bandwidth, storage and the conversion capacity in use.

## Media

- **Libraries**: add, edit, scan, schedule; **Match orphans only**,
  **Re-match all** and **Repair sagas** for metadata. See
  [First steps](first-steps.md#add-your-libraries).
- **Artworks Cache**: the posters and backdrops kept on the server.
- **Media Health**: a read-only scan of every MKV and WebM file gives one
  of two verdicts. **Repairable** (missing seek index, damaged stretch, an
  `.mkv` that is really an MP4) is fixed in one run by **Repair all**, and
  every repaired file keeps its original as a backup. **For information**
  (incomplete download, audio offset...) lists what cannot be fixed there.
  Repairing writes to your media folders, so they must be mounted read-write.
  More in [Media health](../runbooks/media-health.md).

On a title, an administrator can also **edit metadata**, change the poster,
and adjust the **media segments** (intro, recap, credits) by hand or re-run
their detection.

## Users and access

- **Users**: create, edit, disable or delete accounts; restrict each one to
  some libraries; cap its streaming bitrate, its number of profiles and of
  devices per profile; view its history; **Connect as** the user to see what
  they see (a banner reminds you, **Exit** comes back).
- **Invite Links**: a link with an optional expiry, number of uses,
  libraries, profile limit, and administrator role.
- **Media Requests**: the queue of titles users asked for (approve, decline
  or postpone, with a note), and who may request: everyone by default, with
  per-user exceptions.

## Integrations

- **Plugins**: install from a ZIP file. Before installing,
  the panel shows the permissions the plugin declares and what its static
  analysis found. Per plugin: enable, update, resource limits (memory,
  processes, disk, priority), logs. See [Plugins](../plugins.md).
- **Webhooks**: call a URL of yours when something happens (a title added, a
  playback started...), for all events or a chosen few, signed with a secret
  if you give one. **Test** sends a ping.
- **API Keys**: keys for your own scripts and integrations, each limited to
  the scopes you tick (read media, control playback, manage users...). A key
  is shown once, at creation. **API documentation** lists what each scope
  unlocks and lets you try it.

## Monitoring and maintenance

- **Analytics**: plays today and this week, active users, the top titles of
  the last 30 days.
- **Activity**: the history of plays and of administrative actions, and the
  background work in progress.
- **Logs & Debug**: search and follow the server's logs live, filtered by
  area (playback, conversion, scan...). The first stop when a user reports a
  problem.
- **Streaming Quality**: how playback went for real viewers over the last
  day or week - how often they waited for the video, the titles that stall
  most, and the kinds of device that struggle.
- **Database Backup**: create a backup, download it, restore one, or restore
  from a file. The schedule and retention are covered in
  [Backups](../backups.md).

When the panel itself is what stopped working, the same maintenance is
available from the command line: [Command line](../runbooks/cli.md).
