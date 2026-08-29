# Reverse Proxy

Evey serves **both the API and the web UI** on port **9091** (plain HTTP). A
reverse proxy only needs to terminate TLS and forward everything to `evey:9091`
- there is **no separate frontend to build or serve**.

Wherever you run a proxy, set these so Evey knows its public URL and trusts the
proxy's forwarded headers:

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # only needed if the proxy is a container; loopback is always trusted
EVEY_FORWARDED_PROTO=true
```

## Streaming - required for every proxy

These endpoints stream and must run with **response buffering off** and **no
read/write timeout**, or playback and scans stall:

```
/api/v1/media/*/transcode
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

All the example configs below already handle this. If you write your own, don't forget.

## Security headers & CSP - read this if your proxy sets its own

Evey emits a complete Content-Security-Policy. If your proxy passes it through,
you're done.

> Until 2026-07-23 that sentence was wrong: the app's policy was missing
> `media-src` and `worker-src`, so `blob:` fell back to `default-src 'self'` and
> MSE playback broke for anyone who passed it through. It went unnoticed because
> the bundled Caddy config strips the app header and curates its own. If you run
> a version older than that behind Nginx Proxy Manager or Traefik and your player
> fails to start, this is why - curate the policy below at the proxy, or upgrade.

Many setups instead strip it (`header_down -Content-Security-Policy` in Caddy)
and curate their own - that's fine, but then **your** policy must keep these
tokens or features break silently:

| Token | Directive | Breaks without it |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | Every plugin UI (blank iframe). A plugin bundle is fetched with the user's token and inlined into a `srcdoc` iframe, and a srcdoc document inherits the embedder's policy. The iframe's `sandbox="allow-scripts"` (opaque origin, no parent DOM/cookie access) is what bounds the plugin meanwhile. |
| `'wasm-unsafe-eval'` | `script-src` | Browser AC-3/E-AC-3 playback (the ac3go WebAssembly decoder can't compile → the film plays muted). This is the narrow token that permits Wasm compilation only - NOT general `eval()`. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | The Google Cast sender SDK never loads → no cast button for anyone. It is the only way a browser can discover Chromecasts on the user's own LAN. |
| `blob:` | `media-src` | MSE playback (hls.js attaches the stream via a blob URL). |
| `blob:` | `worker-src` | hls.js silently falls back to main-thread demuxing (choppier on mobile). |

A known-good policy for a curating proxy:

```
default-src 'self'; img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
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
- `'unsafe-inline'` in `script-src` is a real cost, and it is currently
  load-bearing (plugin UIs, see the table). Dropping it is only safe on a
  deployment that runs no plugin frontends.

Every bundled example reads the domain from **one place**: `EVEY_DOMAIN` in
your `.env`. Set it once; Caddy, Nginx and Traefik all pick it up.

```env
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
`evey-server:9091` (`evey init --behind-proxy <network> --domain <name>`
writes exactly this):

```yaml
services:
  evey:
    # no ports: the proxy is the only way in
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
      EVEY_FORWARDED_PROTO: "true"
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

1. Add a proxy host: your domain → `evey:9091`, enable **WebSocket support**.
2. SSL tab: request a Let's Encrypt certificate.
3. Advanced tab: paste the streaming `location` blocks from
   [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template) so playback isn't buffered.

## Real client IPs and the remote-access gate

Once your proxy forwards the real client address, Evey can tell a viewer at home
from one on the internet. That is what the WAN bitrate cap and the remote-access
gate act on, so the two settings only behave correctly together:

- `EVEY_TRUSTED_PROXIES` must cover the proxy (`172.16.0.0/12` when it is a
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
(`403 {"code":"remote_access_disabled"}`). That exact sequence locked a whole
household out of a working server for two days: the proxy config changed, no
application setting did, and the only visible symptom was clients failing with
no server-side error.

Evey now guards against it: remote access defaults to enabled whenever a public
URL is configured, the state is logged at every boot, refusals are logged with
the client IP, `evey admin doctor` fails loudly on "published but refusing", and
the admin panel shows a warning. If you change proxy trust settings, check
`evey admin doctor` afterwards.
