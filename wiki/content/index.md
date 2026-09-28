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

**Your films, your series, your music and your photos, streamed from your own
machine to every screen in the house - and beyond it.**

Evey is a self-hosted media server. You point it at the folders you already
have, it recognises what is in them, dresses them with posters, summaries and
cast, and plays them on the web, on Android phones, on Android TV and on a
Chromecast. Nothing leaves your home unless you publish it, and nobody else
decides what you can watch.

[Discover Evey](philosophy.md){ .md-button .md-button--primary }
[Install it](installation.md){ .md-button }
[Try the demo](https://demo.eveyproject.org){ .md-button }

Want to click around first? The public demo at [demo.eveyproject.org](https://demo.eveyproject.org)
runs the real server with free films: sign in with `demo` / `demo`. It is
reset regularly, and the administration screens are visible but mostly
read-only.

## Two ways in

<div class="grid cards" markdown>

-   __Discover__

    ---

    What Evey is trying to be, what it does for the people who watch, and the
    devices it plays on. No terminal required.

    [Why Evey](philosophy.md) · [What you get](discover/features.md) ·
    [On every screen](discover/apps.md)

-   __Technical guide__

    ---

    Installing the server, the first ten minutes after it starts, every setting
    a viewer or an administrator can change, and what to do on the day
    something goes wrong.

    [Installation](installation.md) · [First steps](guide/first-steps.md) ·
    [Administration](guide/admin-settings.md)

</div>

## Why people run it

- **It plays the file you have.** When a device can read a file as it is,
  Evey sends it untouched: full quality, no work for the server. When it
  cannot, Evey converts only what is missing, with the graphics card when
  there is one.
- **It installs in one file.** One container, one compose file, no password to
  invent. The database and the search engine live inside and set themselves up.
- **It is made for a household.** Profiles for everyone, kids profiles behind
  a PIN, age limits enforced by the server rather than by the app, and a room
  to watch together from different homes.
- **It is safe to open to the internet.** Remote access opens when you
  publish the server and stays shut otherwise, sign-in fails closed, and
  extensions run in a sandbox with only the permissions they declared.
- **It is free software.** Licensed under the
  [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html). The code is
  on [GitHub](https://github.com/Evey-project/evey), and so is everything the
  server still cannot protect you from.

## Quick links

| You want to | Read |
| --- | --- |
| Try it on your machine | [Installation](installation.md) |
| Put it on the internet with HTTPS | [Reverse proxy](reverse-proxy.md) |
| Use your graphics card for conversion | [Transcoding and hardware](transcoding.md) |
| Know what each setting does | [Viewer settings](guide/viewer-settings.md) · [Administration](guide/admin-settings.md) |
| Get a server back after a crash | [Recovery](runbooks/recovery.md) |
| Write an extension | [Plugins](plugins.md) |

Security reports go to `security@eveyproject.org` - see the
[plugin sandbox and threat model](SECURITY.md).
