---
hide:
  - navigation
---

<pre class="evey-wordmark" aria-hidden="true"><span>███████╗</span><span class="v">██╗   ██╗</span><span>███████╗██╗   ██╗</span>
<span>██╔════╝</span><span class="v">██║   ██║</span><span>██╔════╝╚██╗ ██╔╝</span>
<span>█████╗  </span><span class="v">██║   ██║</span><span>█████╗   ╚████╔╝ </span>
<span>██╔══╝  </span><span class="v">╚██╗ ██╔╝</span><span>██╔══╝    ╚██╔╝  </span>
<span>███████╗</span><span class="v"> ╚████╔╝ </span><span>███████╗   ██║   </span>
<span>╚══════╝</span><span class="v">  ╚═══╝  </span><span>╚══════╝   ╚═╝   </span></pre>

# Evey

Self-hosted media server for movies, TV shows, music and photos. One container,
your own hardware, your own files.

[Install Evey](installation.md){ .md-button .md-button--primary }
[Browse the source](https://github.com/Evey-project/evey){ .md-button }

## Start here

<div class="grid cards" markdown>

-   __Install__

    ---

    One compose file, no secret to invent. Postgres and the search index run
    inside the container and generate their own credentials on first start.

    [Installation guide](installation.md)

-   __Configure__

    ---

    Everything an operator can change, and where it lives. Almost all of it is
    in the admin panel rather than in environment variables.

    [Configuration reference](configuration.md)

-   __Put it behind a proxy__

    ---

    Caddy, nginx, Traefik and Apache, with the headers that streaming and
    WebSocket playback actually need.

    [Reverse proxy guide](reverse-proxy.md)

-   __Keep it safe__

    ---

    What the server backs up on its own, what it does not, and how to restore
    a database that will not start.

    [Backups](backups.md) · [Recovery](runbooks/recovery.md)

</div>

## What it does

- **Plays what you own.** Direct play whenever the client can handle the file
  untouched, hardware or software transcoding when it cannot, and an HLS
  pipeline that switches audio track and quality mid-stream.
- **Follows you across devices.** Web, Android, Android TV and Chromecast, with
  resume points, watch state and downloads for offline playback.
- **Belongs to the household.** Profiles with their own theme and language,
  parental control enforced on the server, shared watching with SyncPlay.
- **Extends without a rebuild.** Plugins run sandboxed with scoped tokens and
  a deny-by-default permission model, and themes are data rather than code.

## Contributing

The [architecture](ARCHITECTURE.md) page explains how the server is laid out,
and the [architecture decisions](adr/README.md) record why each structural
choice was made. Plugin authors start at the
[plugin developer setup](plugins/dev-setup.md).

Evey is licensed under the [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html).
Security reports go to `security@eveyproject.org` - see the
[security model](SECURITY.md).
