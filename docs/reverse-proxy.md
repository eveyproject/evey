# Reverse Proxy Setup

Evey exposes two ports:

| Port | Protocol | Description |
|---|---|---|
| 9090 | gRPC | Plugin IPC, inter-service communication |
| 9091 | HTTP | REST API, streaming, HLS, static assets |

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | Public URL used for generating links |
| `EVEY_TRUSTED_PROXIES` | `127.0.0.1/32, ::1/128` | Comma-separated CIDR list of trusted proxy IPs |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | Header to read the real client IP from |
| `EVEY_FORWARDED_PROTO` | `false` | Trust `X-Forwarded-Proto` to detect HTTPS |

## Caddy

The project includes a production Caddyfile at `docker/Caddyfile`. Set the `EVEY_DOMAIN` env var.

```caddyfile
{$EVEY_DOMAIN:localhost} {
    encode gzip

    header {
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy strict-origin-when-cross-origin
        X-XSS-Protection "1; mode=block"
        Content-Security-Policy "default-src 'self'; img-src 'self' https://image.tmdb.org data:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; worker-src 'self'"
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }

    # Streaming endpoints need no timeout
    @streaming {
        path /api/v1/media/*/transcode /api/v1/media/*/hls/* /api/v1/media/*/stream /api/v1/libraries/*/scan
    }
    handle @streaming {
        reverse_proxy evey-prod:9091 {
            transport http {
                read_timeout 0
                write_timeout 0
            }
            flush_interval -1
        }
    }

    handle /api/* {
        reverse_proxy evey-prod:9091 {
            flush_interval -1
        }
    }

    handle {
        root * /srv
        try_files {path} /index.html
        file_server
    }
}
```

Key points:
- `flush_interval -1` disables response buffering (required for SSE/streaming)
- Streaming paths (`transcode`, `hls`, `stream`, `scan`) have `read_timeout 0` and `write_timeout 0` for long-running operations
- Static files are served from `/srv` with SPA fallback

## Nginx Proxy Manager

Create a **Proxy Host** with the following settings:

### Details tab

| Field | Value |
|---|---|
| Domain Names | `media.example.com` |
| Scheme | `http` |
| Forward Hostname / IP | `evey-prod` (Docker service name) or container IP |
| Forward Port | `9091` |
| Websockets Support | Enabled |
| Block Common Exploits | Enabled |

### SSL tab

Request a Let's Encrypt certificate or provide your own.

### Advanced tab

Add custom Nginx config for streaming support:

```nginx
# Disable buffering for streaming endpoints
location ~ ^/api/v1/media/.+/(transcode|hls|stream) {
    proxy_pass http://evey-prod:9091;
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
    proxy_pass http://evey-prod:9091;
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

### Evey env vars for NPM

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12
EVEY_REAL_IP_HEADER=X-Real-IP
EVEY_FORWARDED_PROTO=true
```

## Docker Networking

When running with Docker Compose, Evey and the reverse proxy must share a network:

```yaml
services:
  evey-prod:
    # ...
    networks:
      - proxy

  caddy:  # or nginx-proxy-manager
    # ...
    networks:
      - proxy

networks:
  proxy:
    driver: bridge
```

The reverse proxy references Evey by Docker service name (`evey-prod`). If using an external proxy, use the container IP or expose port 9091 on the host.

Port 9090 (gRPC) does not need to be exposed externally -- it is used for plugin IPC within the container.
