# Evey

Self-hosted media server with gRPC + REST API, plugin system, and hardware-accelerated transcoding.

## Architecture

Hexagonal (ports & adapters):

```
cmd/evey/
  main.go               — wiring

internal/
  domain/               — business models + port interfaces
  app/                  — use-case services (pure Go, no framework)
  adapters/
    driven/             — repos (postgres), metadata (TMDB), filesystem, ffmpeg, ebml, meili
    driving/            — gRPC handlers, plugin host API
  config/               — env-based config

gen/evey/v1/            — generated gRPC + grpc-gateway stubs
gen/plugin/v1/          — generated plugin API stubs
proto/                  — .proto sources
plugins/                — example plugins (Python, Node.js)
```

## Quick Start

### Prerequisites

- Go 1.22+
- PostgreSQL 15+
- ffmpeg (for transcoding)
- (optional) Meilisearch (for search)

### Run

```bash
# Copy and edit environment
cp .env.example .env

# Apply migrations
psql $EVEY_DB_DSN -f internal/adapters/driven/postgres/migrations/000001_init.up.sql
psql $EVEY_DB_DSN -f internal/adapters/driven/postgres/migrations/000002_transcode_profiles.up.sql
psql $EVEY_DB_DSN -f internal/adapters/driven/postgres/migrations/000003_profile_languages.up.sql

go run ./cmd/evey
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `EVEY_SERVER_PORT` | `8080` | HTTP + gRPC-gateway port |
| `EVEY_DB_HOST` | `localhost` | PostgreSQL host |
| `EVEY_DB_PORT` | `5432` | PostgreSQL port |
| `EVEY_DB_USER` | `evey` | PostgreSQL user |
| `EVEY_DB_PASSWORD` | `evey` | PostgreSQL password |
| `EVEY_DB_NAME` | `evey` | PostgreSQL database |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `jwt.key` | Path to RSA private key (PEM) |
| `EVEY_TMDB_API_KEY` | — | TMDB API key (Bearer token) |
| `EVEY_METADATA_LANGUAGE` | `en` | ISO 639-1 language for TMDB metadata |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Max parallel transcode jobs |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | Enable HW acceleration |
| `EVEY_MEILI_URL` | `http://localhost:7700` | Meilisearch URL |
| `EVEY_OIDC_ENABLED` | `false` | Enable OIDC SSO |

## API

The server exposes gRPC on port `9090` and REST (grpc-gateway) on port `8080`.

All REST endpoints are under `/api/v1/`. Authentication uses Bearer JWT tokens.

### Services

| Service | gRPC package | REST prefix |
|---|---|---|
| AdminService | `evey.v1.AdminService` | `/api/v1/admin/` |
| UserService | `evey.v1.UserService` | `/api/v1/users/` |
| LibraryService | `evey.v1.LibraryService` | `/api/v1/libraries/` |
| PlaybackService | `evey.v1.PlaybackService` | `/api/v1/playback/` |
| TranscodingService | `evey.v1.TranscodingService` | `/api/v1/transcoding/` |
| MKVService | `evey.v1.MKVService` | `/api/v1/mkv/` |
| MetadataService | `evey.v1.MetadataService` | `/api/v1/metadata/` |
| ActivityService | `evey.v1.ActivityService` | `/api/v1/activity/` |

### Authentication

```http
POST /api/v1/users/login
Content-Type: application/json

{"username": "admin", "password": "secret"}
```

Response: `{"access_token": "...", "refresh_token": "..."}`

Use `Authorization: Bearer <access_token>` on all subsequent requests.

### Setup (first run)

```http
POST /api/v1/admin/setup
Content-Type: application/json

{"admin_username": "admin", "admin_password": "secret", "admin_email": "admin@example.com"}
```

### Library

```http
# Create library
POST /api/v1/libraries
{"name": "Movies", "type": "LIBRARY_TYPE_MOVIES", "paths": ["/media/movies"]}

# List libraries
GET /api/v1/libraries

# Scan library
POST /api/v1/libraries/{id}/scan

# List media items
GET /api/v1/libraries/{library_id}/items?type=movie&limit=50&offset=0
```

### Playback

```http
# Start session
POST /api/v1/playback/sessions
{"profile_id": "...", "media_item_id": "...", "device_name": "Browser", "play_method": "PLAY_METHOD_DIRECT_PLAY"}

# Update progress
PUT /api/v1/playback/sessions/{id}/progress
{"position_ms": 120000}

# Stop session
DELETE /api/v1/playback/sessions/{id}
```

