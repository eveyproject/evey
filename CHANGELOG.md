# Changelog

All notable user-facing changes to Evey are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions
follow the project's release tags. This file starts with the `beta-0.0.1`
line - earlier history lives in git.

## [Unreleased]

## [0.1.5] - 2026-11-10

Hotfix on top of 0.1.4. Server image `eveyproject/evey:0.1.5` (also `0.1`
and `latest`).

### Fixed
- Browser playback of an AC-3 or E-AC-3 track through the in-browser decoder
  spun for ever since 0.1.4: the video-only master stamped an audio re-encode
  pin on its variant URLs, the variant request then resolved another plan
  than the one the master sealed and every variant answered 410, so the
  player reloaded the master in a loop. A playback on the in-browser decoder
  no longer carries that pin. A player already open when the server is
  upgraded takes one last 410 and starts again on clean URLs.
- Android: the barcode scanner screen no longer locks the phone to portrait,
  which Google Play flagged for large screens. Ships with the next app build.

## [0.1.4] - 2026-11-08

Server image `eveyproject/evey:0.1.4` (also `0.1` and `latest`). The
television and phone fixes below ship with the next app builds.

### Upgrade notes
- Migrations 000188 to 000193, all additive. The library is probed again
  once in the background after the upgrade so every stream gets the track
  number the file gives it (renditions pair on it from now on).
- The cast receiver artifact has to be republished with this server.
- Sources with 5.1 audio re-encoded on a multi-rung ladder get a new cache
  key: a session on one of them reloads once.

### Added
- The sign-in screen renews a session the server may still honour instead
  of asking for a password: a page load on the sign-in page with no usable
  access token asks the server first, on the web, the phone and the
  television; a sign-in from another tab sends a tab sitting on the sign-in
  page to the home.
- Mobile: the profile menu's Quick Connect entry opens the screen to type or
  scan the code another device shows, not the one that shows this phone's.
- Television: UP in the player reaches the "Join a room" button of the top
  bar, DOWN the bottom row, BACK returns to the picture; the subtitle style
  picker lays each setting out as one row.
- The television home picks up titles added while it is on screen (rows
  re-read when the notification count rises, and on return when older than a
  minute).
- Stream metadata carries whether an audio or subtitle track is a commentary,
  hearing-impaired, visual-impaired or original-language track; home cards
  carry the community rating and a blurhash. Admin settings `hls_ondemand`
  and `remux_copy` apply live to new playbacks.
- Serving observability: ffmpeg log lines are correlated with their session,
  and a first-segment latency metric ships with an example alert.

### Changed
- Playback: a title watched again resumes below its completion point; the
  media page reads the last position once the player's final report has
  landed, so BACK offers Resume instead of Play.
- People are identified by their TMDB id; homonyms are no longer merged.
  Multi-episode files take the first number, an unnumbered file under a
  season folder is an episode of that season, extras and samples are not
  episodes.
- Library roots follow one rule on create and update; system trees are
  refused. Library roots and sidecar paths are admin-only in the API;
  `/remux.mp4` is bounded by rate and production slots.
- The packager (mkvgo 0.66.1) reads an HLS window ahead as a whole on local
  disks, MP4 segments and Matroska subtitle scans alike, so a cold file comes
  off the disk in large sequential reads instead of thousands of small ones.

### Fixed
- Web: seek previews survive a hover that leaves the bar before the manifest
  arrives; the title no longer stays without previews until a reload.
- Session resilience: only a 401 from the refresh endpoint ends a session; a
  403, a 5xx or a network failure keeps it; a slow database no longer refuses
  refresh tokens; the television profile switch rotates the refresh token;
  parental unlocks are cleared on profile switch.
- Casting: `abr=floor` and `maxKbps` survive the plan as served (HLS and
  DASH), so a receiver's rescue reload gets the single low rung; the next
  episode resumes where the cast left it.
