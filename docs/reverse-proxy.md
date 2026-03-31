# Reverse Proxy Setup

Evey exposes a single HTTP port. Put a reverse proxy in front of it for HTTPS and frontend serving.

| Port | Protocol | Description |
|------|----------|-------------|
| 9091 | HTTP | REST API, streaming, HLS |
| 9090 | gRPC | Internal only (plugins). Do NOT expose. |

## Environment Variables

Set these in your `.env` when using a reverse proxy:

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_FORWARDED_PROTO=true
```

## Important: Streaming Endpoints

These paths handle long-running connections (transcoding, HLS, library scans). Your proxy **must** disable buffering and timeouts for them:

```
/api/v1/media/*/transcode
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

---

## Caddy

The easiest option. Caddy handles HTTPS certificates automatically.

### Option A: Bundled (Docker)

```bash
echo 'EVEY_DOMAIN=media.example.com' >> .env
docker compose --profile caddy up -d
```

The bundled Caddy builds and serves the frontend, proxies API requests to the backend, and manages TLS.

### Option B: Standalone

```caddyfile
media.example.com {
    encode gzip

    header {
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy strict-origin-when-cross-origin
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }

    @streaming path /api/v1/media/*/transcode /api/v1/media/*/hls/* /api/v1/media/*/stream /api/v1/libraries/*/scan
    handle @streaming {
        reverse_proxy localhost:9091 {
            transport http {
                read_timeout 0
                write_timeout 0
            }
            flush_interval -1
        }
    }

    handle /api/* {
        reverse_proxy localhost:9091 {
            flush_interval -1
        }
    }

    # Serve the frontend (build it first: cd frontend && pnpm install && pnpm --filter web build)
    handle {
        root * /path/to/frontend/apps/web/dist
        try_files {path} /index.html
        file_server
    }
}
```

---

## Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name media.example.com;

    ssl_certificate     /etc/ssl/certs/media.example.com.pem;
    ssl_certificate_key /etc/ssl/private/media.example.com.key;

    client_max_body_size 100M;

    # Frontend (static files)
    root /path/to/frontend/apps/web/dist;
    index index.html;

    # Streaming endpoints — no buffering, no timeout
    location ~ ^/api/v1/media/.+/(transcode|hls|stream) {
        proxy_pass http://127.0.0.1:9091;
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

    location ~ ^/api/v1/libraries/.+/scan {
        proxy_pass http://127.0.0.1:9091;
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

    # WebSocket (SyncPlay)
    location /api/v1/syncplay/ {
        proxy_pass http://127.0.0.1:9091;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # API
    location /api/ {
        proxy_pass http://127.0.0.1:9091;
        proxy_buffering off;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # SPA fallback
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

Save as `/etc/nginx/sites-available/evey` and symlink to `sites-enabled`.

---

## Traefik

Add labels to the evey service in your `docker-compose.yml`:

```yaml
services:
  evey:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.evey.rule=Host(`media.example.com`)"
      - "traefik.http.routers.evey.entrypoints=websecure"
      - "traefik.http.routers.evey.tls.certresolver=letsencrypt"
      - "traefik.http.services.evey.loadbalancer.server.port=9091"
      # Disable buffering for streaming
      - "traefik.http.middlewares.evey-buffering.buffering.maxRequestBodyBytes=0"
      - "traefik.http.middlewares.evey-buffering.buffering.maxResponseBodyBytes=0"
      - "traefik.http.routers.evey.middlewares=evey-buffering"
    networks:
      - proxy

networks:
  proxy:
    external: true
```

For the frontend, serve the built dist with a separate container or file server behind Traefik.

---

## Nginx Proxy Manager

1. Create a **Proxy Host**:
   - Domain: `media.example.com`
   - Scheme: `http`
   - Forward Hostname: `evey` (Docker service name) or host IP
   - Forward Port: `9091`
   - Enable **Websockets Support**
   - Enable **Block Common Exploits**

2. **SSL tab**: Request a Let's Encrypt certificate

3. **Advanced tab** — add this custom config:

```nginx
location ~ ^/api/v1/media/.+/(transcode|hls|stream) {
    proxy_pass http://evey:9091;
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

location ~ ^/api/v1/libraries/.+/scan {
    proxy_pass http://evey:9091;
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

Set these in your `.env`:
```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12
EVEY_REAL_IP_HEADER=X-Real-IP
EVEY_FORWARDED_PROTO=true
```

---

## Docker Networking

If your reverse proxy runs in Docker, it must share a network with Evey:

```yaml
services:
  evey:
    networks:
      - proxy

networks:
  proxy:
    external: true
```

Reference Evey by its Docker service name (`evey`). If using an external proxy on the host, use `localhost:9091` or expose the port.

Port 9090 (gRPC) is internal only — used for plugin communication. Never expose it.

## Serving the Frontend

The Evey backend does not serve the frontend. You have three options:

1. **Bundled Caddy** (`docker compose --profile caddy up -d`) — builds and serves frontend automatically
2. **Build yourself** and serve with your reverse proxy:
   ```bash
   cd frontend && pnpm install && pnpm --filter web build
   # Output: frontend/apps/web/dist/
   ```
3. **Separate static hosting** (S3, Cloudflare Pages, Vercel) — point it to the built dist
