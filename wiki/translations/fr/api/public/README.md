---
source_sha: d5c0534aa9aa7e7c333f76023148a199fc8b6bf2f16cee4f6c80face79daf6b2
reviewed: true
---
# API publique

Les points d'accès que tout utilisateur connecté peut appeler : ceux que les
applications utilisent pour parcourir une bibliothèque, lire un titre, gérer
des favoris et des playlists, et demander un média. Un fichier par service.

| Spécification | Contenu |
|------|-------------|
| [library.swagger.json](library.swagger.json) | Bibliothèques, médias, recherche, personnes ; la projection `public` que l'interface d'un plugin peut lire |
| [playback.swagger.json](playback.swagger.json) | Streaming, suivi de progression, historique de lecture, reprise |
| [user.swagger.json](user.swagger.json) | Authentification, profils, clés API, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | Collections intelligentes et manuelles |
| [favorite.swagger.json](favorite.swagger.json) | Favoris |
| [feed.swagger.json](feed.swagger.json) | Sections de l'accueil (reprendre la lecture, ajouts récents...) |
| [playlist.swagger.json](playlist.swagger.json) | Playlists, faites à la main ou par règles |
| [rating.swagger.json](rating.swagger.json) | Notes des utilisateurs |
| [share.swagger.json](share.swagger.json) | Liens de partage publics |
| [syncplay.swagger.json](syncplay.swagger.json) | Salons de lecture synchronisée |
| [notification.swagger.json](notification.swagger.json) | Notifications dans l'application |
| [push.swagger.json](push.swagger.json) | Abonnements aux notifications push du navigateur |
| [music.swagger.json](music.swagger.json) | Bibliothèque musicale (artistes, albums, titres) |
| [photo.swagger.json](photo.swagger.json) | Bibliothèque de photos (albums, photos) |
| [media_request.swagger.json](media_request.swagger.json) | Demandes de médias : recherche, envoi, règles ; les réponses de l'administrateur et l'accès aux demandes |

## Authentification

Les points d'accès prennent `Authorization: Bearer <jeton>` : un jeton de
session obtenu à la connexion, ou une clé d'API dans la limite de ses
permissions ([API du serveur](../README.md#authentication)). Accessibles
sans : la connexion et le renouvellement de session, l'inscription par
invitation, Quick Connect, la réinitialisation du mot de passe, l'assistant
d'installation et son état, et les liens de téléchargement publics.

<!-- sources
auth: internal/adapters/driving/grpcapi/interceptors/auth.go ; public methods: internal/adapters/driving/grpcapi/interceptors/rbac.go
public projection: proto/evey/v1/library.proto - GetPublicMediaItem, ListPublicMediaItems
-->