- Television: the subtitle overlay is handed the appearance the viewer chose;
  saving a theme no longer writes the old theme back from a stale profile
  cache; the profile's subtitle burn-in choice (auto / always / never) is
  obeyed by the native players; the byte cap of the native load control gives
  way to a starved player; a screen opened from outside (a link, a Cast load,
  a device command) gets the home under it so BACK has somewhere to go.
- Scan safety: a replaced file is re-probed; a file upgraded under a new name
  keeps its item (progress, favourites, playlists); orphan cleanup waits for
  files still being written; rows without streams, symlinked roots and
  transactional prune are handled; backups land atomically.
- Episode chains: gapless playback applies the same audio resync as single
  episodes, or refuses the chain.
- Transcoding: twelve divergences between what the manifest declared and what
  was served are closed; renditions are paired with the file's own track
  number instead of a name that mkvgo now numbers when duplicated.
- Media health: cues pointing past the end of a complete file (`cues-stale`)
  send the file to a reindex.
- External audit items: season-folder episodes, recursive mark-watched,
  stale sessions, provider text sanitising, portrait video tiers, dead
  artwork back-off, TMDB image ranking, login history, strict artist match.
- Calibration on a small host: the HDR sample encode declares its memory and
  is refused cleanly when the host lacks the room instead of crashing, lands
  atomically and is resolved once per bench.
- Server: a token refresh presenting no token is refused before any lookup.

### Security
- Dependencies: brotli 1.2.7 and klauspost/compress 1.20.2 (govulncheck
  findings); mkvgo 0.63.0 to 0.66.1 (field order re-probe, 32-bit bounds,
  malformed-input refusals).

## [0.1.3] - 2026-11-02

Hotfix on top of 0.1.2.

### Fixed
- Casting: a receiver that cannot decode the source (HEVC, HDR) gets a
  transcode ladder from the first master; its measured line (`maxKbps`) and
  its starvation rescue (`abr=floor`) were only applied to copy plans, so a
  television whose decoder dropped frames on the top rung reloaded on the
  floor and got the same rungs again until the receiver gave up (image gone,
  sound on). A transcoded plan is now narrowed too: the floor is its lowest
  rung, the line the highest rung it carries.

## [0.1.2] - 2026-11-02

### Changed
- Home (television, mobile): one "Resume" row, kept only after three minutes
  played, one card per series (the unfinished episode or the next one),
  ordered by latest activity; the next-up row is retired. Recommended and
  popular cards print the release year on every client.
- Library (television, mobile): the grid shows whole rows only, poster and
  title; on the television the filters scroll away with the first row, a
  selected chip carries a check mark distinct from the focus, and a summary of
  the active filters stays visible. Media action labels wrap and truncate
  inside their button. Home cards, library grid cards and side rail entries
  are memoised; covers and faces are requested at the size they are shown.

### Fixed
- Television: a token refresh answered during sleep no longer signs the viewer
  out on wake; the player buffer is sized to an eighth of the heap, the dead
  64 MiB ceiling is gone.
- Library prune guard: the confirmation is bound to a snapshot of the missing
  set (deletes only that set, lapses after ten minutes, rejected if the set
  changed); the same mass-disappearance guard covers music and photo
  libraries, and rows under none of the library's paths (a changed library
  path) instead of deleting them unguarded.
- Admin: the edit-user dialog rereads library access at every opening and
  writes it only if the admin changed it; a failed load shows a retry instead
  of a form whose Save would lift the restriction.

### Security
- Frontend dependencies patched through pnpm overrides (node-forge, seroval,
  decode-uri-component, glob, minimatch), with the regressions of the first
  pass corrected: `query-string` 9 loaded through its named-export entry on
  the phone and the television.

## [0.1.1] - 2026-11-01

### Upgrade notes
- Migration 000187 turns real-time library monitoring on for every existing
  library. Going back means restoring the pre-migration dump.
- Plugin fetch broker: an `external_fetch` entry without a port now allows 80
  and 443 only, and `host:port` that port only, on every redirect hop (a
  documented contract change).

