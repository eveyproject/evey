---
source_sha: 8392e2d20e3a5b3c62e2f39d4743e925c172cdcb73dc639a5502e1bf4c16a66d
reviewed: false
---
# Admin-API

Verwaltungsendpunkte, beschränkt auf Benutzer mit Administratorrechten.

## Spezifikationen

### System
| Spezifikation | Beschreibung |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Dashboard, Systeminformationen, Einrichtungsassistent |
| [server_config.swagger.json](server_config.swagger.json) | Schlüssel-Wert-Speicher der Serverkonfiguration |
| [remote_access.swagger.json](remote_access.swagger.json) | Fernzugriff und Netzwerkeinstellungen |
| [backup.swagger.json](backup.swagger.json) | Sicherung und Wiederherstellung der Datenbank |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | Geplante Sicherungen: Cron, Aufbewahrung, sofort ausführen |
| [activity.swagger.json](activity.swagger.json) | Wiedergabeverlauf, Anmeldeverlauf, Servermetriken |
| [admin_session.swagger.json](admin_session.swagger.json) | Audit-Log |
| [admin_task.swagger.json](admin_task.swagger.json) | Geplante Hintergrundaufgaben |
| [presence.swagger.json](presence.swagger.json) | Wer gerade verbunden ist (das Signal der Apps und die Liste der Verwaltung) |

### Medien und Transkodierung
| Spezifikation | Beschreibung |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Medien löschen, Metadaten aktualisieren, Trickplay erzeugen |
| [metadata.swagger.json](metadata.swagger.json) | Metadatenanbieter, Suche, Übersteuerungen |
| [transcoding.swagger.json](transcoding.swagger.json) | Transkodierungsprofile, Hardwareerkennung, laufende Aufträge |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Verwaltung des Transkodierungs-Caches |
| [mkv.swagger.json](mkv.swagger.json) | Werkzeuge für den MKV-Container (Spuren, Kapitel, Muxen/Demuxen) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | Cache der Intro-Zeiten: eine Folge ungültig machen, nachbefüllen |

### Benutzer und Richtlinien
| Spezifikation | Beschreibung |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Bibliothekszugriff pro Benutzer |
| [parental.swagger.json](parental.swagger.json) | Kindersicherung und PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Globale Wiedergabeeinstellungen |

## Authentifizierung

Jeder Endpunkt verlangt einen Administrator: das Sitzungstoken eines Administratorkontos oder einen API-Schlüssel, der immer einem gehört (seine Scopes können ihn einschränken).
