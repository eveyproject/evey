---
source_sha: 4609c608404f7d7614f31c97a0e6a7ce865e34c4d562caf6ca04af7ced28096a
---

# API publique

Points d'accès destinés aux utilisateurs, ouverts à tout compte authentifié.

## Spécifications

| Spécification | Description |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliothèques, éléments multimédias, recherche, personnes |
| [playback.swagger.json](playback.swagger.json) | Diffusion, suivi de la progression, historique de visionnage, reprise |
| [user.swagger.json](user.swagger.json) | Authentification, profils, clés d'API, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Collections intelligentes et manuelles |
| [favorite.swagger.json](favorite.swagger.json) | Éléments favoris |
| [feed.swagger.json](feed.swagger.json) | Sections de l'accueil (reprendre la lecture, ajouts récents, etc.) |
| [filter.swagger.json](filter.swagger.json) | Filtres enregistrés |
| [playlist.swagger.json](playlist.swagger.json) | Playlists (création, réordonnancement, partage) |
| [rating.swagger.json](rating.swagger.json) | Notes des utilisateurs |
| [share.swagger.json](share.swagger.json) | Liens de partage publics |
| [syncplay.swagger.json](syncplay.swagger.json) | Salons de lecture synchronisée |
| [notification.swagger.json](notification.swagger.json) | Notifications dans l'application |
| [push.swagger.json](push.swagger.json) | Abonnements aux notifications push web |
| [music.swagger.json](music.swagger.json) | Bibliothèque musicale (artistes, albums, pistes) |
| [photo.swagger.json](photo.swagger.json) | Bibliothèque photo (albums, photos) |

## Authentification

Tous les points d'accès exigent un jeton JWT valide via `Authorization: Bearer <token>`, sauf `/api/v1/auth/login` et `/api/v1/setup/*`.
