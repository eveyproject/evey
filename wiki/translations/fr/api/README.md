---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
---

# Documentation de l'API Evey

Spécifications OpenAPI (Swagger 2.0) du serveur multimédia Evey.

## Structure

| Section | Description |
|---------|-------------|
| [API publique](public/README.md) | Points d'accès destinés aux utilisateurs : navigation dans les médias, lecture, collections, playlists, etc. |
| [API d'administration](admin/README.md) | Administration du serveur : configuration système, transcodage, sauvegardes, métadonnées, etc. |
| [API des extensions](plugins/README.md) | Gestion des extensions et intégrations par webhook. |

## Utilisation

Chaque dossier contient des fichiers `.swagger.json` individuels, importables dans n'importe quel outil compatible OpenAPI (Swagger UI, Postman, Insomnia, etc.).

## Régénération

Ces spécifications sont générées à partir des définitions protobuf par `buf generate`. Pour les mettre à jour :

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
