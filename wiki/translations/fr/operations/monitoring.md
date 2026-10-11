---
source_sha: d6335afa014ad627b79829f653197424d1a5c4572cdf3b9832ed19a3f7a079ac
reviewed: true
---
# Supervision et logs

Où voir ce que fait le serveur, pour l'administrateur qui veut garder un œil
dessus plutôt que réparer quelque chose : de ce qui est lu en ce moment aux
statistiques du mois, avec les logs entre les deux. Pour un symptôme précis,
commencez par le [Dépannage](troubleshooting.md).

## Ce qui est lu en ce moment

**Admin → En cours de lecture** liste tous les flux en cours : qui, quoi, sur
quel appareil, et comment, piste par piste (copiée, transcodée, et le backend
qui l'encode). Un administrateur peut **Arrêter** une session ou **Envoyer un
message** au spectateur.

La carte **Qui est en ligne** du tableau de bord liste qui est connecté en ce
moment, par profil, avec l'ancienneté du dernier signal de chaque application.
Un Chromecast dont la TV garde le récepteur d'Evey chargé reste listé après la
fin de sa lecture, une fois sorti de **En cours de lecture** ; **Arrêter ce
Chromecast** lui envoie un ordre d'arrêt, que la TV applique sous 30 secondes.

**Admin → Santé du système** montre le processeur, la mémoire, la bande
passante, le stockage, et la part de la capacité de transcodage utilisée.

<figure class="evey-shot" markdown="span">
  ![Admin → Santé du système sur le serveur de démo](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Admin → Santé du système</strong>, avec le travail de fond en cours juste en dessous.</figcaption>
</figure>

## Logs

**Admin → Logs & Debug** permet de chercher dans le journal du serveur et de
le suivre en **Direct**, restreint avec **Zone** à un seul domaine (lecteur et
transcodage, index de lecture, sous-titres, détection d'intro, Chromecast,
authentification, infrastructure...). C'est le premier endroit où regarder
quand un utilisateur signale un problème.

Le même journal est sur le disque, un fichier JSON par jour dans
`./evey-data/logs/` (`/data/logs/evey-AAAA-MM-JJ.log` dans le conteneur),
conservé 90 jours par défaut. `docker compose logs evey` ne montre que ce que
le conteneur affiche pendant qu'il démarre, ce qu'il faut lire quand le
serveur ne se lance pas. Voir [Dépannage](troubleshooting.md#the-logs).

## Comment les lectures se sont vraiment passées

**Admin → Qualité streaming** s'appuie sur ce que les lecteurs remontent
pendant la lecture (niveau de mémoire tampon, recharges, débit, au format
CMCD) : à quelle fréquence les spectateurs ont attendu, quels titres coupent le
plus, et quels types d'appareils peinent, sur le dernier jour ou la dernière
semaine.

Le serveur s'en sert aussi. Quand une catégorie de sources fait régulièrement
attendre les spectateurs, il abaisse pour un temps la qualité maximale de cette
catégorie, en plus du plafond mesuré, puis lève la limite de lui-même. Les
limites en cours figurent sous **Limites de qualité automatiques**, avec leur
raison et leur échéance ; **Réinitialiser** en supprime une immédiatement.

## Historique et statistiques

- **Admin → Analytique** : lectures du jour et de la semaine, utilisateurs
  actifs, titres les plus regardés des 30 derniers jours.
- **Admin → Activité** : l'historique des lectures et des actions
  d'administration, et les tâches de fond en cours.

La durée de conservation de chaque historique se règle dans **Admin →
Configuration du serveur → Paramètres du serveur** (sessions de lecture,
journal d'audit, métriques du serveur, qualité de streaming). Un changement
s'applique au nettoyage quotidien suivant.

## Métriques Prometheus

`GET /metrics` sert les compteurs du serveur au format texte de Prometheus,
tous nommés `evey_...`. Il répond à une session administrateur, ou à un jeton
fixe défini dans `EVEY_METRICS_SCRAPE_TOKEN` et envoyé en
`Authorization: Bearer <jeton>`, pour que Prometheus ou Grafana puissent
collecter sans se reconnecter
([Sauvegardes, logs et supervision](../environment/logs-monitoring.md)).
Les familles qui méritent un premier graphique :

| Métrique | Ce qu'elle dit |
| --- | --- |
| `evey_transcodes_active`, `evey_transcode_queue_depth` | Tâches de transcodage en arrière-plan (hors lecture en direct) en cours, et en attente d'une place. |
| `evey_hlsvod_sessions_active`, `evey_hlsvod_sessions_rejected_total` | Processus FFmpeg qui produisent des segments de lecture, et démarrages refusés parce que le serveur était à sa limite. |
| `evey_hlsvod_cache_size_bytes`, `evey_hlsvod_cache_evicted_bytes_total` | Taille du cache de segments sur le disque, et ce que le balayage a libéré. |
| `evey_mkvgo_walks_in_flight`, `evey_mkvgo_plan_stats` | L'empaqueteur intégré : fenêtres en cours de lecture, lectures en attente (`stat="slot_waits"`), voir [MKVGo dans Evey](../playback/mkvgo.md#load-replaced-files-and-updates). |
| `evey_http_requests_total`, `evey_http_request_duration_seconds` | Requêtes et latence par méthode et par chemin. |
| `evey_watcher_alive`, `evey_scan_queue_depth` | Si la surveillance des dossiers de chaque bibliothèque a répondu à sa dernière vérification (1 ou 0), et fichiers en attente d'analyse. |

## Ligne de commande

`evey admin doctor` vérifie tout le serveur en un seul rapport ; les autres
commandes de diagnostic sont listées dans la
[Ligne de commande](../runbooks/cli.md).
