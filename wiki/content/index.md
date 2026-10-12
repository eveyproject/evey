---
hide:
  - navigation
  - toc
---

# Evey

Evey is a media server you run at home. It holds your films, series, music
and photos, and the people in your household watch them on their own
devices. You never pick a quality: you press play, and Evey works out the
rest.

[Install Evey](installation.md){ .md-button .md-button--primary }
[Try the demo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Beta, no numbered release yet</span>
<span>Free software, AGPL-3.0</span>
<span>Demo: signs you in by itself (<code>demo</code> / <code>demo</code>)</span>
</p>

<p class="evey-platforms">
<strong>What you need:</strong> an x86-64 computer or NAS with Docker, and a
folder with your media. Installing is one file and one command.<br>
<strong>Watch</strong> in a web browser, on Android phones and tablets, on
Android TV, and on a Chromecast. The Android and Android TV apps are in
testing and not on Google Play yet.
</p>

<div class="grid cards" markdown>

-   **You will run the server**

    Install Evey on your computer or NAS, add your media, invite your
    household.
    [Start with Install](installation.md)

-   **Someone gave you an address**

    Somebody at home already runs Evey and you just want to watch.
    [Read Watching with Evey](getting-started/watching.md)

</div>

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

- **A profile per person.** Each profile keeps its own history, resume
  points, favourites and languages. A household usually shares one account,
  with one profile per person.
- **Kids profiles.** A kids profile opens without a code. Leaving it for an
  adult profile asks for a four-digit PIN. A kids profile has an age limit,
  and the server never sends a title above it, not even as a download.
- **Pick up where you left off**, on any device. Resume points are kept on
  the server, not on the phone.
- **On the go.** The Android app can download a title to watch offline, for
  30 days.
- **On the TV.** You sign in an Android TV with **Quick Connect**: the TV
  shows a code, and you approve it from your phone or computer. No password
  to type with the remote.
- **Together.** Several people can watch the same title at the same moment,
  each on their own device.

Music and photos work the same way as films and series, on the web, on
Android and on the TV.
[Connect the apps](getting-started/apps.md) ·
[Users, profiles and parental control](administration/users.md)

## Nothing to choose before pressing play

Some files play everywhere. Others are too heavy for an old phone, or use a
sound format a TV cannot read. Each time you press play, the server looks at
the file, at the device, and at how busy it is, then picks the lightest way
that works:

- **Direct Play**: the file is sent as it is.
- **Remux**: the picture and sound are copied, unchanged, into a file format
  the device accepts. This costs the server almost nothing. Evey has its own
  tool for this, **MKVGo**, and uses FFmpeg when MKVGo cannot.
- **Transcode**: the server converts only what the device cannot play, often
  just the sound. This is the only case that keeps the processor busy.

So the same film can go untouched to a TV and be converted for an older
phone. In the player, **Stream info** shows which of the three you got, and
why. **Admin → Dashboard** shows the same for everyone watching.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Stream info in the web player](images/player-stream-info.webp){ loading=lazy }
  <figcaption>The first playback of the dashboard above, seen from the player. Remux: the file format changes, picture and sound are copied, and the last line says why the file could not be sent as it is.</figcaption>
</figure>

[How playback is decided](playback/how-playback-is-decided.md) ·
[MKVGo in Evey](playback/mkvgo.md)

## One container, nothing to tune

The whole installation is one compose file. There is no `.env` to write and
no memory or CPU setting.

- **Everything is inside.** The database and the search engine run inside
  the container. Their passwords are generated on the first start.
- **The server never runs as root.** It runs as the owner of your media
  folder when it can detect one, so that repairs can write.
- **The server measures itself.** At a quiet moment, it times real
  conversions on the processor and on any graphics card you gave it. It then
  knows how many people it can serve at once. When it runs short, a new
  playback gets a lower quality, or moves from a busy graphics card to the
  processor. If a playback cannot start, the message says why: the server is
  full, it is still measuring itself, or the file is not ready.
- **Access from outside your home is off** until you give the server a
  public address.

You can also bring your own database or search engine:
[Advanced deployments](operations/advanced-deployments.md).
[Transcoding and server capacity](playback/transcoding.md)

## What Evey does with your files and your data

**Your files.** Removing a title or a library removes it from Evey, never
from the disk. Evey writes into your media folders in three cases only: a
repair you asked for, a subtitle fetched through the API, and a small test
file that checks Evey still sees new files arrive. Mount your media read-only
and none of that happens. **Media Health** checks your MKV and WebM files and
lists the problems it finds. It repairs a file only when you ask, works on a
copy that is checked in full before it replaces the file, keeps the original
by default, and records every change. What cannot be repaired is reported
and left alone.
[Media health](runbooks/media-health.md)

**Your data.** The project runs no service that receives it: no account with
us, no analytics, no crash reports. The apps talk to your server and to
nothing else we operate. What leaves your network: the server fetches
artwork and descriptions from outside services (The Movie Database, and cover
art sources for music), and from time to time asks whether a newer version
exists, which you can switch off. The database is saved before every update
that changes it.
[Privacy policy](privacy.md) · [Backups and updates](backups.md)

## Plugins run isolated and controlled

Plugins add metadata sources, notification channels, home sections and
player overlays. Themes change the look. Before a plugin is installed, the
administrator sees what it was found to do and the permissions it asks for;
a new permission must be accepted explicitly. A plugin runs as its own
process, under a separate user, with no direct network access, and is
stopped when it goes over its memory, process or disk limit. How tightly its
files are locked down depends on the machine, and the **Security Dashboard**
shows the mode in use. A plugin's widget shows on the web, on Android and on
the TV; on the TV you can see it but not use it.
[Plugins](plugins.md) ·
[Security model and plugin sandbox](SECURITY.md#sandbox-modes)

## Find your way

<div class="grid cards" markdown>

-   **[Install Evey](installation.md)**

    What you need, the compose file, the first start, the address to open.

-   **[First setup](getting-started/first-setup.md)**

    The setup wizard, your libraries, the people who will watch.

-   **[Connect the apps](getting-started/apps.md)**

    The address to type on a phone or TV, signing in, and what each error
    means.

-   **[Watching with Evey](getting-started/watching.md)**

    For the people who watch: profiles, finding a title, subtitles, asking
    for a film.

-   **[Administration](administration/libraries.md)**

    Libraries, users and parental control, access from outside, graphics
    card, backups.

-   **[Troubleshooting](operations/troubleshooting.md)**

    One table per problem: where to look, what to check, how to fix it.

-   **[How playback is decided](playback/how-playback-is-decided.md)**

    Direct Play, Remux, Transcode, and how to read Stream info.

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
two entry points (runs the server / was given an address): docs/installation.md ; docs/getting-started/watching.md
kids profile age limit: docs/administration/users.md (Max age rating, kids profiles only)
security dashboard label: i18n admin.security.title
-->