### Added
- Metadata plugins lay their texts over TMDB, field by field, per locale: the
  administrator chooses which fields a plugin may replace, reversibly.
- Real-time library monitoring is the default and can be switched on an
  existing library; a title already out of the catalogue shows a dedicated
  "gone" screen on the four players, its library is rescanned in the
  background, and the player moves on to the next episode.
- Native players (television, phone): a playback error reaches the server
  taken apart (renderer, format verdict, decoder, mime, codecs, size, frame
  rate, bitrate, causes); every stream verdict is logged with the
  capabilities the client declared; a source that fails before the player
  screen subscribes is no longer lost.
- Web: the render watch carries Firefox's painted frame rate beside the frame
  callback count, so a cadence report tells a missed frame from a coalesced
  callback.
- Webhooks: each delivery adds `X-Evey-Timestamp` and `X-Evey-Signature-V2`
  (an HMAC over timestamp, delivery id and body) for replay protection;
  `X-Evey-Signature` is unchanged.
- Library prune guard: deletions under a root are held when too many of its
  items vanish in one pass (5 % and at least 20 by default, configurable), one
  notification per suspension, automatic resume when the ratio falls back,
  otherwise an admin confirms from a banner.

### Changed
- Television app fluid on low-memory devices: lighter lists, player
  stability; the launcher icon fits the adaptive mask.
- A source heavier than the server's whole encode capacity is refused for
  good (`source_too_heavy`) and every player drops its retry for another
  device.
- The library watcher runs its initial walk in the background with a timeout
  per root; a root that fails drops its watches and marks the library
  unhealthy; an inotify overflow triggers a rescan; mergerfs, shfs, virtiofs
  and grpcfuse count as network filesystems.
- Dependabot opens security updates only, grouped per ecosystem.

### Fixed
- Lipsync on the television for 23.976 and 29.97 sources: the audio rendition
  playlist declares the grid its segments are cut on, so sequence-mode
  players no longer run the audio early segment after segment; seek and
  teardown corrected alongside.
- An interlaced H.264 track muxed one field per block reports its field rate;
  it is now counted in frames for its level, so a 1080i stream no longer
  re-encodes for a decoder that plays it.
- Native player: a file's audio track is applied by language and title, not
  by list position (the original-language to dubbed swap).
- Player: the subtitle overlay and the skip button follow the playhead through
  a selector and re-render only when the active cue or segment changes.
- Playlist music tracks play in the music player on every client instead of
  landing on the video page; the device socket backs off instead of retrying
  every 30 seconds forever.
- The logs page stops its live tail on a refusal instead of hammering a
  refused stream; the admin menu leaves out Logs where the demo refuses them.
- HLS accounting: a resource served whole counts when its bytes leave, not
  when it is opened; a HEAD answers from its size.
- Parental control: the barrier comes first on every route; wrong PINs feed
  one lockout shared by HTTP and gRPC, growing from a minute to a day;
  loosening a PIN-guarded profile needs its current PIN; library restriction
  is an explicit flag, so a deleted library or a revoked last grant denies
  instead of granting; the Quick Connect secret stays server-side, an
  authorized code is claimed once, an expired request can no longer be
  authorized.
- Plugins: the live directory is moved aside during an upgrade and restored
  on any failed step; the boot sweep restores an interrupted swap; the fetch
  broker caps in-flight fetches and body bytes per plugin; plugin logs are
  capped at a daily byte budget; the seccomp filter refuses resource,
  scheduling and page placement syscalls aimed at another process.
- Backup restore streams the validated dump into psql, stops on a rejected
  line before it can commit, and refuses to overlap a backup or another
  restore.
- Search: a Meilisearch write waits for its task and fails when the task
  fails; a refused batch is bisected down to the refused documents; a
  document is dead-lettered after five refusals; deleting a library purges its
  documents at once; the outbox drain no longer deletes a row re-enqueued
  after it was listed.
