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

## Caddy (bundled)

The repo ships [`docker/Caddyfile`](../docker/Caddyfile) (built into an image by
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy)): it gets automatic
HTTPS for your domain and proxies everything to `evey:9091`, with the streaming
exceptions already set. Add a `caddy` service to your compose (see the `caddy`
service in the repo [`docker-compose.yml`](../docker-compose.yml)) and set your
domain:

```env
EVEY_DOMAIN=media.example.com
```

## Nginx

Use [`examples/nginx/evey.conf`](../examples/nginx/evey.conf) (a ready compose is
at [`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
It proxies to `evey:9091` with streaming buffering disabled - no frontend build step.

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
