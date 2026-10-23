# Reverse proxy configurations

Evey serves the API and the web interface together on port **9091**, in plain
HTTP; a reverse proxy only has to terminate TLS and forward everything to
`evey:9091`. Pick your proxy below (bundled Caddy, Nginx, Traefik, Nginx Proxy
Manager, or one you already run), then read the two sections at the end if you
tune streaming or set your own security headers.

## Tell Evey about the proxy

Whatever the proxy, Evey needs its public address and which proxy to trust.
The place for both is **Admin → Remote Access**: an `https://` **External
URL** and, when the proxy is a container, its network in **Trusted proxies**.
Both apply without a restart. An `https://` address is what tells Evey a proxy
terminates TLS in front of it: cookies get the `Secure` flag, HSTS is sent and
`X-Forwarded-Proto` from a trusted proxy is believed. The remote-access switch
and the real client address are explained in
[Remote access and HTTPS](administration/remote-access.md).

The same answers can come from the environment, under `environment:` in the
compose file, for an installation described entirely as code (the panel wins
when both are set):

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12   # only needed if the proxy is a container; loopback is always trusted
```

The former `EVEY_FORWARDED_PROTO` is still accepted as an override but
deprecated: the scheme of the address is enough.

Every bundled example reads the domain from `EVEY_DOMAIN`, with a placeholder
in the file you copy: replace it with your domain there, on the `EVEY_DOMAIN:`
line of the proxy service (Caddy, Nginx) or in the `Host(...)` rule (Traefik).

## Caddy (bundled)

### Files

[`docker/Caddyfile`](../docker/Caddyfile), built into an image by
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy); the `caddy` service
in the repository's [`docker-compose.yml`](../docker-compose.yml).

### Set it up

1. Write your domain on the `EVEY_DOMAIN:` line of the `caddy` service and
   make sure ports 80 and 443 of the host reach the Caddy container.
2. Add the `caddy` service from the repository compose file to yours and run
   `docker compose up -d`. Caddy obtains a Let's Encrypt certificate for
   `EVEY_DOMAIN` on its own.
3. In **Admin → Remote Access**, set the `https://` **External URL** and
   `172.16.0.0/12` in **Trusted proxies**.

### What it already handles