### Metadata

```http
# Search metadata
GET /api/v1/metadata/search?query=Inception&type=movie

# Refresh metadata for a media item
POST /api/v1/metadata/items/{media_item_id}/refresh
{"provider_id": "27205", "provider_name": "tmdb", "media_type": "movie"}

# Get artwork
GET /api/v1/metadata/items/{media_item_id}/artwork

# List configured providers
GET /api/v1/metadata/providers
```

### Transcoding

```http
# Start transcode job
POST /api/v1/transcoding/jobs
{"media_item_id": "...", "profile_id": "..."}

# Check job status
GET /api/v1/transcoding/jobs/{id}

# List transcode profiles
GET /api/v1/transcoding/profiles
```

### Users & Profiles

```http
# Create user (admin only)
POST /api/v1/users
{"username": "bob", "password": "pass", "email": "bob@example.com"}

# Create profile
POST /api/v1/users/{user_id}/profiles
{"name": "Kids", "max_rating": "PG"}

# Update profile language preferences
PUT /api/v1/users/{user_id}/profiles/{profile_id}
{"preferences": {"audio_languages": "fr,en", "subtitle_languages": "fr"}}
```

## i18n / Localization

### Server-level metadata language

Set `EVEY_METADATA_LANGUAGE=fr` to fetch all TMDB metadata (titles, overviews) in French.

TMDB returns localized titles and overviews when the language is supported. The server stores whatever TMDB returns in `media_items.title` and `media_items.overview`.

### Per-profile audio/subtitle preferences

Each profile has ordered fallback lists:
- `audio_languages` — e.g. `["fr", "en"]` plays French audio, falls back to English
- `subtitle_languages` — e.g. `["fr"]` shows French subtitles when available

These are stored as `TEXT[]` in PostgreSQL and serialized into the `preferences` map in the API response as comma-separated strings (`"audio_languages": "fr,en"`).

## Plugin System

Plugins extend Evey via gRPC over Unix sockets.

### How it works

1. Evey starts a `HostService` gRPC server on `/tmp/evey/plugins/evey-host.sock`
2. Each plugin process is launched with environment variables:
   - `EVEY_PLUGIN_ID` — unique plugin ID
   - `EVEY_PLUGIN_SOCK` — Unix socket path where the plugin must listen (PluginService)
   - `EVEY_HOST_SOCK` — host service socket path
3. The plugin listens on `EVEY_PLUGIN_SOCK` and serves `PluginService` (host→plugin calls)
4. The plugin connects to `EVEY_HOST_SOCK` and calls `HostService` (plugin→host calls), always sending `x-plugin-id` in gRPC metadata

### Plugin manifest (`plugin.yaml`)

```yaml
name: my-plugin
version: 1.0.0
description: My plugin description
author: Author Name
language: python          # python | nodejs | go | ...
entrypoint: python main.py
min_evey_version: 0.1.0
capabilities:
  - media.read            # can read library items
  - metadata.provider     # can provide metadata
  - notification.sender   # can emit events
permissions:
  - media.read
  - api.library
  - api.metadata
config_schema:
  api_key:
    type: string
    required: false
```

**Known capabilities:** `metadata.provider`, `media.read`, `media.write`, `webhook.receiver`, `notification.sender`, `ui.extension`

**Known permissions:** `network`, `filesystem`, `media.read`, `media.write`, `api.library`, `api.user`, `api.metadata`

### Installing a plugin

```http
POST /api/v1/admin/plugins/install
{"source": "/path/to/plugin.zip"}
```

The zip must contain `plugin.yaml` at the root.

### Example plugins

- [`plugins/python-example/`](plugins/python-example/) — Python plugin
- [`plugins/nodejs-example/`](plugins/nodejs-example/) — Node.js plugin

See each directory for setup instructions.

## Development

### Running tests

```bash
# Unit tests (fast, no external deps)
go test ./internal/...

# Integration tests (requires Docker for testcontainers)
go test ./internal/adapters/driven/postgres/... -timeout 180s
```

### Regenerating protos

```bash
buf generate
```

### Code structure conventions

- Domain models in `internal/domain/<name>/model.go`
- Port interfaces in `internal/domain/<name>/ports.go`
- In-memory fakes for testing in `internal/domain/<name>/fakes/`
- PostgreSQL adapters in `internal/adapters/driven/postgres/`
- gRPC handlers in `internal/adapters/driving/grpcapi/`
- All converter functions (domain ↔ proto) in `internal/adapters/driving/grpcapi/converters.go`
