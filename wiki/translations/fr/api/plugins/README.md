---
source_sha: 7d46731b1dfd44908d653fde2e55780a150fa9681de75fa6518859def0fe4dbd
reviewed: true
---
# API des plugins

Les points d'accès de gestion des plugins et des intégrations par webhook.

## Spécifications

| Spécification | Contenu |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Installer, activer, désactiver, désinstaller des plugins. Configuration et journaux des plugins. Environnements d'exécution. |
| [webhook.swagger.json](webhook.swagger.json) | Création, lecture, modification et suppression des webhooks, historique des envois |

## Développer un plugin

Les plugins dialoguent avec le serveur Evey en gRPC. Le guide complet est dans la
[documentation des plugins](../../plugins.md), notamment :

- le cycle de vie d'un plugin (installation, activation, configuration,
  désactivation, désinstallation) ;
- les événements disponibles ;
- le schéma de configuration ;
- l'isolation de sécurité (seccomp, isolation des processus).
