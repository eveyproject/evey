---
source_sha: 5d6e1c496b373ae579e3de32d6359d6c1c9712b93ef8c17806d17afca066d8f8
reviewed: true
---
# API du serveur

L'API HTTP du serveur, décrite en fichiers OpenAPI (Swagger 2.0), un par
service. Cet index dit quel dossier contient quoi et comment un appel
s'authentifie ; les fichiers eux-mêmes sont la référence.

| Partie | Contenu |
|---------|-------------|
| [API publique](public/README.md) | Ce qu'un utilisateur connecté peut faire : parcourir, lire, collectionner, demander. |
| [API d'administration](admin/README.md) | L'administration du serveur : configuration, transcodage, sauvegardes, métadonnées, utilisateurs. |
| [API des plugins](plugins/README.md) | La gestion des plugins, les webhooks, et les préférences que l'interface d'un plugin garde par utilisateur. |

Chaque fichier `.swagger.json` s'importe dans n'importe quel outil OpenAPI
(Swagger UI, Postman, Insomnia). Les routes HTTP sont générées à partir des
définitions gRPC de `proto/evey/v1/` : un nom de méthode dans un fichier est
la méthode gRPC qui se trouve derrière.

## Authentification

Trois sortes d'identifiants atteignent l'API, tous dans l'en-tête
`Authorization` :

| Identifiant | Qui | Comment |
|---|---|---|
| Jeton de session | Une personne connectée par une application | `Bearer <jeton>`, obtenu à la connexion et renouvelé par l'application. |
| Clé d'API | Un script, au nom de l'administrateur qui l'a créée | `Bearer evey_...` (le préfixe `Bearer` est facultatif). À permissions limitées, révocable : [Webhooks et clés d'API](../webhooks-api-keys.md). |
| Jeton de plugin | L'interface d'un plugin, par la page hôte | Émis pour cinq minutes, pour un plugin et un utilisateur, ne portant que les portées `public` et `prefs` accordées au plugin ; toute autre méthode le refuse. Le cadre ne le détient jamais : la page hôte relaie les appels. |

Un jeton de plugin ne peut jamais atteindre une méthode d'administration,
quelles que soient ses portées
([ADR-0003](../adr/0003-plugin-scoped-tokens.md),
[ADR-0012](../adr/0012-plugin-token-deny-by-default.md)).

## Régénération

Pour les contributeurs : les fichiers sont régénérés à partir des définitions
protobuf.

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```

<!-- sources
session token, api key, plugin token: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticate, authenticateAPIKey, authorizePluginToken
plugin token: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
generation: docs/api/sync.py ; buf.gen.local.yaml
-->
