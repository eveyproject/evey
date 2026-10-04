---
hide:
  - navigation
  - toc
---

# Evey

Evey is a self-hosted server for your films, series, music and photos. You add
your media and the people who watch; Evey adapts playback to your server and to
each device on its own.

[Install Evey](installation.md){ .md-button .md-button--primary }
[Try the demo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Beta, no numbered release yet</span>
<span>Free software, AGPL-3.0</span>
<span>Demo sign-in: <code>demo</code> / <code>demo</code></span>
</p>

<p class="evey-platforms">
<strong>Watch</strong> in any web browser, on a Chromecast, on Android phones and
tablets, and on Android TV. The Android and Android TV apps are in testing and
not on Google Play yet.<br>
<strong>Server:</strong> one Docker container on an x86-64 computer or NAS.
</p>

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

## Run your media server, not its infrastructure

One container, one compose file, no `.env` to write.

- **Database and search engine included.** They are reachable only inside the
  container, with passwords generated on the first start.
- **File permissions adapted automatically**, when possible, so that repairs
  can write. The server itself never runs as root.
- **Less to size by hand.** The graphics card is detected, the transcoding
  capacity is measured, and the playback mode follows what each device can
  play.

Bringing your own database or search cluster stays possible:
[Advanced deployments](operations/advanced-deployments.md).

## How it works

<div class="evey-points" markdown>

<div markdown>

**Do as little work as possible**

**Direct Play** sends the file as it is. When the device can play the tracks
but not the file's container, **MKVGo** changes the container without
re-encoding the audio and video tracks, and without starting an extra program
for each playback; FFmpeg copies the tracks where MKVGo cannot. **FFmpeg** handles real conversions.
[MKVGo in Evey](playback/mkvgo.md)

</div>

<div markdown>

**Know what the machine can actually do**

An accelerator counts only once a real encode on it succeeds. At a quiet
moment the server times test encodes, then sends each stream to the cheapest
hardware that proved it can serve it. When room runs short, a new stream gets
a lower quality, or moves from a saturated graphics card to the processor; it
is refused, with a clear message, only when no capacity is left.
[Transcoding and server capacity](playback/transcoding.md)

</div>

<div markdown>

**Third-party code is contained, not trusted**

A plugin is analysed before install, gets only the permissions the
administrator accepted, and runs as its own process, with no direct network
access and capped resources. How strict the sandbox is depends on the host,
and the admin panel says which one is active.
[Security model and plugin sandbox](SECURITY.md#sandbox-modes)

</div>

<div markdown>

**Find broken media before playback does**

**Media Health** reads your MKV and WebM files and explains what it finds. A
repair runs on request, is checked before it replaces the file, keeps the
original by default and is journaled. The rest is reported, never rewritten
behind your back.
[Media health](runbooks/media-health.md)

</div>

</div>

## See what Evey decided, and why

The dashboard lists every playback: who, on which device, in **Direct Play**,
**Remux** or **Transcode**, and what is copied or encoded. In the player,
**Stream info** shows the plan actually served, and why the file was not played
as it is.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Stream info in the web player](images/player-stream-info.webp){ loading=lazy }
  <figcaption>Remux: the container changes, without re-encoding the video or the audio. "Copied" describes the tracks, not the playback method.</figcaption>
</figure>

[Reading Stream info](playback/how-playback-is-decided.md#reading-stream-info)

## Apps and extensions

A TV signs in from a phone or a computer with **Quick Connect**, without typing
a password. Plugins and themes extend the interface, and the same plugin runs
on the web, on Android and on the TV.
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
This documentation follows the development branch. ·
<a href="https://github.com/Evey-project/evey">Source on GitHub</a> ·
Security reports: security@eveyproject.org
</p>

<!-- sources
beta, no release tag: docs/installation.md (beta-0.0.1 image) ; git tag (none)
demo sign-in: docs/adr/0031-demo-mode-is-a-boot-flag-on-the-production-binary.md ; internal/app/demo
platforms: docs/getting-started/apps.md ; frontend/apps/mobile/app.json ; frontend/apps/tv/app.json
apps not on Google Play: play.google.com listing for com.evey.app answers 404 (checked 2026-10-04)
server: docs/installation.md (x86-64 host ; ARM64 needs a release tag)
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
