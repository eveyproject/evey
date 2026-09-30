# Background work and server load

Besides playback, the server runs work of its own: scanning libraries,
fetching artwork, generating the thumbnails of the seek bar, detecting intros
and credits, analysing files (keyframes, codec levels, media health). This
page explains how that work shares the machine with the people watching.

## Playback comes first

Background tasks draw from a share of the processor, **25 %** by default
(**Admin → Server Config → Processing power**). **50 %** speeds them up, for
when nobody uses the server; a heavy task (media analysis, thumbnails, intro
detection, classification) can be given a share of its own. A change applies
to the next item processed.

Whatever the share, a background task pauses while someone is watching, and
resumes afterwards. On a new library, thumbnails and intro markers therefore
arrive over hours rather than minutes if the server is in use.

## What runs in the background

| Work | What it gives | When it runs |
| --- | --- | --- |
| Library scan | New, changed and removed files | On file changes, on schedule, at startup if enabled. See [Libraries and metadata](../administration/libraries.md). |
| Artwork download | Posters and backdrops kept on the server, in WebP | During the scan; missing ones are retried hourly. Off: images are loaded from the internet by the browser, which some browsers block. |
| Seek bar thumbnails | The previews shown while dragging the seek bar (one image every 7 seconds) | When a title is added, and at startup for those without. Can be turned off in **Server Config**. |
| Intro and credits detection | The **Skip** buttons | Continuously, for titles community databases do not cover |
| Keyframe index and codec levels | Direct Play and copy without re-encoding, correct colours | Continuously, on files scanned before the information was collected |

## Memory

**Memory profile** (**Admin → Server Config**): **Low** suspends plugins that
are idle to save memory, and wakes them on demand. **Auto** picks Low on
machines with less than 3 GB of memory.

## Settings read at startup

A few settings are read once, when the server starts: saving them stores your
choice, a restart applies it. The panel says so next to them. They are the
**Background work** card (pausing and pacing of the keyframe and codec-level
passes), the **Intro Skip Providers** switch, and the **Advanced limits** of
transcoding.

<!-- sources
processing power, pause during playback: i18n admin.serverConfig.cpuBudget.* ; admin.activity.bgTaskHelp.*
artwork: i18n admin.serverConfig.artworkInfo, artworkRetryDesc
thumbnails: i18n admin.activity.bgTaskHelp.trickplay_generation
memory: i18n admin.serverConfig.memoryProfileDesc
read at startup: i18n admin.serverConfig.backgroundJobs.restartWarning, introProvidersRestart ; docs/playback/transcoding.md
-->
