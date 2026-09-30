---
source_sha: daa61595acb5937c7640f83d1baaec5b538968a8324e20be12058340e8e77c15
reviewed: true
---
# Bibliothèques et métadonnées

Scan, sources de métadonnées et détection des intros. La plupart de ces
variables ont un équivalent dans le panneau d'administration, qui l'emporte dès
qu'une valeur y est enregistrée.

## Scan

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | La surveillance des fichiers en temps réel. |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | La fréquence à laquelle les dossiers sont revérifiés pour rattraper ce que la surveillance a manqué (partages réseau). **Admin → Configuration du serveur → Paramètres du serveur** la remplace. |
| `EVEY_LIBRARY_SCAN_WORKERS` | selon la part CPU | Fichiers analysés en parallèle pendant un scan. Définie à la main, elle court-circuite la **Puissance allouée** pour les scans. |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | L'emplacement où l'analyse de Santé des médias garde ses résultats et sa tâche en cours d'un redémarrage à l'autre. Vide : rien n'est conservé. |

## Métadonnées

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | La langue des titres et des résumés. La langue choisie dans l'assistant de configuration ou dans **Admin → Configuration du serveur → Langue et région** l'emporte. |
| `EVEY_TMDB_API_KEY` | clé intégrée | Votre propre clé TMDB, pour votre propre quota. La clé saisie dans **Admin → Configuration du serveur → Clés API** l'emporte. |
| `EVEY_OMDB_API_KEY` | vide | Une clé OMDb, seconde source pour les films. Même priorité. |
| `EVEY_OPENSUBTITLES_API_KEY` | vide | Une clé OpenSubtitles, pour la recherche de sous-titres par l'API. Même priorité. |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | vide | Un compte OpenSubtitles, utilisé pour se connecter à son API. |

## Intro, résumé et générique

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Interroge les bases communautaires (TheIntroDB, puis IntroDB) avant la détection du serveur. **Admin → Configuration du serveur → Sources de détection des intros** l'emporte ; redémarrez après l'avoir modifiée. |
| `EVEY_INTRO_TIDB_API_KEY` | vide | Une clé TheIntroDB : 500 requêtes par jour au lieu de 100. |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Tous les lecteurs passent d'eux-mêmes les intros détectées. S'applique immédiatement depuis le panneau. |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Propose aussi de passer l'aperçu de l'épisode suivant. S'applique immédiatement depuis le panneau. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | selon la part CPU | Titres analysés en même temps. Définie à la main, elle court-circuite la **Puissance allouée** pour cette tâche. |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Threads par processus ffmpeg pendant la détection (1 à 16). |
