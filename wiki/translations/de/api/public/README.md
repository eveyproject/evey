---
source_sha: 4609c608404f7d7614f31c97a0e6a7ce865e34c4d562caf6ca04af7ced28096a
---
# Öffentliche API

Endpunkte für Benutzer, verfügbar für alle angemeldeten Benutzer.

## Spezifikationen

| Spezifikation | Beschreibung |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotheken, Medieneinträge, Suche, Personen |
| [playback.swagger.json](playback.swagger.json) | Streaming, Fortschritt, Wiedergabeverlauf, Fortsetzen |
| [user.swagger.json](user.swagger.json) | Anmeldung, Profile, API-Schlüssel, Schnellverbindung |
| [collection.swagger.json](collection.swagger.json) | Intelligente und manuelle Sammlungen |
| [favorite.swagger.json](favorite.swagger.json) | Favoriten |
| [feed.swagger.json](feed.swagger.json) | Bereiche der Startseite (weiterschauen, neu hinzugefügt usw.) |
| [filter.swagger.json](filter.swagger.json) | Gespeicherte Filter |
| [playlist.swagger.json](playlist.swagger.json) | Playlists (anlegen, umordnen, teilen) |
| [rating.swagger.json](rating.swagger.json) | Bewertungen der Benutzer |
| [share.swagger.json](share.swagger.json) | Öffentliche Freigabelinks |
| [syncplay.swagger.json](syncplay.swagger.json) | Räume für synchrone Wiedergabe |
| [notification.swagger.json](notification.swagger.json) | Benachrichtigungen in der App |
| [push.swagger.json](push.swagger.json) | Web-Push-Abonnements |
| [music.swagger.json](music.swagger.json) | Musikbibliothek (Künstler, Alben, Titel) |
| [photo.swagger.json](photo.swagger.json) | Fotobibliothek (Alben, Fotos) |

## Authentifizierung

Alle Endpunkte verlangen ein gültiges JWT über `Authorization: Bearer <token>`, außer `/api/v1/auth/login` und `/api/v1/setup/*`.
