# Subtitles

How a subtitle is shown on your device, and whether choosing it changes how
the video is delivered. For viewers; the only setting that matters is the
profile's **Subtitle Burn-in**, in **Settings → Playback**. The short
answer: text and Blu-ray subtitles never touch the video, DVD and broadcast
subtitles are burned in and force a re-encode.

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
labels: i18n auth.settings.playback.subtitleBurnIn, subtitleBurnAuto, subtitleBurnAlways, subtitleBurnNever, subtitleLanguage ; player.settings.subtitleSync, subtitleStyle, uploadSubtitle ; player.subtitleStyle.* ; player.streamInfo.reason.subtitle_burn_in
-->
