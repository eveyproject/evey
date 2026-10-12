---
source_sha: 423abcebbc76e4563928da41b2a7c832f1c58ae0ac12a6c3ca307b8a59582cb1
reviewed: true
---
# Sauvegardes, logs et supervision

Les variables qui règlent les sauvegardes de la base, le log du serveur, la
durée de conservation des historiques et les points d'accès pour une
supervision externe. Le quotidien est décrit dans
[Sauvegardes et mises à jour](../backups.md) et
[Supervision et logs](../operations/monitoring.md).

## Sauvegardes

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | L'emplacement des sauvegardes de la base. | aucun |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (désactivé) | Une sauvegarde toutes les N heures, indépendante de la planification du panneau, qui est la méthode habituelle. | Sauvegarde BDD et restauration → **Planification** (expression cron), immédiat |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Les sauvegardes plus anciennes sont supprimées quand l'intervalle ci-dessus est défini, au démarrage puis après chacune de ses exécutions. La planification du panneau a sa propre **Rétention**. | Sauvegarde BDD et restauration → Planification → **Rétention**, immédiat |

## Log

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` ou `error`. | Configuration du serveur → Paramètres du serveur → **Niveau de journalisation**, immédiat |
| `EVEY_LOG_DIR` | `/data/logs` | L'emplacement du fichier de log quotidien `evey-AAAA-MM-JJ.log`. Définie par l'image. Ce fichier fait référence : `docker logs` montre le même flux, mais Docker le fait tourner. | aucun |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Taille d'un fichier de log avant rotation. | aucun |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Nombre de jours de conservation des fichiers de log. | aucun |

## Conservation des historiques

Les durées de conservation sont relues à chaque nettoyage quotidien : une
valeur enregistrée dans **Administration → Configuration du serveur → Paramètres du
serveur** s'applique au balayage suivant.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (toujours) | Les sessions de lecture terminées de l'historique. | Paramètres du serveur → **Rétention de l'historique des sessions de lecture**, balayage suivant |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (toujours) | Le journal d'audit des actions d'administration. | Paramètres du serveur → **Rétention du journal d'audit**, balayage suivant |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | Les échantillons de ressources du serveur (processeur, mémoire, bande passante). | Paramètres du serveur → **Rétention des métriques serveur**, balayage suivant |
| `EVEY_QOE_RETENTION_DAYS` | `30` | Les rapports de qualité de lecture des lecteurs. | Paramètres du serveur → **Rétention qualité de lecture (QoE)**, balayage suivant |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | La fréquence de ces échantillons de ressources. | aucun |

## Supervision externe

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | vide | Un jeton fixe accepté sur `GET /metrics` en plus d'une session d'administrateur, pour que Prometheus ou Grafana puissent collecter sans renouveler une connexion. Vide : seule une session d'administrateur est acceptée. | aucun |
| `EVEY_OTLP_ENDPOINT` | vide | Un point d'accès OpenTelemetry vers lequel envoyer les traces. Vide : aucune trace. | aucun |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | Le nom du service dans les traces. | aucun |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | La part des requêtes tracées, de 0 à 1 (un ratio, pas un pourcentage). | aucun |

## Développement uniquement

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Mode développeur : les contrôles de sécurité du démarrage ne font qu'avertir, et les plugins peuvent tourner sans la bibliothèque de sandbox. Jamais sur un vrai serveur ; avec `EVEY_ENV=production`, le serveur refuse de démarrer. | aucun |
