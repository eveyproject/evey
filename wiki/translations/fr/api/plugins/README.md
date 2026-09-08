---
source_sha: 7d46731b1dfd44908d653fde2e55780a150fa9681de75fa6518859def0fe4dbd
---

# API des extensions

Points d'accès pour gérer les extensions et les intégrations par webhook.

## Spécifications

| Spécification | Description |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Installer, activer, désactiver et désinstaller des extensions. Configuration et journaux d'une extension. Environnements d'exécution. |
| [webhook.swagger.json](webhook.swagger.json) | Création, lecture, modification et suppression de webhooks, historique des envois |

## Développer une extension

Les extensions dialoguent avec le serveur Evey par gRPC. La [documentation des extensions](../../plugins.md) contient le guide de développement complet, notamment :
- le cycle de vie d'une extension (installation, activation, configuration, désactivation, désinstallation) ;
- les points d'accroche disponibles pour les événements ;
- le schéma de configuration ;
- l'isolation de sécurité (seccomp, séparation des processus).