- Transcoding: a seek the host has no encode slot for is refused at once; the
  audio orchestrator stops an evicted session outside its lock; a session
  whose ffmpeg exits on its own releases itself; the rebase pump replaces a
  published init whose moov is incomplete; segments are served under a
  sliding write deadline so a client that stops reading releases the handler.
- Webhooks: a configured header can no longer replace the signature or
  delivery headers; event deliveries get a budget that covers every retry.
- DLNA: renderer discovery trusts responders on private or link-local
  addresses only, and play refuses a media URL that is not absolute http(s).
- Smart playlist rules compile to SQL in the database adapter, with the genre
  bound safely; playlist items pointing at a removed title are purged.
- tv-web: "remember me" keeps only the username and erases a password left in
  local storage.

### Security
- go-jose 4.1.6 closes a denial of service when parsing JWKs with RSA keys.
- Frontend dependencies moved past their advisories (orval 8, vitest 4,
  vite 7, happy-dom 21, turbo 2.11), the example plugins and the end-to-end
  suite likewise.
- mkvgo 0.62.0 on the server and the offline audio decoder.

## [0.1.0] - 2026-10-24

First public release: the server image `eveyproject/evey:0.1.0` (also
`0.1` and `latest`), the Android phone app 0.1.0 and the Android TV app
0.1.0 on Google Play. Everything below is what changed since `beta-0.0.1`.

### Upgrade notes
- An install whose `EVEY_DB_PASSWORD` is still the old default `evey` now
  refuses to start; `changeme` only warns. Set a real database password
  before upgrading.

### Added
- Mobile: a banner offers the Play Store page when the server was released
  with a newer phone build (checked on start and every 6 hours against
  `/healthz`, which now names the mobile build next to the TV's). The store's
  own auto-update waits for an idle device on Wi-Fi and left a phone a month
  behind on a build with a known playback bug.
- Subtitles on MKV/MP4 playback come from the packager as segmented WebVTT
  renditions in the HLS master (copy, hybrid, wasm-audio and transcode
  masters alike), served window by window next to the video segments - the
  web player switches to them the moment a track is picked, instead of the
  whole-file extraction that took minutes on a large file. Other containers
  keep the extraction route.
- Admin: the Now Playing card and the sessions page name what actually serves
  each track - mkvgo copy, ffmpeg with its backend (software, Quick Sync,
  VAAPI, NVENC), or audio decoded by the player (ac3go) - recorded by the
  server when it renders the master playlist and kept in step with the wasm
  beacons, instead of a single "Transcode" badge.
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
  generates their secrets on first boot; the compose file needs only the
  media path, and no `.env`.
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
- Picking a subtitle track that still has to be produced (a bitmap track, or a
  text track of a large MKV without a block index) no longer leaves the
  request hanging for minutes: the server answers "being prepared" with a
  retry delay computed from the extraction's real progress, and the web, TV
  and mobile players show "Subtitles are being prepared" and poll until the
  track is ready. A track this build of the packager cannot decode is
  memorised and refused at once instead of re-read on every click.
- Subtitle extraction reads the track's blocks where the file's own Cues
  point (text and bitmap tracks, on demand and in the prewarmer) whenever the
  Cues are verified complete for the track, so a large remux answers in
  seconds instead of a full read; mkvgo 0.42.0 also decodes bitmap tracks
  whose blocks were zlib-compressed without saying so (five such films in the
  dev library used to be refused).
- Admin: a notification when the subtitle cache stops writing because the
  disk is below the free-space floor shared with the transcode cache; the
  admin cache wipe now also empties the subtitle cache.
- Operators: `evey_mkvgo_walks_in_flight` and `evey_mkvgo_plan_stats{stat}`
  on /metrics expose the packager's windows being read, queued walks,
  streamed segments and fallbacks, and the subtitle tracks served from the
  Cues.

### Changed
- Webhooks: a `media.item.updated` event whose `Reason` is `file_replaced`
  now carries the file's duration before and after the replacement, in
  milliseconds, as `PreviousDurationMs` and `DurationMs` (0 when unknown: a
  file never probed, or a new file the probe could not read). The fields are
  added to the payload; none is renamed or removed, so an existing receiver
  keeps working. Other reasons send both at 0.
- Packager upgraded to mkvgo 0.42.0: media segments of an MKV are written
  from the source as they go out instead of being assembled whole in memory,
  so the server's memory no longer grows with slow viewers (a 4-film run
  settles around 200 MiB); the windows read at once are capped at one per
  CPU within a quarter of the heap limit, the rest queue. Segment timing no
  longer overlaps a frame at open-GOP boundaries and laced AAC/AC-3 without a
  DefaultDuration keeps its real frame size to the end of a film, so the
  remux cache is re-produced (format v4) and players fetch fresh segments
  (the packager version is part of every segment URL).
