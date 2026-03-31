# Reverse Proxy

Evey listens on port 9091 (HTTP). It doesn't handle HTTPS — put a reverse proxy in front.

## Bundled Caddy (easiest)

```bash
# Add to .env
EVEY_DOMAIN=media.example.com

# Start with Caddy
docker compose --profile caddy up -d
```

Caddy handles HTTPS certificates automatically. It builds and serves the frontend too.

## Nginx

Copy [`examples/nginx/evey.conf`](../examples/nginx/evey.conf) to `/etc/nginx/sites-enabled/`.

Build the frontend first:
```bash
cd frontend && pnpm install && pnpm --filter web build
# Copy frontend/apps/web/dist/ to /var/www/evey/
```

Set in `.env`:
```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=127.0.0.1/32
EVEY_FORWARDED_PROTO=true
```

## Traefik

See [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml).

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

You'll need to serve the frontend separately (static file server behind Traefik, or the bundled Caddy).

## Nginx Proxy Manager

1. Add proxy host: domain → `evey:9091`, enable WebSocket support
2. SSL tab: request Let's Encrypt cert
3. Advanced tab: paste the streaming location blocks from [`examples/nginx/evey.conf`](../examples/nginx/evey.conf)

## Important

Streaming endpoints (`/api/v1/media/*/transcode`, `/api/v1/media/*/hls/*`, `/api/v1/media/*/stream`) need **buffering disabled** and **no timeout**. All example configs handle this. If you write your own, don't forget.

## Frontend Serving

The backend doesn't serve the frontend. Options:
1. **Bundled Caddy** — handles everything
2. **Build and serve yourself** — `cd frontend && pnpm install && pnpm --filter web build`, serve `apps/web/dist/` as static files with SPA fallback (`try_files $uri /index.html`)
