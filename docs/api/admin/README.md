# Admin API

Administrative endpoints restricted to users with admin privileges.

## Specs

### System
| Spec | Description |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Dashboard, system info, setup wizard |
| [server_config.swagger.json](server_config.swagger.json) | Server configuration key/value store |
| [remote_access.swagger.json](remote_access.swagger.json) | Remote access and network settings |
| [backup.swagger.json](backup.swagger.json) | Database backup and restore |
| [activity.swagger.json](activity.swagger.json) | Play history, login history, server metrics |
| [admin_session.swagger.json](admin_session.swagger.json) | Audit log |
| [admin_task.swagger.json](admin_task.swagger.json) | Scheduled background tasks |

### Media & Transcoding
| Spec | Description |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Delete media items, refresh metadata, generate trickplay |
| [metadata.swagger.json](metadata.swagger.json) | Metadata providers, search, overrides |
| [transcoding.swagger.json](transcoding.swagger.json) | Transcode profiles, hardware detection, active jobs |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Transcode cache management |
| [mkv.swagger.json](mkv.swagger.json) | MKV container tools (tracks, chapters, mux/demux) |

### Users & Policies
| Spec | Description |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Per-user library access control |
| [parental.swagger.json](parental.swagger.json) | Parental controls and PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Global playback settings |

## Authentication

All endpoints require a valid admin JWT token.
