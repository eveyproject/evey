# Evey API Documentation

OpenAPI (Swagger 2.0) specifications for the Evey media server.

## Structure

| Section | Description |
|---------|-------------|
| [Public API](public/README.md) | User-facing endpoints - media browsing, playback, collections, playlists, etc. |
| [Admin API](admin/README.md) | Server administration - system config, transcoding, backups, metadata, etc. |
| [Plugin API](plugins/README.md) | Plugin management and webhook integrations. |

## Usage

Each folder contains individual `.swagger.json` files that can be imported into any OpenAPI-compatible tool (Swagger UI, Postman, Insomnia, etc.).

## Regeneration

These specs are generated from protobuf definitions via `buf generate`. To update:

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
