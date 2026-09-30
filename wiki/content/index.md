---
hide:
  - navigation
  - toc
---

# Evey documentation

Evey is a self-hosted server for films, series, music and photos. For every
playback it compares the file, the player and the server, keeps the original
whenever the player can use it, and re-encodes only what it must. Most
installations need nothing beyond libraries and users: hardware, capacity and
playback routes are detected and measured by the server itself.

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

## Better with less

<div class="evey-points" markdown>

<div markdown>

**The cheapest route first**

The file as it is, then repackaged without an encoder, then only the tracks
that need it re-encoded. A viewer in Direct Play or Remux costs the server
little more than a file read, so a modest machine serves many at once.
[How playback is decided](playback/how-playback-is-decided.md)

</div>

<div markdown>

**A server that measures itself**

It times real 4K encodes on the processor and each graphics card, and uses
a card only for what it proved. No capacity to estimate.
[Transcoding and server capacity](playback/transcoding.md)

</div>

<div markdown>

**Full means refused, not slower**

No more encodes than measured; when full, the player is told the server is
busy instead of every stream degrading. Background work pauses during playback.
[Background work and server load](operations/background-work.md)

</div>

<div markdown>

**One plugin, three screens**

The same plugin bundle runs on the web, on Android and on Android TV, and
knows which screen it landed on.
[Plugin API reference](plugins/api-reference.md#on-a-television)

</div>

<div markdown>

**Plugins are contained, not trusted**

Code analysed before install, its own directory only, dangerous system calls
refused, no direct network, a token denied by default.
[Security model and plugin sandbox](SECURITY.md)

</div>

<div markdown>

**What Stream info says is what runs**

One decision per playback, and Stream info reads it from the plan the encoder
runs, with the reason the file was not played as it is.
[How playback is decided](playback/how-playback-is-decided.md#reading-stream-info)

</div>

</div>

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
cheapest route: docs/design-principles.md (3) ; docs/playback-serving.md (mkvgo, no process) ; internal/app/transcoding/decision.go - wasmAudioApplies ; internal/config/config.go (EVEY_TRANSCODING_SINGLE_VARIANT)
measures itself: internal/app/transcoding/calibration_service.go ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
full means refused: internal/app/transcoding/admission_shaper.go ; internal/app/transcoding/video_session_limiter.go ; i18n errors max_concurrent_reached
one plugin, three screens: frontend/packages/client/src/native/PluginWebView.tsx ; frontend/apps/mobile/src/features/plugins/PluginSlot.tsx ; frontend/apps/tv/src/features/plugins/PluginSlot.tsx
contained plugins: docs/SECURITY.md ; docs/adr/0012-plugin-token-deny-by-default.md ; docs/adr/0014-landlock-plugin-sandbox.md
one decision: docs/adr/0030-one-plan-one-decision.md ; internal/app/transcoding/route_tonemap_report_test.go
-->
