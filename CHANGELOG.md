# Changelog

All notable user-facing changes to Evey are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions
follow the project's release tags. This file starts with the `beta-0.0.1`
line - earlier history lives in git.

## [Unreleased]

### Added
- Media health (admin): every MKV/WebM gets one verdict - repairable here,
  your call, or for information - with acknowledgement, "Repair all" acting
  on exactly the repairable set (damaged files only with an explicit resync
  consent), and a one-click rename of a mislabelled .mkv that is really an
  MP4. Runbook in `docs/runbooks/media-health.md`.
- Capacity load test: the report names an opaque storage mount (mergerfs,
  FUSE, network) instead of guessing at disks, and a calibration taken on a
  busy host is marked provisional and redone when the box is idle.
- Mobile: volume up to 200% through the app's own audio chain (VLC-style
  preamp), with smooth steps between the device's hardware levels.
- Single-container install: the image embeds Postgres (and Meilisearch) and
  generates their secrets on first boot; `.env` needs only `MEDIA_DIR`.
  Existing installs with their own Postgres (`EVEY_DB_HOST` set) are
  unchanged.
- TMDB metadata works out of the box: a built-in read token is seeded as the
  default; an admin's own key (panel or `EVEY_TMDB_API_KEY`) replaces it. The
  other provider keys appear in the panel from the first boot, empty.
- `scripts/evey-update.sh`: repeatable host-side update (pre-update dump,
  pull, health wait, commit check, rollback line), twin of
  `scripts/evey-rollback.sh`.
- Watch progress now keeps tracking while casting even after the browser
  closes: the server derives the position from the receiver's own fetches.
- Retention windows (audit log, server metrics, playback QoE, play-session
  history) and the transcode cache budget are editable from the admin server
  settings panel; the retention sweeps pick changes up without a restart.
- Play-session history retention (opt-in, keeps the table bounded).
- AC-3/E-AC-3 playback in browsers without native support, decoded client-side.
- Gapless multi-episode playback sessions (opt-in).
- Media requests: users can request missing titles, admins triage them.
- Built-in capacity load test (admin), synthetic viewers against the real stack.

### Changed
- Seek-index diagnosis moves to mkvgo 0.27: an index is judged against the
  picture's real end (sound outlasting picture is no longer a hole), holes are
  probed to say whether a rebuild, a re-encode or the source is the remedy,
  and an audio track ending before the picture is reported as such. The
  first scan after the upgrade re-diagnoses the whole library.
- Mobile and TV declare AC-3/E-AC-3/DTS/TrueHD to the server only when the
  device's audio output accepts them - a decoder alone no longer counts.
  Devices that rendered direct-played AC-3 choppily now get the audio
  transcoded; passthrough boxes are unaffected.
- Counts read correctly in the singular in all six languages ("1 result",
  "1 élément ajouté").
- API calls time out after 60 s instead of hanging on a server that never
  answers; the operations the server performs inside the request (plugin
  install, backup now, transcode-cache clear, library delete) are exempt.
- Casting reworked end to end: reliable device pairing on desktop Chrome and
  Android, resume and seek land on the exact position on both direct play and
  transcode, and cast failures are reported server-side.
- Seek in transcode playback is served natively from a complete on-demand
  manifest (no session restart) on web, mobile and TV.
- Theme contrast raised to WCAG AA on all five themes.
- Admin logs are request-scoped (request id on gRPC and HTTP log lines) and
  searchable from the admin panel.

### Fixed
- Playback: the stereo fold of a 5.1 track distorted on loud passages (the
  level makeup pushed the mix past full scale and the encoder hard-clipped it,
  audible on a tablet through a whole action climax). The fold now runs in
  float behind a look-ahead limiter: same level, no clipping, and a source
  already in stereo is left untouched.
- Mobile: the Music tab crashed with "Rendered more hooks than during the
  previous render"; the same class was removed from the TV home banner, the
  web library page and the mobile home rows, and the lint gate now refuses
  it (including behind an eslint-disable comment).
- Mobile cast: a token refresh in the middle of a cast no longer reloads the
  receiver from the start; the sender now reports a receiver that stops
  answering (`cast_sender_stall`).
- Media health: an acknowledgement posted while a scan runs is no longer
  lost when the scan completes.
- People the metadata provider has no biography for are no longer re-fetched
  every day (negative cache, 30 days).
- Mobile home feeds panel: one save per changed row instead of one per feed,
  and no duplicate saves under StrictMode.
- Artwork is requested at display size (hero backdrops, detail posters, music
  covers) and list thumbnails keep a memory cache; a track's tag-embedded
  cover is now resized server-side too (`?w=`) instead of shipped in full.
- Player regressions: audio after deep resume, back navigation restarting the
  player, episode-change error screens on mobile, focus traps on TV.
- Library watcher: real-time monitoring survives bad files, reports its own
  liveness honestly, and stays quiet when monitoring is disabled by choice.
- Parental controls hardened across every access path (subtitles, artwork,
  search, ZIP download, metadata RPCs) and profile-management escapes closed.

### Security
- Stream tokens hardened (scoped, sliding TTL, revocation on user disable).
- Plugin sandbox: opaque-origin iframes, scoped tokens, per-plugin egress
  rules; installer validation tightened.
- Rate limiting extended to subtitle extraction and trickplay endpoints.

## [beta-0.0.1]

Initial public beta: library scanning and metadata enrichment, direct play /
remux / transcode playback with adaptive quality, web + mobile + TV clients,
music and photo libraries, subtitles (embedded and external), trickplay
previews, intro/credits detection, multi-profile with parental controls,
plugins, SyncPlay, Chromecast and DLNA, downloads, backups, i18n (6 locales).