Proxies everything to `evey:9091`, with the streaming paths flushed without
buffering and without a timeout. Curates the security headers and the
canonical CSP of the [security section](#security-headers--csp---read-this-if-your-proxy-sets-its-own)
and strips the app's own CSP so exactly one reaches the browser; the two pages
that carry their own policy keep it.

## Nginx

### Files

[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
and a ready compose overlay,
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml).

### Set it up

1. Write your domain on the `EVEY_DOMAIN:` line of the overlay. The nginx
   image renders the template at startup with `EVEY_DOMAIN` substituted.
2. Put the certificate and key where the template expects them, under the
   matching file names (`/etc/ssl/certs/<domain>.pem`,
   `/etc/ssl/private/<domain>.key` in the `nginx-certs` volume): the overlay
   does not obtain certificates for you.
3. `docker compose -f docker-compose.yml -f examples/nginx/docker-compose.nginx.yml up -d`.
4. In **Admin → Remote Access**, set the `https://` **External URL** and
   `172.16.0.0/12` in **Trusted proxies**.

### What it already handles

Proxies to `evey:9091` with buffering and timeouts off on the streaming paths,
the SyncPlay WebSocket upgraded, `X-Forwarded-For` and `X-Forwarded-Proto`
forwarded, and `client_max_body_size 2G` for plugin uploads and backup
restores. It passes the app's own security headers and CSP through.

## Traefik

### Files

[`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml).

### Set it up

1. Run Traefik with a `websecure` entrypoint, a `letsencrypt` certificate
   resolver and an external Docker network named `proxy`; the overlay labels
   assume those names.
2. Write your domain in the overlay's `Host(...)` rule, then:

    ```bash
    docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
    ```

3. In **Admin → Remote Access**, set the `https://` **External URL** and the
   `proxy` network's range in **Trusted proxies** (`172.16.0.0/12` covers
   Docker's default networks).

### What it already handles

The overlay only adds the router labels and joins the `proxy` network. Traefik
on its defaults neither buffers responses nor caps a response's duration, so
no streaming tuning is needed; it forwards `X-Forwarded-For` and
`X-Forwarded-Proto` on its own and passes the app's security headers through.

## Nginx Proxy Manager

### Files

None: everything is set in NPM's interface. The evey container must be on the
network NPM uses, see [A proxy you already run](#a-proxy-you-already-run-tls-terminated-upstream).

### Set it up

In **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: on** (SyncPlay). **Cache Assets: off** - it
   would cache media segments and playlists with a fixed expiry. Block Common
   Exploits can stay on (turn it off if a search or a plugin upload ever
   answers 403 for no reason).
2. **SSL**: your certificate, Force SSL, HTTP/2. HSTS is optional here - the
   app already sends it.
3. In **Admin → Remote Access**, set the `https://` **External URL** and the
   NPM network's range in **Trusted proxies**.

### What it already handles

NPM forwards `X-Forwarded-For`, `X-Forwarded-Proto` and the Host header on its
own, and its default timeouts never fire on Evey's streams. Nothing else is
required.

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

## A proxy you already run (TLS terminated upstream)

On a NAS that already has a reverse proxy (the vendor's, Nginx Proxy Manager,
a Traefik on a shared Docker network), no Caddy is needed: Evey emits its own
security headers and a complete CSP, and the streaming rules can only be
enforced by the proxy that faces the client anyway. A second proxy behind it
adds nothing.

### Set it up

Put the evey service on that proxy's network and target `evey-server:9091`
(the container name; the service name `evey` resolves too):

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
the streaming rules below on the streaming paths - the Nginx and Nginx Proxy
Manager sections above show the exact lines.

### What it already handles

That depends on the proxy: check that exactly one CSP header reaches the
browser (next sections) and run `evey admin doctor` once the trusted proxies
are set.

## Streaming - what a proxy's defaults do to it

Evey works behind a reverse proxy left on its defaults: every long-lived
response either sends bytes continuously (HLS segments, direct play,
transcode output) or keeps the connection alive (the event streams send a keep-alive), so a
default read timeout never fires. Two things are still worth knowing:

- **Buffering.** A proxy that buffers responses (nginx does by default) reads
  a direct-play file into its own buffers and temp files before the client
  gets it - functional, but a multi-GB film then transits through the proxy
  host's disk. `proxy_buffering off` on the streaming paths avoids that.
- **Scans.** `/api/v1/libraries/*/scan` streams progress for as long as the
  scan runs; a proxy that caps a response's total duration (rare - nginx,
  Caddy and Traefik do not by default) would cut it short. The scan itself
  continues server-side regardless.

The paths concerned:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

The bundled Caddy and the Nginx example set both (no buffering, no timeout)
on these paths. The Traefik example and a UI-driven proxy such as Nginx Proxy
Manager keep their defaults, which are fine; the NPM snippet above is optional.

## Security headers & CSP - read this if your proxy sets its own

Evey emits a complete Content-Security-Policy. If your proxy passes it through,
you are done.

Many setups instead strip it (`header_down -Content-Security-Policy` in Caddy)
and curate their own - that is fine, but then **your** policy must keep these
tokens or features break silently:

| Token | Directive | Breaks without it |
| --- | --- | --- |
| `'self'` | `frame-src` | Every plugin UI and theme script (blank iframe). They run in `/plugin-frame.html`, a page of Evey framed by the app (see below). |
| `'wasm-unsafe-eval'` | `script-src` | Browser AC-3/E-AC-3 playback (the ac3go WebAssembly decoder cannot compile, so the film plays muted). This is the narrow token that permits Wasm compilation only, not general `eval()`. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | The Google Cast sender SDK never loads, so nobody gets a cast button. It is the only way a browser can discover Chromecasts on the user's own LAN. |
| `blob:` | `media-src` | MSE playback (hls.js attaches the stream via a blob URL). |
| `blob:` | `worker-src` | hls.js silently falls back to main-thread demuxing (choppier on mobile). |

A known-good policy for a curating proxy, the one the bundled Caddy sends:

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
unrestricted. Keep them if you curate your own policy. `script-src` has no
`'unsafe-inline'`: a script injected into the page does not run. Do not add it
back - nothing in the app needs it.

Two pages carry their own policy, sent by the app; leave the app's header on
those two paths:

- `/plugin-frame.html`, the page plugin UIs and theme scripts run in. Its
  policy sandboxes it (even opened outside an iframe), limits its network to
  the hosts the plugin's manifest declares, and lets only the app frame it,
  so it also sends `X-Frame-Options: SAMEORIGIN`: do not overwrite that with
  `DENY` on this path, or every plugin UI stays blank.
- `/cast-receiver.html`, the page a Chromecast loads when it plays from Evey.
  The Cast player injects styles, fonts and images from `www.gstatic.com`
  into it. Leave the app's header on that path, or apply this policy to that
  path only:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

!!! warning "Exactly one CSP header must reach the browser"
    With two (the app's and your proxy's, or Caddy's and Nginx Proxy Manager's
    "Advanced" snippet), browsers enforce the *intersection*: the strictest
    wins and the failure is silent. Check with
    `curl -sI https://your.domain | grep -ci content-security-policy`, which
    must print `1`.

Two more checks after any change:

- **Keep the policy on one line.** A CSP split across lines in a proxy config
  is a classic silent corruption: the join swallows the characters at the seam
  (`...gstatic.com; connect-src` became `...gstaticonnect-src` once, which
  disabled the Cast SDK and merged two directives).
- **The plugin frame keeps its own policy:**
  `curl -sI https://your.domain/plugin-frame.html | grep -i -e content-security -e x-frame`
  must show `sandbox allow-scripts` and `SAMEORIGIN`.

## Real client IPs and the remote-access gate

Once the proxy forwards the real client address, Evey can tell a viewer at
home from one on the internet: that is what **Max Remote Bitrate** and the
remote-access switch act on. Two rules:

- The trusted proxies (panel or `EVEY_TRUSTED_PROXIES`) must cover the proxy:
  `172.16.0.0/12` when it is a container on the Docker network, nothing to add
  when it runs on the host, since loopback is always trusted and whatever you
  set is added to it.
- Chained proxies each need to preserve `X-Forwarded-For`. In Caddy that is
  `servers { trusted_proxies static private_ranges }`; nginx uses
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

Turning this on changes who counts as remote: a server whose remote access
was off starts refusing real remote clients
(`403 {"code":"remote_access_disabled"}`) although no application setting
changed. Evey opens remote access on its own when a public address is set and
no choice was made, logs the state at every boot and each refusal with the
client address, and `evey admin doctor` fails on "published but refusing".
The whole mechanism is described in
[Remote access and HTTPS](administration/remote-access.md#the-real-client-address).

<!-- sources
one port for API and UI, https mode from the scheme: internal/config/config.go (EVEY_FORWARDED_PROTO deprecated) ; internal/config/network_live.go - EffectiveHTTPS
bundled Caddy: docker/Caddyfile (streaming matcher, @ownPolicy, header_down CSP) ; docker/Dockerfile.caddy ; docker-compose.yml caddy service
nginx: examples/nginx/evey.conf.template ; examples/nginx/docker-compose.nginx.yml (nginx-certs volume, no ACME)
traefik: examples/traefik/docker-compose.traefik.yml (labels + proxy network only)
streaming paths: cmd/evey/gateway.go ; event streams keep-alive ; scan progress stream
csp and own-policy pages: internal/adapters/driving/http/helpers.go - securityHeadersMiddleware ; /plugin-frame.html ; /cast-receiver.html
trusted proxies, catch-all refused, doctor: internal/config/trusted_proxies.go ; cmd/evey/admin_ops.go - checkTrustedProxies
remote access inferred: docs/adr/0026-remote-access-inferred-from-a-published-url.md
labels: i18n admin.remoteAccess.*
-->
