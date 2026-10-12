---
source_sha: a960630483d1b2a05980e34cb7d0eacb371e9de2acf7fcd4af72e9609a09a8cf
reviewed: true
---
# API des plugins

Les points d'accès autour des plugins : les installer et les gérer
(administrateur), les webhooks (administrateur), et les préférences que
l'interface d'un plugin garde par utilisateur. Écrire un plugin relève d'un
autre contrat, en gRPC sur un socket local : voir
[Écrire un plugin](../../plugins/dev-setup.md) et la
[Référence de l'API des plugins](../../plugins/api-reference.md).

| Spécification | Contenu |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Installer, activer, désactiver, désinstaller des plugins ; configuration, journaux, limites de ressources, environnements d'exécution ; aperçu et application d'une mise à jour ; `IssuePluginToken` |
| [webhook.swagger.json](webhook.swagger.json) | Créer, lister, modifier et supprimer des webhooks ; historique des envois |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | Les préférences par utilisateur de l'interface d'un plugin (la permission `prefs`) |

L'envoi d'un paquet est un `POST /api/v1/admin/plugins/upload` multipart
(champs `plugin`, `sig` facultatif, `accept_permission_changes`) ;
`POST /api/v1/admin/plugins/preview` analyse sans installer. Les deux sont
des routes d'administrateur.

## Authentification

La gestion des plugins et les webhooks exigent un administrateur, comme
l'[API d'administration](../admin/README.md). `plugin_prefs` est le seul
service conçu pour un jeton de plugin : la page hôte l'émet pour un plugin et
un utilisateur, et le service ne renvoie que les lignes de ce couple
([API du serveur](../README.md#authentication)).

<!-- sources
methods: proto/evey/v1/admin_plugin.proto, webhook.proto, plugin_prefs.proto
upload and preview routes: internal/adapters/driving/http/plugin_asset_handler.go
plugin token on prefs: internal/adapters/driving/grpcapi/interceptors/rbac.go - PluginMethodScopes ; internal/adapters/driving/grpcapi/plugin_prefs_handler.go
-->
