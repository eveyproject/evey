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
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | Scheduled backups: cron, retention, run now |
| [activity.swagger.json](activity.swagger.json) | Play history, login history, server metrics |
| [admin_session.swagger.json](admin_session.swagger.json) | Audit log |
| [admin_task.swagger.json](admin_task.swagger.json) | Scheduled background tasks |
| [presence.swagger.json](presence.swagger.json) | Who is connected right now (the apps' heartbeat and the admin list) |

### Media & Transcoding
| Spec | Description |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Delete media items, refresh metadata, generate trickplay |
| [metadata.swagger.json](metadata.swagger.json) | Metadata providers, search, overrides |
| [transcoding.swagger.json](transcoding.swagger.json) | Transcode profiles, hardware detection, active jobs |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Transcode cache management |
| [mkv.swagger.json](mkv.swagger.json) | MKV container tools (tracks, chapters, mux/demux) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | Intro timing cache: invalidate one episode, backfill |

### Users & Policies
| Spec | Description |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Per-user library access control |
| [parental.swagger.json](parental.swagger.json) | Parental controls and PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Global playback settings |

## Authentication

Every endpoint needs an administrator: the session token of an administrator
account, or an API key, which always belongs to one (its scopes can narrow it).
