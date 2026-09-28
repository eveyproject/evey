---
source_sha: e314c511a93178cdc35b47b8bcd886cb60fd2ea303b7fc7d6f6c3c9ca1f9b6d0
---
# Administrations-API

Verwaltungsendpunkte, beschränkt auf Benutzer mit Administratorrechten.

## Spezifikationen

### System
| Spezifikation | Beschreibung |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Dashboard, Systeminformationen, Einrichtungsassistent |
| [server_config.swagger.json](server_config.swagger.json) | Schlüssel-Wert-Speicher der Serverkonfiguration |
| [remote_access.swagger.json](remote_access.swagger.json) | Fernzugriff und Netzwerkeinstellungen |
| [backup.swagger.json](backup.swagger.json) | Sicherung und Wiederherstellung der Datenbank |
| [activity.swagger.json](activity.swagger.json) | Wiedergabeverlauf, Anmeldeverlauf, Servermetriken |
| [admin_session.swagger.json](admin_session.swagger.json) | Audit-Log |
| [admin_task.swagger.json](admin_task.swagger.json) | Geplante Hintergrundaufgaben |

### Medien und Transkodierung
| Spezifikation | Beschreibung |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Medieneinträge löschen, Metadaten aktualisieren, Trickplay erzeugen |
| [metadata.swagger.json](metadata.swagger.json) | Metadatenanbieter, Suche, Überschreibungen |
| [transcoding.swagger.json](transcoding.swagger.json) | Transkodierprofile, Hardwareerkennung, laufende Aufträge |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Verwaltung des Transkodierungs-Caches |
| [mkv.swagger.json](mkv.swagger.json) | Werkzeuge für MKV-Container (Spuren, Kapitel, Mux/Demux) |

### Benutzer und Richtlinien
| Spezifikation | Beschreibung |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Bibliothekszugriff pro Benutzer |
| [parental.swagger.json](parental.swagger.json) | Jugendschutz und PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Globale Wiedergabeeinstellungen |

## Authentifizierung

Alle Endpunkte verlangen ein gültiges Administrator-JWT.
