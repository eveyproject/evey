# Changelog

All notable user-facing changes to Evey are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions
follow the project's release tags. This file starts with the `beta-0.0.1`
line - earlier history lives in git.

## [Unreleased]

### Added
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
- Casting reworked end to end: reliable device pairing on desktop Chrome and
  Android, resume and seek land on the exact position on both direct play and
  transcode, and cast failures are reported server-side.
- Seek in transcode playback is served natively from a complete on-demand
  manifest (no session restart) on web, mobile and TV.
- Theme contrast raised to WCAG AA on all five themes.
- Admin logs are request-scoped (request id on gRPC and HTTP log lines) and
  searchable from the admin panel.

### Fixed
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
