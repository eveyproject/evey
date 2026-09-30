---
source_sha: 4ea9a6d07b743691fa611b4154d0b15ea314d413a0aa20fafe038abaf561a972
reviewed: true
---
# Sauvegardes, logs et supervision

Les sauvegardes, le log du serveur, la durée de conservation des historiques,
et les métriques pour une supervision externe.

## Sauvegardes

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | L'emplacement des sauvegardes de la base. |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (désactivé) | Une sauvegarde toutes les N heures. Utilisée seulement si aucune planification n'est définie dans **Admin → Sauvegarde BDD et restauration → Planification**, qui est la méthode habituelle. |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Les sauvegardes plus anciennes sont supprimées au démarrage et par la planification ci-dessus. Une planification définie dans le panneau a sa propre durée de conservation. |

Voir [Sauvegardes et mises à jour](../backups.md).

## Log

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` ou `error`. **Admin → Configuration du serveur → Paramètres du serveur** le remplace. |
| `EVEY_LOG_DIR` | `/data/logs` | L'emplacement du fichier de log quotidien `evey-AAAA-MM-JJ.log`. Définie par l'image. Ce fichier fait référence : `docker logs` montre le même flux, mais Docker le fait tourner. |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Taille d'un fichier de log avant rotation. |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Nombre de jours de conservation des fichiers de log. |

## Conservation des historiques

Chacune est remplacée par le champ correspondant de **Admin → Configuration du
serveur → Paramètres du serveur**, appliqué au nettoyage quotidien suivant.

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (toujours) | Les sessions de lecture terminées de l'historique. |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (toujours) | Le journal d'audit des actions d'administration. |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | Les échantillons de ressources du serveur (processeur, mémoire, bande passante). |
| `EVEY_QOE_RETENTION_DAYS` | `30` | Les rapports de qualité de lecture des lecteurs. |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | La fréquence de ces échantillons de ressources. |

## Supervision externe

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | vide | Un jeton fixe accepté sur `GET /metrics` en plus d'une session d'administrateur, pour que Prometheus ou Grafana puissent collecter sans renouveler une connexion. Vide : seule une session d'administrateur est acceptée. |
| `EVEY_OTLP_ENDPOINT` | vide | Un point d'accès OpenTelemetry vers lequel envoyer les traces. Vide : aucune trace. |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | Le nom du service dans les traces. |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | La part des requêtes tracées, de 0 à 1. |

## Développement uniquement

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Mode développeur : les contrôles de sécurité du démarrage ne font qu'avertir, et les plugins peuvent tourner sans la bibliothèque de sandbox. Jamais sur un vrai serveur ; avec `EVEY_ENV=production`, le serveur refuse de démarrer. |
