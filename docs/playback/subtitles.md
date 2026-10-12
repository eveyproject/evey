# Subtitles

**To turn subtitles on**, open the settings menu in the player and choose
**Subtitles**. To get them in your language every time, set **Settings →
Playback → Default Subtitle Language**.

Most subtitles appear straight away. Two cases are different:

- **The first time anyone picks a subtitle stored inside the file**, Evey has
  to take it out of the file. The player shows **Subtitles are being
  prepared…**, the video keeps playing, and the subtitles appear by
  themselves. On a very large film this can take a few minutes, once: after
  that, they show at once for everyone. See
  [The first time a subtitle is shown](#the-first-time-a-subtitle-is-shown).
- **DVD and TV-broadcast subtitles** are pictures, not text. The server
  draws them into the video itself, which makes it work much harder. The
  profile setting **Subtitle Burn-in** (**Settings → Playback**) can refuse
  this, but then those subtitles cannot be shown. See
  [Subtitle Burn-in](#subtitle-burn-in).

The rest of this page explains each subtitle format and what it changes for
the server.

## What each format does to the video

| Format | Examples | How it is shown | Effect on the video |
| --- | --- | --- | --- |
| Text | SRT, ASS/SSA, WebVTT, subtitles inside the file | Sent alongside the video and drawn by the player, with your **Subtitle Style** | None |
| PGS (Blu-ray) | `hdmv_pgs_subtitle` | Decoded into images once by the server, cached, and drawn by the player as they are | None |
| Other picture formats | VobSub (DVD), DVB, XSUB | Burned into the picture by the server | The video is re-encoded |

Every player (web, Android, Android TV) draws text and PGS subtitles itself,
so choosing one never changes how the video is delivered. Only the last row
does: burning subtitles into the picture requires decoding and re-encoding the
video, so a title that played directly becomes a transcode. Stream info then
lists **Subtitles burned into the picture**.

A picture subtitle is burned in only when you actually select it. A file that
merely carries one, or marks it as default, keeps its Direct Play.

## Subtitle Burn-in

The profile setting **Subtitle Burn-in** (**Settings → Playback**) governs the
formats of the last row:

| Setting | A selected VobSub, DVB or XSUB track | Text and PGS |
| --- | --- | --- |
| **Auto** | Burned in, the video is re-encoded. | Drawn by the player. |
| **Always burn in** | Same as Auto. | Drawn by the player: text is never burned in, whatever the setting. |
| **Never burn in** | Cannot be shown: the video is never re-encoded for a subtitle. | Drawn by the player. |

## The first time a subtitle is shown

A subtitle carried inside the file (text or PGS) is extracted by the server the
first time someone selects it, then kept: every later viewing, by anyone, gets
it at once. When the file's own index covers the track, the server reads only
the subtitle's blocks and it takes seconds. Without that index it reads the
whole file: several minutes for a large film (about five for an 85 GB UHD
remux on a local disk, longer while the same server is streaming).

Meanwhile the player shows **Subtitles are being prepared…** where the
subtitles appear, checks back on its own and shows them as soon as they are
ready; the video keeps playing. Choosing another track does not stop the work:
the server finishes it for the next time.

A track the server cannot extract stays empty. The failure is remembered, so
selecting it again does not start another full read; an update of MKVGo, the
library that extracts it, tries again. See
[MKVGo in Evey](mkvgo.md#what-mkvgo-does) for what is read and when.

## Choosing and adjusting

- The players select the subtitle of the profile's **Default Subtitle
  Language**, then the track the file marks as default (a forced track, for
  example). A track chosen by hand is never overridden, and a new title
  starts the choice from scratch.
- **Subtitle sync** in the player shifts the subtitles for the current title
  (positive values show them later). It applies at once.
- **Subtitle Style** sets the size, text colour, background, font, shadow and
  position of text subtitles. PGS images are drawn as they are.
- **Upload subtitle…** adds a file for the title.
- With an OpenSubtitles key (see [Libraries and metadata](../administration/libraries.md#metadata-keys)),
  the server API can search for and download a subtitle the file does not carry
  (`GET /api/v1/media/{id}/subtitles/search`, then
  `POST /api/v1/media/{id}/subtitles/download`). The file is saved in a
  `.subtitles` folder next to the film. The apps do not offer this search.

<!-- sources
decision: internal/app/transcoding/decision.go - evaluateSubtitle, forceBurnVideoTranscode
client-drawn pgs, burn-in formats: frontend/packages/api/src/imageSubtitles.ts - isImageSubtitle, isClientDrawableSubtitle, isBurnInSubtitle ; internal/adapters/driving/http/subtitle_pgs.go
only when selected: frontend/packages/api/src/clientSubtitleTrack.ts - burnInSubtitleIndex ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts (profile languages, then the file's default; hand pick never overridden)
players: frontend/apps/mobile/src/components/player/SubtitleOverlay.tsx ; frontend/apps/tv/src/components/SubtitleOverlay.tsx ; frontend/apps/web/src/features/playback/PGSSubtitleOverlay.tsx
opensubtitles: internal/app/subtitle/service.go (.subtitles folder) ; internal/adapters/driving/http/subtitle_handler.go - registerSubtitleRoutes (no client calls search/download)
first extraction, preparing, failure memo: internal/adapters/driving/http/subtitle_preparing.go (202 + Retry-After past 20 s, detached extraction) ; subtitle_cache.go - getFailure, putFailure (keyed by the mkvgo version) ; frontend/packages/api/src/subtitleFetch.ts ; frontend/packages/client/src/hooks/useSubtitleTrack.ts ; i18n player.subtitlesPreparing
labels: i18n auth.settings.playback.subtitleBurnIn, subtitleBurnAuto, subtitleBurnAlways, subtitleBurnNever, subtitleLanguage ; player.settings.subtitleSync, subtitleStyle, uploadSubtitle ; player.subtitleStyle.* ; player.streamInfo.reason.subtitle_burn_in
viewer opening: i18n player.subtitles, player.subtitlesPreparing, auth.settings.playback.subtitleLanguage, subtitleBurnIn
-->
