# Reverse Proxy

Evey serves **both the API and the web UI** on port **9091** (plain HTTP). A
reverse proxy only needs to terminate TLS and forward everything to `evey:9091`
- there is **no separate frontend to build or serve**.

Wherever you run a proxy, set these so Evey knows its public URL and trusts the
proxy's forwarded headers:

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=127.0.0.1/32        # or 172.16.0.0/12 if the proxy is a container on the Docker network
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

Evey already emits a Content-Security-Policy. If your proxy passes it through,
you're done. Many setups instead strip it (`header_down -Content-Security-Policy`
in Caddy) and curate their own - that's fine, but then **your** policy must keep
these tokens or features break silently:

| Token | Directive | Breaks without it |
| --- | --- | --- |
| `'wasm-unsafe-eval'` | `script-src` | Browser AC-3/E-AC-3 playback (the ac3go WebAssembly decoder can't compile → the film plays muted). This is the narrow token that permits Wasm compilation only - NOT general `eval()`. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | The Google Cast sender SDK never loads → no cast button for anyone. It is the only way a browser can discover Chromecasts on the user's own LAN. |
| `blob:` | `media-src` | MSE playback (hls.js attaches the stream via a blob URL). |
| `blob:` | `worker-src` | hls.js silently falls back to main-thread demuxing (choppier on mobile). |

A known-good policy for a curating proxy:

```
default-src 'self'; img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline'; script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

Two rules of thumb:

- **Exactly one CSP header must reach the browser.** With two (e.g. the app's
  AND your proxy's, or Caddy's AND Nginx Proxy Manager's "Advanced" snippet),
  browsers enforce the *intersection* - the strictest wins and the failure is
  silent. Check with `curl -sI https://your.domain | grep -ci content-security-policy`
  → must print `1`.
- Avoid `'unsafe-inline'` in `script-src`: the app does not need it (only
  `style-src` does), and it defeats most of the XSS protection a CSP provides.

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
   [`examples/nginx/evey.conf`](../examples/nginx/evey.conf) so playback isn't buffered.
