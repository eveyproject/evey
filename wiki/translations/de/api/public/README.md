---
source_sha: e1e56c5e8b4af228a44f567d290622a0165ed4a03deb071e09411371605750d5
reviewed: false
---
# Öffentliche API

Endpunkte für Benutzer, verfügbar für alle angemeldeten Benutzer.

## Spezifikationen

| Spezifikation | Beschreibung |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotheken, Medien, Suche, Personen |
| [playback.swagger.json](playback.swagger.json) | Streaming, Fortschritt, Verlauf, Fortsetzen |
| [user.swagger.json](user.swagger.json) | Authentifizierung, Profile, API-Schlüssel, Schnellverbindung |
| [collection.swagger.json](collection.swagger.json) | Intelligente und manuelle Sammlungen |
| [favorite.swagger.json](favorite.swagger.json) | Favoriten |
| [feed.swagger.json](feed.swagger.json) | Abschnitte der Startseite (Weiterschauen, kürzlich hinzugefügt usw.) |
| [playlist.swagger.json](playlist.swagger.json) | Playlists (anlegen, umsortieren, teilen) |
| [rating.swagger.json](rating.swagger.json) | Bewertungen der Benutzer |
| [share.swagger.json](share.swagger.json) | Öffentliche Freigabelinks |
| [syncplay.swagger.json](syncplay.swagger.json) | Räume für synchrone Wiedergabe |
| [notification.swagger.json](notification.swagger.json) | Benachrichtigungen in der App |
| [push.swagger.json](push.swagger.json) | Web-Push-Abonnements |
| [music.swagger.json](music.swagger.json) | Musikbibliothek (Künstler, Alben, Titel) |
| [photo.swagger.json](photo.swagger.json) | Fotobibliothek (Alben, Fotos) |
| [media_request.swagger.json](media_request.swagger.json) | Medienanfragen: Suche, Einreichen, Regeln; die Antworten des Administrators und der Zugang zu Anfragen |

## Authentifizierung

Die Endpunkte erwarten `Authorization: Bearer <token>`: ein Sitzungstoken aus der Anmeldung oder einen API-Schlüssel (siehe [Webhooks und API-Schlüssel](../../webhooks-api-keys.md)). Ohne erreichbar: Anmelden und Erneuern einer Sitzung, Registrieren mit einer Einladung, Quick Connect, Zurücksetzen des Passworts, der Einrichtungsassistent und sein Status sowie öffentliche Download-Links.
