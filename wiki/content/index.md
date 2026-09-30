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
