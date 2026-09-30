---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
reviewed: true
---
# Documentation de l'API d'Evey

Les spécifications OpenAPI (Swagger 2.0) du serveur multimédia Evey.

## Organisation

| Partie | Contenu |
|---------|-------------|
| [API publique](public/README.md) | Ce que voient les utilisateurs : navigation dans les médias, lecture, collections, playlists, etc. |
| [API d'administration](admin/README.md) | L'administration du serveur : configuration, transcodage, sauvegardes, métadonnées, etc. |
| [API des plugins](plugins/README.md) | La gestion des plugins et les intégrations par webhook. |

## Utilisation

Chaque dossier contient des fichiers `.swagger.json` à importer dans n'importe
quel outil compatible OpenAPI (Swagger UI, Postman, Insomnia, etc.).

## Régénération

Ces spécifications sont générées à partir des définitions protobuf avec
`buf generate`. Pour les mettre à jour :

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
