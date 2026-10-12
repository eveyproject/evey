---
source_sha: d5c0534aa9aa7e7c333f76023148a199fc8b6bf2f16cee4f6c80face79daf6b2
reviewed: true
---
# Öffentliche API

Die Endpunkte, die jeder angemeldete Benutzer aufrufen kann: die, mit denen
die Apps eine Bibliothek durchstöbern, einen Titel abspielen, Favoriten und
Playlists führen und ein Medium anfragen. Eine Datei pro Dienst.

| Spezifikation | Beschreibung |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliotheken, Medien, Suche, Personen; die Projektion `public`, die die Oberfläche eines Plugins lesen darf |
| [playback.swagger.json](playback.swagger.json) | Streaming, Fortschritt, Wiedergabeverlauf, Fortsetzen |
| [user.swagger.json](user.swagger.json) | Authentifizierung, Profile, API-Schlüssel, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Intelligente und manuelle Sammlungen |
| [favorite.swagger.json](favorite.swagger.json) | Favoriten |
| [feed.swagger.json](feed.swagger.json) | Bereiche der Startseite (Weiterschauen, kürzlich hinzugefügt...) |
| [playlist.swagger.json](playlist.swagger.json) | Playlists, von Hand oder nach Regeln erstellt |
| [rating.swagger.json](rating.swagger.json) | Bewertungen der Benutzer |
| [share.swagger.json](share.swagger.json) | Öffentliche Freigabelinks |
| [syncplay.swagger.json](syncplay.swagger.json) | Räume für synchrone Wiedergabe |
| [notification.swagger.json](notification.swagger.json) | Benachrichtigungen in der App |
| [push.swagger.json](push.swagger.json) | Web-Push-Abonnements |
| [music.swagger.json](music.swagger.json) | Musikbibliothek (Künstler, Alben, Titel) |
| [photo.swagger.json](photo.swagger.json) | Fotobibliothek (Alben, Fotos) |
| [media_request.swagger.json](media_request.swagger.json) | Medienanfragen: Suche, Einreichen, Regeln; die Antworten des Administrators und der Zugang zu Anfragen |

## Authentifizierung

Die Endpunkte erwarten `Authorization: Bearer <token>`: einen Sitzungstoken
aus der Anmeldung oder einen API-Schlüssel im Rahmen seiner Bereiche
([Server-API](../README.md#authentication)). Ohne Token offen: Anmelden und
eine Sitzung erneuern, Registrierung mit einer Einladung, Quick Connect,
Passwort zurücksetzen, der Einrichtungsassistent und sein Status sowie
öffentliche Download-Links.

<!-- sources
auth: internal/adapters/driving/grpcapi/interceptors/auth.go ; public methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - PublicMethods
public projection: proto/evey/v1/library.proto - GetPublicMediaItem, ListPublicMediaItems
-->
