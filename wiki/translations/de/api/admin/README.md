---
source_sha: cc4cecd7aee5e32086fae7de125434b98b804bf28bed8ea336efed0bb38c02bb
reviewed: true
---
# Admin-API

Die Endpunkte, die Administratoren vorbehalten sind: das, was **Verwaltung**
in der Web-App aufruft. Eine Datei pro Dienst, nach Bereich gruppiert.

### System
| Spezifikation | Beschreibung |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Dashboard, Systeminformationen, Einrichtungsassistent |
| [server_config.swagger.json](server_config.swagger.json) | Schlüssel-Wert-Speicher der Serverkonfiguration |
| [remote_access.swagger.json](remote_access.swagger.json) | Fernzugriff und Netzwerkeinstellungen |
| [backup.swagger.json](backup.swagger.json) | Sicherung und Wiederherstellung der Datenbank |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | Geplante Sicherungen: Cron, Aufbewahrung, jetzt ausführen |
| [activity.swagger.json](activity.swagger.json) | Wiedergabeverlauf, Anmeldeverlauf, Servermetriken |
| [admin_session.swagger.json](admin_session.swagger.json) | Prüfprotokoll |
| [admin_task.swagger.json](admin_task.swagger.json) | Geplante Hintergrundaufgaben |
| [presence.swagger.json](presence.swagger.json) | Wer gerade verbunden ist (das Lebenszeichen der Apps und die Liste für die Verwaltung) |

### Medien und Transkodierung
| Spezifikation | Beschreibung |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Medienzustand (Prüfungen, Reparaturen, Verlauf und Rückgängig), Medien löschen, Metadaten aktualisieren, Trickplay erzeugen |
| [metadata.swagger.json](metadata.swagger.json) | Metadatenanbieter, Suche, manuelle Korrekturen |
| [transcoding.swagger.json](transcoding.swagger.json) | Transkodierprofile, Hardwareerkennung, laufende Aufträge |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Verwaltung des Transkodier-Caches |
| [mkv.swagger.json](mkv.swagger.json) | Werkzeuge für den MKV-Container (Spuren, Kapitel, Mux/Demux) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | Cache der Intro-Zeitmarken: eine Folge verwerfen, nachträglich auffüllen |

### Benutzer und Regeln
| Spezifikation | Beschreibung |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Bibliothekszugriff pro Benutzer |
| [parental.swagger.json](parental.swagger.json) | Kindersicherung und PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Globale Wiedergabeeinstellungen |

## Authentifizierung

Jeder Endpunkt verlangt einen Administrator: den Sitzungstoken eines
Administratorkontos oder einen API-Schlüssel. Ein API-Schlüssel gehört immer
einem Administrator und muss den Bereich tragen, den die Methode verlangt
(ein Schlüssel ohne Bereiche erreicht keine Verwaltungsmethode). Ein
Plugin-Token wird hier abgelehnt, egal welche Bereiche er hat
([Server-API](../README.md#authentication)).

<!-- sources
admin methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - MethodRoles ; api key scope on admin methods: interceptors/auth.go - authorize ; plugin token floor: auth.go - authorizePluginToken
-->
