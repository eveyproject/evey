# Backups

Evey can automatically back up your database on a schedule.

## Configuration

```env title=".env"
EVEY_BACKUP_DIR=/data/backups          # Where backups are stored
EVEY_BACKUP_INTERVAL_HOURS=24          # Auto-backup interval (0 = disabled)
EVEY_BACKUP_RETENTION_DAYS=30          # Delete backups older than this (0 = keep forever)
```

## What's Backed Up

The backup includes all user data:
- Users, profiles, roles, permissions
- Libraries, collections, playlists, favorites
- Watch progress, play history
- Plugins, server config, scheduled tasks
- Notifications, webhooks, audit logs

**Not backed up**: media files (those are yours) and transcode cache (regenerated automatically).

## Backup Format

Each backup is a `.tar.gz` containing:
- `manifest.json` - metadata, version, SHA256 integrity hash
- `dump.sql` - full PostgreSQL dump

## Manual Backup

Trigger a backup from the admin panel or via API:

```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:9091/api/v1/admin/backups
```

## Restore

Restoring from backup is available through the admin API. The backup integrity is verified via SHA256 hash before applying.

## Persistent Storage

Make sure `EVEY_BACKUP_DIR` maps to a persistent volume or host path. The default `/data/backups` is inside the `evey-data` Docker volume.

To store backups on the host:
```yaml title="docker-compose.yml"
services:
  evey:
    volumes:
      - /path/on/host/backups:/data/backups
```
