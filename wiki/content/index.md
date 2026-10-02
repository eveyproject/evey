---
hide:
  - navigation
  - toc
---

# Evey

Evey is a self-hosted server for films, series, music and photos. It aims to
behave like an appliance: you give it your media and the people who watch,
and it works out the rest from the machine it runs on.

[Install Evey](installation.md){ .md-button .md-button--primary }
[Try the demo](https://demo.eveyproject.org){ .md-button }

## Run your media server, not its infrastructure

A normal installation is one container and one compose file, with no `.env`
to write. What a media server needs around it comes inside:

- **Database and search engine included.** Postgres and Meilisearch run in the
  same container, listen only inside it, and get passwords generated on the
  first start, kept owner-only in the data folder.
- **File ownership worked out.** When it can, the container adopts the owner of
  your media folder, so that repairs can write. The server itself never runs as
  root.
- **Less to size by hand.** The graphics card is detected, the transcoding
  capacity is measured, and the route for each playback follows what the
  device says it can play.

Bringing your own database or search cluster stays possible:
[Advanced deployments](operations/advanced-deployments.md).

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![The home page](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>The home page: resume playback, then each library.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![A film page](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>A film page, with its artwork, ratings and credits.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![The web player](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>The web player.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![The admin dashboard while two films play](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>The admin dashboard while two people watch: the picture each one is seeing, and how it is delivered.</figcaption>
</figure>

</div>

## How it works

<div class="evey-points" markdown>

<div markdown>

**Do less work, whenever possible**

- **Direct Play**: the file as it is; the server only reads it.
- **[MKVGo](playback/mkvgo.md)**: when only the container has to change, Evey's in-process media
  layer repackages the tracks: no re-encoding, no media process started per
  stream, no segments written to disk. FFmpeg copies them where it cannot.
- **FFmpeg**: for real transformations, and as a fallback when MKVGo cannot
  handle the structural work.

[MKVGo in Evey](playback/mkvgo.md)

</div>

<div markdown>

**Know what the machine can actually do**

An accelerator counts only once a real encode on it succeeds. At its first
quiet moment the server times real encodes from a 4K clip, up to 1440p on the
processor and up to 2160p on each accelerator, then sends every stream to the
cheapest route that proved it can serve it. When room runs short, a new
stream gets a lower quality, or moves from a full graphics card to the
processor; it is refused, with a clear message, only when no capacity is left.
[Transcoding and server capacity](playback/transcoding.md)

</div>

<div markdown>

**Third-party code is contained, not trusted**

A plugin is analysed before install, gets only the permissions declared and
accepted by the administrator, and runs as its own process, with no direct
network access and capped memory, processes and disk. In Docker on a recent
kernel it sees only its own directory, and a seccomp denylist blocks system
calls known to be dangerous; an older host gets a weaker sandbox, and the
admin panel says which one is active.
[Security model and plugin sandbox](SECURITY.md#sandbox-modes)

</div>

<div markdown>

**Find broken media before playback does**

**Media Health** reads your MKV and WebM files and explains what it finds.
What it can repair (a missing seek index, an MP4 named `.mkv`, tracks stored
in blocks) is repaired on request, checked before it replaces the file, with
the original kept by default and every change journaled. The rest is reported,
never rewritten behind your back.
[Media health](runbooks/media-health.md)

</div>

</div>

## See what Evey decided, and why

The dashboard above lists every playback: who, on which device, in **Direct
Play**, **Remux** or **Transcode**, whether video and audio are copied or
encoded and by what, the bitrate and the progress. In the player, **Stream
info** reflects the plan actually served, and says why the file was not
played as it is.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Stream info in the web player](images/player-stream-info.webp){ loading=lazy }
  <figcaption>Remux: the packaging changes, without re-encoding the video or the audio. "Copied" describes the tracks, not the playback method.</figcaption>
</figure>

[Reading Stream info](playback/how-playback-is-decided.md#reading-stream-info)

## Apps and extensions

A web app served by the server itself, apps for Android and Android TV
(signed in from a phone or a computer with Quick Connect), and casting to a Chromecast.
Plugins and themes extend the interface, and the same plugin runs on the web,
on Android and on the TV.
[Apps and devices](getting-started/apps.md) ·
[Plugins](plugins.md)

## Find your way

<div class="grid cards" markdown>

-   **[Install Evey](installation.md)**

    One container and one compose file. Requirements and first launch.

-   **[First setup](getting-started/first-setup.md)**

    The setup wizard, your libraries, the people who will use the server.

-   **[Administration](administration/libraries.md)**

    Libraries, users and profiles, remote access, hardware acceleration,
    backups.

-   **[How playback is decided](playback/how-playback-is-decided.md)**

    Direct Play, Remux and Transcode, and how to read Stream info.

-   **[Troubleshooting](operations/troubleshooting.md)**

    Where to look when something does not work: symptoms, logs, recovery.

-   **[Plugins & API](plugins.md)**

    Installing and writing plugins and themes, the server API.

</div>

<p class="evey-meta">
Evey is in beta and has no numbered release yet: this documentation follows the
development branch. ·
<a href="https://github.com/Evey-project/evey">Source on GitHub</a> ·
<a href="https://demo.eveyproject.org">Public demo</a> (sign in with demo / demo) ·
AGPL-3.0-or-later ·
Security reports: security@eveyproject.org
</p>

<!-- sources
one container, no .env: docs/installation.md ; docs/adr/0028-install-is-one-compose-file.md
embedded db/search, loopback, generated secrets: docker/entrypoint.sh - ensure_secret, pg_start 127.0.0.1, meilisearch --http-addr 127.0.0.1:7700
media owner adopted, never root: docker/entrypoint.sh - PUID/PGID auto-detection, setpriv
do less work: docs/playback/mkvgo.md ; internal/app/transcoding/decision.go ; internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager (no ffmpeg, no session state, no on-disk segments) ; docs/playback-serving.md (copy rung: mkvgo, else ffmpeg -c copy)
less to size: internal/adapters/driven/ffmpeg/hw_readiness.go ; internal/app/transcoding/calibration_service.go ; docs/playback/how-playback-is-decided.md
accelerators proved, quiet moment: internal/app/transcoding/calibration_service.go - idleProbes ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
1440p software, 2160p hardware: internal/adapters/driven/ffmpeg/benchmark.go - swBenchLadder, hwBenchLadder
lower rung, gpu to software, then refused: internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper.GrantRoute ; internal/app/transcoding/video_session_limiter.go - VideoSessionLimiter, ErrVideoCapacityExhausted
contained plugins: docs/SECURITY.md (Administrator preview, System calls: denylist) ; internal/app/plugin/install.go ; internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; internal/adapters/driven/pluginrunner/runner_resmon.go
media health: internal/app/maintenance/seek_index.go - Verdict ; internal/app/maintenance/seek_index_reindex.go - DeepVerify ; internal/app/maintenance/repair_journal.go - Reversible
stream info, one plan: docs/adr/0030-one-plan-one-decision.md ; internal/app/transcoding/route_tonemap_report_test.go
one plugin, three screens: frontend/packages/client/src/native/PluginWebView.tsx ; frontend/apps/tv/src/features/plugins/PluginSlot.tsx
-->
