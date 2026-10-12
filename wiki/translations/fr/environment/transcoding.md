---
source_sha: 6bbcf7c54732dc7eb7955206675ddd1923616486ecdec3c988c6ad0463cf8a04
reviewed: true
---
# Transcodage et matériel

Les variables qui règlent la capacité, la carte graphique, les limites de
qualité et le cache de transcodage. La plupart des installations n'en
définissent aucune : le serveur mesure la machine et choisit lui-même le
matériel, comme l'explique
[Transcodage et capacité du serveur](../playback/transcoding.md). Quand un
réglage existe dans **Administration → Configuration transcodage**, la valeur
enregistrée l'emporte.

## Accélération matérielle

Donner le GPU au conteneur :
[Accélération matérielle](../administration/hardware-acceleration.md).

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | vide | `auto`, `off`, `nvenc`, `vaapi`, `qsv` ou `rkmpp`. Vide : déduit des deux variables suivantes. | Configuration transcodage → **Accélération matérielle**, immédiat (relance la mesure) |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | L'ancien interrupteur marche/arrêt. | Configuration transcodage → **Accélération matérielle**, immédiat |
| `EVEY_TRANSCODING_HW_BACKEND` | vide (détection) | Impose un backend (`nvenc`, `vaapi`, `qsv`) : seul celui-ci est mesuré. Permet à plusieurs conteneurs Evey d'une même machine d'utiliser chacun une voie GPU différente. | aucun |
| `EVEY_TRANSCODING_HW_DEVICE` | vide (tous) | Impose un périphérique : un nœud de rendu (`/dev/dri/renderD129`) ou un index CUDA (`1`). Avec `EVEY_TRANSCODING_HW_BACKEND`, attache un conteneur à un GPU sur une machine qui en a plusieurs. | aucun |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | Une session matérielle qui n'a pas produit son premier segment dans ce délai est considérée comme un GPU bloqué et passe sur le processeur. | aucun |
| `EVEY_TRANSCODING_HW_TONEMAP` | activé | `0` désactive le tone mapping HDR sur le GPU et le confie au processeur, pour un pilote connu pour mal se comporter. | aucun |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | Un ffmpeg par qualité sur la voie GPU au lieu d'un seul processus pour toutes : isole les échecs, et évite le graphe à sorties multiples qui fait planter certains pilotes VA fragiles. | Configuration transcodage → Limites avancées → **Un processus d'encodage matériel par palier**, redémarrage |

L'encodeur Intel basse consommation et la sortie AV1 (interrupteur et
plafond) n'ont pas de variable : ils se règlent uniquement dans **Administration →
Configuration transcodage**.

## Capacité et limites

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (nombre de CPU) | Nombre total de sessions de transcodage simultanées. Sur une machine qui suffit à peine pour un transcodage 4K HDR logiciel, mettez `1` ou `2`. | Configuration transcodage → Limites avancées → **Sessions HLS simultanées maximum**, redémarrage |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Les tâches de transcodage hors lecture en direct. | Cache de transcodage → **Maximum simultané**, immédiat |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | N'encode que la qualité regardée. `false` partage un seul décodage entre toutes les qualités mais encode celles que personne ne regarde ; seulement pour une machine sans décodage matériel et des sources lourdes. | Configuration transcodage → Limites avancées → **Encoder un seul palier à la fois**, redémarrage |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (mesuré) | Impose le plafond de résolution du processeur à la place de celui qui est mesuré. | Configuration transcodage → Limites avancées → **Plafond de hauteur en transcodage logiciel**, redémarrage |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (mesuré) | Idem, pour les sources HDR converties par le processeur. | Configuration transcodage → Limites avancées → **Plafond de hauteur HDR en logiciel**, redémarrage |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | sans plafond | Threads par encodeur logiciel (1 à 64 ; toute autre valeur = sans plafond). Les encodeurs matériels l'ignorent. | aucun |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (suit la source) | Un plafond sur le débit de la vidéo transcodée. | Configuration transcodage → **Plafond de débit vidéo**, immédiat |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (aucun) | Le même plafond pour les clients hors du réseau domestique. | Configuration transcodage → **Plafond de débit vidéo, spectateurs distants**, immédiat |

## Cache

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | La taille de tout le cache de transcodage (segments, remux, téléchargements préparés), les éléments les moins récemment utilisés partant en premier. | Configuration du serveur → Paramètres du serveur → **Budget du cache de transcodage**, redémarrage |
| `EVEY_TRANSCODE_CACHE_MIN_FREE_MB` | `2048` | L'espace libre gardé sur le disque du cache. En dessous, les segments inutilisés sont évincés, puis les nouveaux segments sont refusés plutôt qu'écrits ; les sous-titres extraits ne sont plus conservés et les administrateurs sont prévenus. `0` désactive cette marge. Voir [Le cache de transcodage](../playback/transcoding.md#the-transcoding-cache). | aucun |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Son emplacement. Définie par l'image. | aucun |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | L'emplacement des miniatures de la barre de lecture. Définie par l'image : elles coûtent cher à régénérer, elles restent donc dans `/data`. | aucun |

## Extrait de calibrage

La mesure encode un extrait 4K. Par ordre de préférence : votre fichier, un
extrait téléchargé une fois et vérifié par son empreinte, ou un extrait
synthétique. Un extrait réel mesure plus fidèlement que l'extrait
synthétique. Aucune de ces variables n'a d'équivalent dans le panneau.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | vide | Un extrait à vous, HDR et SDR. | aucun |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | vide | Un extrait à télécharger une fois, en `https`. | aucun |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | vide | L'empreinte SHA256 de ce téléchargement ; obligatoire avec une URL. | aucun |
