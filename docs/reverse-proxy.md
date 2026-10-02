# Reverse Proxy

Evey serves **both the API and the web UI** on port **9091** (plain HTTP). A
reverse proxy only needs to terminate TLS and forward everything to
`evey:9091`: there is **no separate frontend to build or serve**.

Wherever you run a proxy, tell Evey its public URL and which proxy to trust.
The simplest place is **Admin → Remote Access**: an `https://` **External URL**
and, when the proxy is a container, its network in **Trusted proxies**. Both
apply without a restart. The same answers can come from the environment, for
an installation described entirely as code (the panel wins when both are set):

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # only needed if the proxy is a container; loopback is always trusted
```

An `https://` address is what tells Evey a proxy terminates TLS in front of
it: cookies get the `Secure` flag, HSTS is sent, and `X-Forwarded-Proto` from
a trusted proxy is honoured. No separate switch is needed (the former
`EVEY_FORWARDED_PROTO` is still accepted as an override, but deprecated).

## Streaming - what a proxy's defaults do to it

Evey works behind a reverse proxy left on its defaults: every long-lived
response either sends bytes continuously (HLS segments, direct play,
transcode output) or pings the client every 30 s (the event streams), so a
default read timeout never fires. Two things are still worth knowing:

- **Buffering.** A proxy that buffers responses (nginx does by default) reads
  a direct-play file into its own buffers and temp files before the client
  gets it - functional, but a multi-GB film then transits through the proxy
  host's disk. `proxy_buffering off` on the streaming paths avoids that.
- **Scans.** `/api/v1/libraries/*/scan` streams progress for as long as the
  scan runs; a proxy that caps a response's total duration (rare - nginx and
  Caddy do not by default) would cut it short. The scan itself continues
  server-side regardless.

The paths concerned:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

The bundled Caddy and the Nginx/Traefik examples below set both (no
buffering, no timeout) on these paths. With a UI-driven proxy such as Nginx
Proxy Manager, the defaults are fine and the tuning is an optional snippet.

## Security headers & CSP - read this if your proxy sets its own

Evey emits a complete Content-Security-Policy. If your proxy passes it through,
you're done.

Many setups instead strip it (`header_down -Content-Security-Policy` in Caddy)
and curate their own - that's fine, but then **your** policy must keep these
tokens or features break silently:

| Token | Directive | Breaks without it |
| --- | --- | --- |
| `'self'` | `frame-src` | Every plugin UI and theme script (blank iframe). They run in `/plugin-frame.html`, a page of Evey framed by the app (see below). |
| `'wasm-unsafe-eval'` | `script-src` | Browser AC-3/E-AC-3 playback (the ac3go WebAssembly decoder can't compile → the film plays muted). This is the narrow token that permits Wasm compilation only - NOT general `eval()`. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | The Google Cast sender SDK never loads → no cast button for anyone. It is the only way a browser can discover Chromecasts on the user's own LAN. |
| `blob:` | `media-src` | MSE playback (hls.js attaches the stream via a blob URL). |
| `blob:` | `worker-src` | hls.js silently falls back to main-thread demuxing (choppier on mobile). |

A known-good policy for a curating proxy:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src 'self' https://www.gstatic.com
```

`base-uri` and `object-src` have no fallback to `default-src`, so a policy that
omits them leaves `<base href>` injection and `<object>`/`<embed>` embedding
unrestricted. Keep them if you curate your own policy.

`script-src` has no `'unsafe-inline'`: a script injected into the page does
not run. Do not add it back - nothing in the app needs it.

Two pages carry their own policy, sent by the app; leave the app's header on
those two paths:

- `/plugin-frame.html`, the page plugin UIs and theme scripts run in. Its
  policy sandboxes it (even opened outside an iframe), limits its network to
  the hosts the plugin's manifest declares, and lets only the app frame it,
  so it also sends `X-Frame-Options: SAMEORIGIN`: do not overwrite that with
  `DENY` on this path, or every plugin UI stays blank.
- `/cast-receiver.html`, described below.

One page needs a wider policy: `/cast-receiver.html`, the page a Chromecast
loads when it plays from Evey. The Cast player injects styles, fonts and
images from `www.gstatic.com` into it, and the app sends that page its own
policy for this reason. Leave the app's header on that path, or apply this
policy to that path only:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

Rules of thumb:

- **Exactly one CSP header must reach the browser.** With two (e.g. the app's
  AND your proxy's, or Caddy's AND Nginx Proxy Manager's "Advanced" snippet),
  browsers enforce the *intersection* - the strictest wins and the failure is
  silent. Check with `curl -sI https://your.domain | grep -ci content-security-policy`
  → must print `1`.
- **Keep the policy on ONE line.** A CSP split across lines in a proxy config
  is a classic silent corruption: the join swallows the characters at the seam
  (`...gstatic.com; connect-src` became `...gstaticonnect-src` once, which
  disabled the Cast SDK and merged two directives).
- **Check the plugin frame keeps its own policy:**
  `curl -sI https://your.domain/plugin-frame.html | grep -i -e content-security -e x-frame`
  must show `sandbox allow-scripts` and `SAMEORIGIN`.

Every bundled example reads the domain from **one place**: `EVEY_DOMAIN` in
your `.env`. Set it once; Caddy, Nginx and Traefik all pick it up.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (bundled)

The repo ships [`docker/Caddyfile`](../docker/Caddyfile) (built into an image by
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy)): it gets automatic
HTTPS for `EVEY_DOMAIN` and proxies everything to `evey:9091`, with the
streaming exceptions already set. It also curates the canonical security
headers/CSP from the section above (and strips the app's own CSP so exactly
one reaches the browser). Add a `caddy` service to your compose (see the
`caddy` service in the repo [`docker-compose.yml`](../docker-compose.yml)).

