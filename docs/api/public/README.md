# Public API

User-facing endpoints available to all authenticated users.

## Specs

| Spec | Description |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Libraries, media items, search, people |
| [playback.swagger.json](playback.swagger.json) | Streaming, progress tracking, watch history, resume |
| [user.swagger.json](user.swagger.json) | Authentication, profiles, API keys, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Smart and manual collections |
| [favorite.swagger.json](favorite.swagger.json) | Favorite items |
| [feed.swagger.json](feed.swagger.json) | Home feed sections (continue watching, recently added, etc.) |
| [playlist.swagger.json](playlist.swagger.json) | Playlists (create, reorder, share) |
| [rating.swagger.json](rating.swagger.json) | User ratings |
| [share.swagger.json](share.swagger.json) | Public share links |
| [syncplay.swagger.json](syncplay.swagger.json) | Synchronized playback rooms |
| [notification.swagger.json](notification.swagger.json) | In-app notifications |
| [push.swagger.json](push.swagger.json) | Web push subscriptions |
| [music.swagger.json](music.swagger.json) | Music library (artists, albums, tracks) |
| [photo.swagger.json](photo.swagger.json) | Photo library (albums, photos) |
| [media_request.swagger.json](media_request.swagger.json) | Media requests: search, submit, policy; the administrator's answers and request access |

## Authentication

All endpoints require a valid JWT token via `Authorization: Bearer <token>`, except `/api/v1/auth/login` and `/api/v1/setup/*`.
