---
source_sha: 943149aa81fcdc21ea4425ec9e43d282740de164c026d29cc366577b71c0fd72
reviewed: true
---
# Transcodage et matériel

Capacité, carte graphique, limites de qualité et cache de transcodage. La
plupart des installations n'en définissent aucune : le serveur mesure la
machine et choisit lui-même le matériel (voir
[Transcodage et capacité du serveur](../playback/transcoding.md)). Quand un
réglage existe dans **Admin → Configuration transcodage**, la valeur
enregistrée l'emporte ; le tableau le précise.

## Accélération matérielle

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | vide | `auto`, `off`, `nvenc`, `vaapi`, `qsv` ou `rkmpp`. Vide : déduit des deux variables suivantes. Remplacé par **Accélération matérielle** dans le panneau, qui relance la mesure à chaud. |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | L'ancien interrupteur marche/arrêt. Remplacé par le panneau. |
| `EVEY_TRANSCODING_HW_BACKEND` | vide (détection) | Impose un backend (`nvenc`, `vaapi`, `qsv`) : seul celui-ci est mesuré. Permet à plusieurs conteneurs Evey d'une même machine d'utiliser chacun une voie GPU différente. |
| `EVEY_TRANSCODING_HW_DEVICE` | vide (tous) | Impose un périphérique : un nœud de rendu (`/dev/dri/renderD129`) ou un index CUDA (`1`). Avec `EVEY_TRANSCODING_HW_BACKEND`, attache un conteneur à un GPU sur une machine qui en a plusieurs. |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | Une session matérielle qui n'a pas produit son premier segment dans ce délai est considérée comme un GPU bloqué et passe sur le processeur. |
| `EVEY_TRANSCODING_HW_TONEMAP` | activé | `0` désactive le tone mapping HDR sur le GPU et le confie au processeur, pour un pilote connu pour mal se comporter. |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | Un ffmpeg par qualité sur la voie GPU au lieu d'un seul processus pour toutes : isole les échecs, et évite le graphe à sorties multiples qui fait planter certains pilotes VA fragiles. Remplacé par **Limites avancées** ; lu au démarrage. |

Donner le GPU au conteneur : [Accélération matérielle](../administration/hardware-acceleration.md).

## Capacité et limites

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (nombre de CPU) | Nombre total de sessions de transcodage simultanées. Sur une machine qui tient à peine un transcodage 4K HDR logiciel, mettez `1` ou `2`. Remplacé par **Limites avancées** ; lu au démarrage. |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Les tâches de transcodage hors lecture en direct. Remplacé par **Maximum simultané** dans **Admin → Cache de transcodage**. |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | N'encode que la qualité regardée. `false` partage un seul décodage entre toutes les qualités mais encode celles que personne ne regarde ; seulement pour une machine sans décodage matériel et des sources lourdes. Remplacé par **Limites avancées** ; lu au démarrage. |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (mesuré) | Impose le plafond de résolution du processeur à la place de celui qui est mesuré. Remplacé par **Limites avancées** ; lu au démarrage. |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (mesuré) | Idem, pour les sources HDR converties par le processeur. |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | sans plafond | Threads par encodeur logiciel (1 à 64). Les encodeurs matériels l'ignorent. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (suit la source) | Un plafond sur le débit de la vidéo transcodée. Remplacé par **Plafond de débit vidéo**. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (aucun) | Le même plafond pour les clients hors du réseau de la maison. Remplacé par **Plafond de débit vidéo, spectateurs distants**. |

## Cache

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | La taille de tout le cache de transcodage (segments, remux, téléchargements préparés), les éléments les moins récemment utilisés partant en premier. 2 Gio restent toujours libres sur le volume. Remplacé par **Budget du cache de transcodage** dans **Paramètres du serveur** ; lu au démarrage. |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Son emplacement. Définie par l'image. |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | L'emplacement des miniatures de navigation. Définie par l'image : elles coûtent cher à régénérer, elles restent donc dans `/data`. |

## Extrait de calibrage

La mesure encode un extrait 4K. Par ordre de préférence : votre fichier, un
extrait téléchargé une fois et vérifié par son empreinte, ou un extrait
synthétique. Un extrait réel mesure plus fidèlement que l'extrait
synthétique.

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | vide | Un extrait à vous, HDR et SDR. |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | vide | Un extrait à télécharger une fois, en `https`. |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | vide | L'empreinte SHA256 de ce téléchargement ; obligatoire avec une URL. |
