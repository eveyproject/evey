---
source_sha: 4609c608404f7d7614f31c97a0e6a7ce865e34c4d562caf6ca04af7ced28096a
reviewed: true
---
# API publique

Les points d'accès ouverts à tous les utilisateurs authentifiés.

## Spécifications

| Spécification | Contenu |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliothèques, médias, recherche, personnes |
| [playback.swagger.json](playback.swagger.json) | Streaming, suivi de progression, historique de lecture, reprise |
| [user.swagger.json](user.swagger.json) | Authentification, profils, clés API, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Collections intelligentes et manuelles |
| [favorite.swagger.json](favorite.swagger.json) | Favoris |
| [feed.swagger.json](feed.swagger.json) | Sections de l'accueil (reprendre la lecture, ajouts récents, etc.) |
| [filter.swagger.json](filter.swagger.json) | Filtres enregistrés |
| [playlist.swagger.json](playlist.swagger.json) | Playlists (création, réorganisation, partage) |
| [rating.swagger.json](rating.swagger.json) | Notes des utilisateurs |
| [share.swagger.json](share.swagger.json) | Liens de partage publics |
| [syncplay.swagger.json](syncplay.swagger.json) | Salons de lecture synchronisée |
| [notification.swagger.json](notification.swagger.json) | Notifications dans l'application |
| [push.swagger.json](push.swagger.json) | Abonnements aux notifications push du navigateur |
| [music.swagger.json](music.swagger.json) | Bibliothèque musicale (artistes, albums, titres) |
| [photo.swagger.json](photo.swagger.json) | Bibliothèque de photos (albums, photos) |

## Authentification

Tous les points d'accès demandent un jeton JWT valide dans
`Authorization: Bearer <jeton>`, sauf `/api/v1/auth/login` et
`/api/v1/setup/*`.
