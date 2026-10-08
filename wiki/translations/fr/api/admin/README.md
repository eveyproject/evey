---
source_sha: cc4cecd7aee5e32086fae7de125434b98b804bf28bed8ea336efed0bb38c02bb
reviewed: true
---
# API d'administration

Les points d'accès réservés aux administrateurs : ce qu'appelle **Admin**
dans l'application web. Un fichier par service, groupés par domaine.

### Système
| Spécification | Contenu |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | Tableau de bord, informations système, assistant d'installation |
| [server_config.swagger.json](server_config.swagger.json) | Réglages du serveur, en clé/valeur |
| [remote_access.swagger.json](remote_access.swagger.json) | Accès distant et réglages réseau |
| [backup.swagger.json](backup.swagger.json) | Sauvegarde et restauration de la base de données |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | Sauvegardes planifiées : expression cron, rétention, lancement immédiat |
| [activity.swagger.json](activity.swagger.json) | Historique de lecture, historique des connexions, métriques du serveur |
| [admin_session.swagger.json](admin_session.swagger.json) | Journal d'audit |
| [admin_task.swagger.json](admin_task.swagger.json) | Tâches de fond planifiées |
| [presence.swagger.json](presence.swagger.json) | Qui est connecté en ce moment (le signal des applications et la liste de l'administrateur) |

### Médias et transcodage
| Spécification | Contenu |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | Santé des médias (scans, réparations, historique et annulation), suppression de médias, rafraîchissement des métadonnées, génération des miniatures de navigation |
| [metadata.swagger.json](metadata.swagger.json) | Sources de métadonnées, recherche, corrections manuelles |
| [transcoding.swagger.json](transcoding.swagger.json) | Profils de transcodage, détection du matériel, tâches en cours |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | Gestion du cache de transcodage |
| [mkv.swagger.json](mkv.swagger.json) | Outils pour le conteneur MKV (pistes, chapitres, multiplexage et démultiplexage) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | Cache des temps d'intro : invalider un épisode, remplissage |

### Utilisateurs et règles
| Spécification | Contenu |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | Accès aux bibliothèques par utilisateur |
| [parental.swagger.json](parental.swagger.json) | Contrôle parental et code PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | Réglages de lecture globaux |

## Authentification

Chaque point d'accès exige un administrateur : le jeton de session d'un
compte administrateur, ou une clé d'API, qui appartient toujours à l'un d'eux
et doit porter la permission que la méthode demande (une clé sans permission
n'atteint aucune méthode d'administration). Un jeton de plugin est refusé
ici quelles que soient ses portées
([API du serveur](../README.md#authentication)).

<!-- sources
admin methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - MethodRoles ; api key scope on admin methods: interceptors/auth.go - authorize ; plugin token floor: auth.go - authorizePluginToken
-->
