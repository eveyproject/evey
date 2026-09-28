---
source_sha: 69eb69dc75f4051d291795ad196ce5ee4b61038c94cbe8a97d9b7ee7d7470a69
---
# Sicherungen

Evey kann Ihre Datenbank nach einem festen Zeitplan automatisch sichern.

## Konfiguration

```env title=".env"
EVEY_BACKUP_DIR=/data/backups          # Ablageort der Sicherungen
EVEY_BACKUP_INTERVAL_HOURS=24          # Abstand der automatischen Sicherungen (0 = deaktiviert)
EVEY_BACKUP_RETENTION_DAYS=30          # Ältere Sicherungen werden gelöscht (0 = unbegrenzt aufbewahren)
```

## Was gesichert wird

Die Sicherung enthält sämtliche Benutzerdaten:
- Benutzer, Profile, Rollen, Berechtigungen
- Bibliotheken, Sammlungen, Wiedergabelisten, Favoriten
- Wiedergabestatus, Wiedergabeverlauf
- Erweiterungen, Serverkonfiguration, geplante Aufgaben
- Benachrichtigungen, Webhooks, Auditprotokolle

**Nicht gesichert werden**: Ihre Mediendateien (die gehören Ihnen) und der Transkodierungs-Cache (der wird automatisch neu aufgebaut).

## Aufbau einer Sicherung

Jede Sicherung ist ein `.tar.gz`-Archiv und enthält:
- `manifest.json` - Metadaten, Version, SHA256-Prüfsumme
- `dump.sql` - vollständiger PostgreSQL-Dump

## Sicherung von Hand auslösen

Sie können eine Sicherung im Administrationsbereich oder über die API starten:

```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:9091/api/v1/admin/backups
```

## Wiederherstellung

Die Wiederherstellung aus einer Sicherung läuft über die Admin-API. Vor dem Einspielen wird die Unversehrtheit der Sicherung anhand der SHA256-Prüfsumme überprüft.

## Dauerhafte Ablage

Achten Sie darauf, dass `EVEY_BACKUP_DIR` auf ein dauerhaftes Volume oder ein Verzeichnis des Hosts zeigt. Der Standardwert `/data/backups` liegt innerhalb des Docker-Volumes `evey-data`.

So legen Sie die Sicherungen auf dem Host ab:
```yaml title="docker-compose.yml"
services:
  evey:
    volumes:
      - /path/on/host/backups:/data/backups
```
