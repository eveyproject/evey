---
source_sha: e314c511a93178cdc35b47b8bcd886cb60fd2ea303b7fc7d6f6c3c9ca1f9b6d0
---

# API d'administration

Points d'accès d'administration, réservés aux comptes disposant des droits correspondants.

## Spécifications

### Système
| Spécification | Description |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Tableau de bord, informations système, assistant d'installation |
| [server_config.swagger.json](server_config.swagger.json) | Magasin clé/valeur de la configuration serveur |
| [remote_access.swagger.json](remote_access.swagger.json) | Accès distant et réglages réseau |
| [backup.swagger.json](backup.swagger.json) | Sauvegarde et restauration de la base de données |
| [activity.swagger.json](activity.swagger.json) | Historique de lecture, historique de connexion, métriques du serveur |
| [admin_session.swagger.json](admin_session.swagger.json) | Journal d'audit |
| [admin_task.swagger.json](admin_task.swagger.json) | Tâches de fond planifiées |

### Médias et transcodage
| Spécification | Description |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Suppression d'éléments, rafraîchissement des métadonnées, génération du trickplay |
| [metadata.swagger.json](metadata.swagger.json) | Fournisseurs de métadonnées, recherche, valeurs forcées |
| [transcoding.swagger.json](transcoding.swagger.json) | Profils de transcodage, détection du matériel, tâches en cours |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Gestion du cache de transcodage |
| [mkv.swagger.json](mkv.swagger.json) | Outils pour conteneurs MKV (pistes, chapitres, multiplexage et démultiplexage) |

### Utilisateurs et règles
| Spécification | Description |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Contrôle d'accès aux bibliothèques, utilisateur par utilisateur |
| [parental.swagger.json](parental.swagger.json) | Contrôle parental et code PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Réglages de lecture globaux |

## Authentification

Tous les points d'accès exigent un jeton JWT d'administrateur valide.
