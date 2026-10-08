# Public API

The endpoints every signed-in user can call: the ones the apps use to browse
a library, play a title, keep favorites and playlists, and ask for a media.
One file per service.

| Spec | Description |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Libraries, media items, search, people; the `public` projection a plugin's interface may read |
| [playback.swagger.json](playback.swagger.json) | Streaming, progress tracking, watch history, resume |
| [user.swagger.json](user.swagger.json) | Authentication, profiles, API keys, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Smart and manual collections |
| [favorite.swagger.json](favorite.swagger.json) | Favorite items |
| [feed.swagger.json](feed.swagger.json) | Home feed sections (continue watching, recently added...) |
| [playlist.swagger.json](playlist.swagger.json) | Playlists, made by hand or by rules |
| [rating.swagger.json](rating.swagger.json) | User ratings |
| [share.swagger.json](share.swagger.json) | Public share links |
| [syncplay.swagger.json](syncplay.swagger.json) | Synchronized playback rooms |
| [notification.swagger.json](notification.swagger.json) | In-app notifications |
| [push.swagger.json](push.swagger.json) | Web push subscriptions |
| [music.swagger.json](music.swagger.json) | Music library (artists, albums, tracks) |
| [photo.swagger.json](photo.swagger.json) | Photo library (albums, photos) |
| [media_request.swagger.json](media_request.swagger.json) | Media requests: search, submit, policy; the administrator's answers and request access |

## Authentication

Endpoints take `Authorization: Bearer <token>`: a session token from signing
in, or an API key within its scopes ([Server API](../README.md#authentication)).
Open without one: signing in and renewing a session, signing up with an
invitation, Quick Connect, password reset, the setup wizard and its status,
and public download links.

<!-- sources
auth: internal/adapters/driving/grpcapi/interceptors/auth.go ; public methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - PublicMethods
public projection: proto/evey/v1/library.proto - GetPublicMediaItem, ListPublicMediaItems
-->
