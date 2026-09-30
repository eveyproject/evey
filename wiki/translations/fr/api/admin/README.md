---
source_sha: e314c511a93178cdc35b47b8bcd886cb60fd2ea303b7fc7d6f6c3c9ca1f9b6d0
reviewed: true
---
# API d'administration

Les points d'accès réservés aux utilisateurs ayant les droits d'administrateur.

## Spécifications

### Système
| Spécification | Contenu |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Tableau de bord, informations système, assistant d'installation |
| [server_config.swagger.json](server_config.swagger.json) | Réglages du serveur, en clé/valeur |
| [remote_access.swagger.json](remote_access.swagger.json) | Accès distant et réglages réseau |
| [backup.swagger.json](backup.swagger.json) | Sauvegarde et restauration de la base de données |
| [activity.swagger.json](activity.swagger.json) | Historique de lecture, historique des connexions, métriques du serveur |
| [admin_session.swagger.json](admin_session.swagger.json) | Journal d'audit |
| [admin_task.swagger.json](admin_task.swagger.json) | Tâches de fond planifiées |

### Médias et transcodage
| Spécification | Contenu |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Suppression de médias, rafraîchissement des métadonnées, génération des miniatures de navigation |
| [metadata.swagger.json](metadata.swagger.json) | Sources de métadonnées, recherche, corrections manuelles |
| [transcoding.swagger.json](transcoding.swagger.json) | Profils de transcodage, détection du matériel, tâches en cours |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Gestion du cache de transcodage |
| [mkv.swagger.json](mkv.swagger.json) | Outils pour le conteneur MKV (pistes, chapitres, multiplexage et démultiplexage) |

### Utilisateurs et règles
| Spécification | Contenu |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Accès aux bibliothèques par utilisateur |
| [parental.swagger.json](parental.swagger.json) | Contrôle parental et code PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Réglages de lecture globaux |

## Authentification

Tous les points d'accès demandent un jeton JWT d'administrateur valide.
