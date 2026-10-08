---
hide:
  - navigation
  - toc
---

# Evey

Evey is a server you run at home for your films, series, music and photos.
You add your media and the people who will watch. For each playback, the
file, the player and the server negotiate how the picture is delivered, with
nothing to choose before pressing play.

[Install Evey](installation.md){ .md-button .md-button--primary }
[Try the demo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Beta, no numbered release yet</span>
<span>Free software, AGPL-3.0</span>
<span>Demo: signs you in by itself (<code>demo</code> / <code>demo</code>)</span>
</p>

<p class="evey-platforms">
<strong>What you need:</strong> an x86-64 computer or NAS with Docker, a
folder with your media, and a free port. One container, one compose file,
one command.<br>
<strong>Watch</strong> in any web browser, on a Chromecast, on Android phones
and tablets, and on Android TV. The Android and Android TV apps are in testing
and not on Google Play yet.
</p>

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![The home page](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Home: the titles to resume, then your libraries row by row.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![A film page](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>A film page: artwork, age rating, ratings, synopsis and credits.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![The web player](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>The web player. In the bar: Watch Together, Cast, and the (i) that opens Stream info.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![The admin dashboard while two films play](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>The dashboard while two people watch: the picture, the device, Direct Play or Remux.</figcaption>
</figure>

</div>

## For everyone at home

- **One profile per person.** Each profile keeps its own history, resume
  points, favourites and languages; a household usually shares one account.
- **Kids profiles.** A kids profile opens without a code; leaving it for an
  adult profile asks for a four-digit PIN. A profile can carry a maximum age,
  enforced by the server: a title above it is never sent, not even as a
  download.
- **Pick up where you left off**, on any device: resume points live on the
  server, not on the phone.
- **On the go.** The Android app downloads a title to watch offline, for 30
  days.
- **On the TV.** Android TV signs in with **Quick Connect**: a code shown on
  the screen, typed or scanned from a phone or a computer already signed in.
  No password typed with a remote.
- **Together.** Several people watch the same title at the same moment, each
  on their own device.

Music and photo libraries get the same treatment as films and series, on the
web, on Android and on the TV.
[Apps and devices](getting-started/apps.md) ·
[Users, profiles and parental control](administration/users.md)

## How a file reaches the screen

Evey never asks you to pick a quality. Each time a title starts, the server
chooses from three things: what the file contains, what this player can
decode, and what this server can encode right now. It prefers, in this order:

- **Direct Play**: the file as it is.
- **Remux**: the same picture and sound in another file format, nothing
  re-encoded. **MKVGo** does it inside the server where it can, without
  starting an extra program for each playback; FFmpeg copies the tracks
  otherwise.
- **Transcode**: only the tracks the device cannot play are re-encoded by
  FFmpeg, often just the audio.

The same film can go untouched to a TV and be re-encoded for an older phone.
In the player, **Stream info** shows the plan actually served and why the
file was not sent as it is. **Admin → Dashboard** shows the same for every
viewer.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Stream info in the web player](images/player-stream-info.webp){ loading=lazy }
  <figcaption>The first playback of the dashboard above, seen from the player. Remux: the file format changes, picture and sound are copied, and the last line says why the file could not be sent as it is.</figcaption>
</figure>

[How playback is decided](playback/how-playback-is-decided.md) ·
[MKVGo in Evey](playback/mkvgo.md)

## One container, and the server measures itself

One compose file is the whole installation: no `.env` to write, no memory or
CPU setting.

- **Database and search engine run inside the container**, reachable only
  from it, with passwords generated on the first start.
- **The server never runs as root.** It runs as the owner of your media
  folder when it can detect one, so that repairs can write.
- **Transcoding capacity is measured, not guessed.** At a quiet moment the
  server times real encodes on the processor and on each graphics card you
  gave the container; a card counts only for what succeeded. When room runs
  short, a new playback gets a lower quality, or moves from a busy card to
  the processor. A start that cannot be served is refused with a message that
  names the reason: the server is full, it is still measuring itself, or the
  file is not ready.
- **Remote access opens when you give the server a public address**, and
  stays closed otherwise.

Bringing your own database or search engine stays possible:
[Advanced deployments](operations/advanced-deployments.md).
[Transcoding and server capacity](playback/transcoding.md)

## What Evey does with your files

**Your files.** Removing a title or a library removes it from Evey, not from
the disk. Evey writes into your media folders only for a repair you asked
for, for a subtitle fetched through the API, and for a small probe file that
checks the folder watcher still works; mount a library read-only and none of
that happens. **Media Health** reads your MKV and WebM files and lists what
it finds; a repair rewrites a file only on request, in a copy checked in full
before it replaces the file, keeps the original by default, and every change
is journaled. What cannot be repaired is reported and left as it is.
[Media health](runbooks/media-health.md)

**Your data.** The project runs no service that receives it: no account with
us, no analytics, no crash reporting. The apps talk to your server and to
nothing else we operate. What leaves your network: the server asks outside
services for artwork and descriptions (The Movie Database, and cover art
sources for music), and once in a while asks the image registry whether a
newer build exists, which you can switch off. The database is dumped before
every update that changes it.
[Privacy policy](privacy.md) · [Backups and updates](backups.md)

## Plugins are contained, not trusted

A plugin is analysed before install; the administrator sees its findings and
the permissions it asks for, and a new permission needs an explicit
acknowledgement. It runs as its own process under a separate user, with no
direct network access, and is stopped when it exceeds its memory, process or
disk limit. How much the filesystem is confined depends on the host, and the
Security Dashboard shows the sandbox mode actually in use. Plugins add
metadata sources, notification channels, home sections and player overlays;
themes restyle the interface. A plugin's widget is shown on the web, on
Android and on the TV; on the TV it is displayed, not operated.
[Plugins](plugins.md) ·
[Security model and plugin sandbox](SECURITY.md#sandbox-modes)

## Find your way

<div class="grid cards" markdown>

-   **[Install Evey](installation.md)**

    Requirements, the compose file, the first launch, the address to open.

-   **[First setup](getting-started/first-setup.md)**

    The wizard, your libraries, the people who will watch, the apps.

-   **[Administration](administration/libraries.md)**

    Libraries, users, profiles and parental control, remote access, graphics
    card, backups.

-   **[How playback is decided](playback/how-playback-is-decided.md)**

    Direct Play, Remux, Transcode, and how to read Stream info.

-   **[Troubleshooting](operations/troubleshooting.md)**

    One table per symptom: where to look, what to check, how to recover.

-   **[Plugins & API](plugins.md)**

    Installing and writing plugins and themes, the server API.

</div>

<p class="evey-meta">
This documentation follows the development branch. ·
<a href="https://github.com/Evey-project/evey">Source on GitHub</a> ·
Security reports: security@eveyproject.org ·
<a href="privacy/">Privacy policy</a>
</p>

<!-- sources
beta, no release tag: docs/installation.md (beta-0.0.1 image) ; git tag (none)
demo signs in by itself, shared admin: frontend/apps/web/src/routes/login.tsx - demoAutoLoginAllowed ; docs/adr/0031-demo-mode-is-a-boot-flag-on-the-production-binary.md
what you need: docs/installation.md ; docker-compose.yml (EVEY_HTTP_PORT 9091) ; docs/operations/advanced-deployments.md (ARM needs a release tag)
platforms: docs/getting-started/apps.md ; frontend/apps/mobile/app.json ; frontend/apps/tv/app.json
apps not on Google Play: play.google.com listing for com.evey.app answers 404 (checked 2026-10-04)
profiles, kids PIN, max age: docs/administration/users.md ; internal/app/user/service.go - ErrPINRequired, ErrInvalidPINFormat ; internal/domain/user/kids_pin_invariant.go
age limit on downloads: frontend/apps/mobile/src/lib/offlineAccess.ts - isAgeExceeded
offline 30 days: frontend/apps/mobile/src/lib/offlineAccess.ts - OFFLINE_DOWNLOAD_TTL_MS
quick connect: frontend/apps/tv/app/quick-connect.tsx ; i18n auth.quickConnect.*
watch together: docs/playback/watching-together.md ; internal/app/syncplay/hub.go
music, photos: proto/evey/v1/library.proto (MUSIC, PHOTOS) ; frontend/apps/tv/app/(auth)/music
three outcomes, no quality to pick: docs/playback/how-playback-is-decided.md ; internal/app/transcoding/decision.go ; docs/adr/0030-one-plan-one-decision.md
mkvgo in process, ffmpeg copy otherwise: docs/playback/mkvgo.md ; internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager ; docs/playback-serving.md
stream info, dashboard: internal/app/transcoding/route_tonemap_report_test.go ; i18n admin.nowPlaying.*
one compose file, no .env: docs/installation.md ; docs/adr/0028-install-is-one-compose-file.md
embedded db/search, loopback, generated secrets: docker/entrypoint.sh - ensure_secret, pg_start 127.0.0.1, meilisearch --http-addr 127.0.0.1:7700
media owner adopted, never root: docker/entrypoint.sh - PUID/PGID auto-detection, setpriv
calibration at a quiet moment, cards proved by a real encode: internal/app/transcoding/calibration_service.go - idleProbes ; internal/adapters/driven/ffmpeg/hw_readiness.go ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
lower rung, gpu to software, refusal reasons: internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper.GrantRoute ; internal/adapters/driving/http/transcode_vod_admission.go - writeShed ; frontend/packages/client/src/lib/shed.ts
remote access inferred: docs/adr/0026-remote-access-inferred-from-a-published-url.md
files never removed from disk: internal/app/library (no media removal ; watcher_probe.go removes only its probe file) ; docs/installation.md (:ro)
media health: internal/app/maintenance/seek_index.go - Verdict ; internal/app/maintenance/seek_index_reindex.go - DeepVerify ; internal/app/maintenance/repair_journal.go - Reversible ; docs/runbooks/media-health.md
no analytics, outside services, update check: docs/privacy.md ; internal/app/updatecheck/service.go
pre-migration dump: internal/adapters/driven/postgres/migrate.go ; docs/backups.md
contained plugins: docs/SECURITY.md (Administrator preview, A separate user, Resource limits, Sandbox modes) ; internal/app/plugin/install.go ; internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; internal/adapters/driven/pluginrunner/runner_resmon.go ; i18n admin.security.title
plugin widget on tv, not operated: frontend/apps/tv/src/features/plugins/PluginSlot.tsx ; docs/plugins/api-reference.md
-->