### Behind a proxy you already run (TLS terminated upstream)

On a NAS that already has a reverse proxy (the vendor's, Nginx Proxy Manager,
a Traefik on a shared Docker network), **no Caddy is needed**: Evey emits its
own security headers and a complete CSP, and the streaming rules can only be
enforced by the proxy that faces the client anyway - a second proxy behind it
adds nothing. Put the evey service on that proxy's network and target
`evey-server:9091` (the container name; the service name `evey` resolves too):

```yaml title="docker-compose.yml"
services:
  evey:
    # no ports: the proxy is the only way in
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Then, in that proxy, for the route to `evey-server:9091`: forward
`X-Forwarded-For` and `X-Forwarded-Proto`, enable WebSocket support, and apply
the streaming rules above (no buffering, no read/write timeout on the
streaming paths) - the Nginx, Traefik and Nginx Proxy Manager sections below
show the exact lines.

## Nginx

Use [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
(a ready compose is at
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
The nginx image renders it at startup with `EVEY_DOMAIN` substituted -
certificates must exist under the matching file names. It proxies to
`evey:9091` with streaming buffering disabled - no frontend build step.

## Traefik

See [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml):

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

The evey container must be on the network NPM uses (see "Behind a proxy you
already run" above). Then, **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: on** (SyncPlay). **Cache Assets: off** - it
   would cache media segments and playlists with a fixed expiry. Block Common
   Exploits can stay on (turn it off if a search or a plugin upload ever
   answers 403 for no reason).
2. **SSL**: your certificate, Force SSL, HTTP/2. HSTS is optional here - the
   app already sends it.
That is all: NPM forwards `X-Forwarded-For` / `X-Forwarded-Proto` and the
Host header on its own, and its default timeouts never fire on Evey's
streams (see the streaming section above).

**Optional** - keep NPM from buffering direct-play files through its own disk:
**Advanced** → *Custom Nginx Configuration* (the `Custom Locations` tab cannot
express the regex):

```nginx
# Plugin uploads (up to 100 MB) and backup restores (up to 2 GB).
client_max_body_size 2G;

# Streaming - no buffering, no timeout.
location ~ ^/api/v1/media/.+/(hls|stream) {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# Library scans stream their progress for as long as the scan runs.
location ~ ^/api/v1/libraries/.+/scan {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Every other path (the web UI, the rest of the API, the SyncPlay WebSocket)
goes through NPM's own proxy block, which the Details tab configures. Skip
the snippet and nothing breaks - it only changes where big files are buffered.

## Real client IPs and the remote-access gate

Once your proxy forwards the real client address, Evey can tell a viewer at home
from one on the internet. That is what the WAN bitrate cap and the remote-access
gate act on, so the two settings only behave correctly together:

- The trusted proxies (panel or `EVEY_TRUSTED_PROXIES`) must cover the proxy (`172.16.0.0/12` when it is a
  container on the Docker network; nothing to add when it runs on the host,
  loopback is always trusted). Whatever you set is ADDED to the loopback
  baseline - it never replaces it, so a Docker value cannot silently stop the
  gate from seeing real client addresses.
- Chained proxies each need to preserve `X-Forwarded-For`. In Caddy that is
  `servers { trusted_proxies static private_ranges }`; nginx uses
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**Turning this on changes who counts as remote.** Before it, every request looked
like it came from the proxy's own private address, so everyone was classified
LAN. After it, real remote clients are correctly identified - and if remote
access is disabled they are refused on every route
(`403 {"code":"remote_access_disabled"}`), although no application setting
changed.

Evey guards against it: remote access defaults to enabled whenever a public
URL is configured and no explicit choice was made, the state is logged at every
boot, refusals are logged with the client IP, `evey admin doctor` fails loudly
on "published but refusing", and the admin panel shows a warning. If you change proxy trust settings, check
`evey admin doctor` afterwards.
