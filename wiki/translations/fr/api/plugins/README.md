---
source_sha: eea62b3dd7b2be9c176a9b8431d6ee58738ecf22c64c0f154bb1b1211878f293
reviewed: true
---
# API des plugins

Les points d'accès de gestion des plugins et des intégrations par webhook.

## Spécifications

| Spécification | Contenu |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Installer, activer, désactiver, désinstaller des plugins. Configuration et journaux des plugins. Environnements d'exécution. |
| [webhook.swagger.json](webhook.swagger.json) | Création, lecture, modification et suppression des webhooks, historique des envois |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | Les préférences par utilisateur de l'interface d'un plugin (la permission `prefs`) |

## Développer un plugin

Les plugins dialoguent avec le serveur Evey en gRPC. Le guide complet est dans la
[documentation des plugins](../../plugins.md), notamment :

- le cycle de vie d'un plugin (installation, activation, configuration,
  désactivation, désinstallation) ;
- les événements disponibles ;
- le schéma de configuration ;
- l'isolation de sécurité (seccomp, isolation des processus).