- Packager upgraded to mkvgo 0.43.1: a streamed segment's large spans go to
  the connection through the kernel (`sendfile`) and its small ones in packed
  writes, and the remux's first pass writes each track through a buffer; less
  server CPU per segment served, same bytes. A segment write the connection
  takes only in part now cuts the connection instead of ending a short body
  as if complete, and a remux that fails on a full disk no longer leaves temp
  files behind.
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
- Server: a phone or tablet app older than 0.0.1-beta.76 no longer gets a
  DTS or TrueHD track in direct play - those builds declared the codecs on a
  signal Android honours on a TV only, so a DTS-only film played silent or
  stopped on a decoder error; the server now transcodes the track for them
  until the Play Store updates the app.
- Players (web, mobile, TV): resuming after a pause of a few seconds or more
  showed a frozen picture with the sound running for about three seconds
  before the video jumped ahead (the decoder had been parked and the clock
  restarted before any frame was decoded). The players now seek in place on
  such a resume so picture and sound leave together.
- Subtitles: an SRT track ripped from ASS showed its override tags in the
  text ("{\an8}Une heure."). They are stripped, and the alignment they carry
  becomes a WebVTT placement - a sign the author put at the top stays at the
  top on the web player, as it does in VLC.
- Intel Quick Sync transcodes cut their segments on the encoder's own 10 s GOP
  instead of the 3 s grid the manifest promises (the forced-IDR flag sat
  behind a probe that never ran for Intel), so a Chromecast stalled about
  20 s in and every seek landed off-grid. The flag is now always set. Clear
  the transcode cache after deploying: segments already produced keep the
  old cut.
- Sign-out ends the session of the device that signs out, not every device of
  the account (a phone signing out logged the TV and the browser out too). The
  clients also now send the sign-out with their credentials: it used to leave
  after the local session was cleared, so the server never revoked anything.
- Mobile/TV: changing the app language logged the user out about fifteen
  minutes later - the JWT refresh that followed rotated the refresh token
  server-side but never stored the new one on the device, so the next silent
  refresh replayed a dead token. Mobile settings also no longer calls an
  admin-only RPC for every user (a server warning per non-admin visit).
- Accounts: a username, email or password typed with a surrounding space (a
  phone keyboard appends one after autocomplete) was stored as-is while login
  trimmed it, so the new account could never sign in. Registration, setup,
  admin creation, rename and every password change now trim exactly like login.
- Playback: the stereo fold of a 5.1 track distorted on loud passages (the
  level makeup pushed the mix past full scale and the encoder hard-clipped it,
  audible on a tablet through a whole action climax). The fold now runs in
  float behind a look-ahead limiter, with a +3 dB makeup instead of +6 dB so
  the limiter barely works on real mixes; a source already in stereo is left
  untouched.
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
- API keys: an admin's key uses the admin role only within its scopes. A
  `users:write` or `playback:write` key no longer changes another account's
  password or role, manages its profiles or stops its sessions (another
  account's profiles and sessions need `admin:write`; passwords, roles and
  invites stay sign-in only), and reading another account's play history,
  logins or profiles needs `admin:read`. Older keys created without any scope
  no longer reach the admin statistics, library schedules or plugin tokens:
  give them `admin:read` or `admin:write`.
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
