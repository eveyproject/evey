---
source_sha: e3b24610d4942cf71d19babf82add41a0f2e47fd513b1cc840fba554421a75e9
reviewed: true
---
# Bibliothèques et métadonnées { #libraries-and-metadata }

Les variables qui règlent le scan, les sources de métadonnées et la détection
des intros. La plupart ont un champ dans le panneau d'administration, qui
l'emporte dès qu'une valeur y est enregistrée ; le côté panneau est décrit
dans [Bibliothèques et métadonnées](../administration/libraries.md).

## Scan { #scanning }

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | La surveillance des fichiers en temps réel. | Configuration du serveur → Bibliothèques → **Surveillance des fichiers de la bibliothèque**, redémarrage |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | La fréquence à laquelle les dossiers sont revérifiés pour rattraper ce que la surveillance a manqué (partages réseau). | Configuration du serveur → Bibliothèques → **Intervalle d'analyse périodique**, redémarrage |
| `EVEY_LIBRARY_SCAN_WORKERS` | selon la part CPU | Fichiers analysés en parallèle pendant un scan. Définie à la main, elle remplace la **Puissance allouée** pour les scans. | aucun |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | L'emplacement où l'analyse de Santé des médias garde ses résultats et sa tâche en cours d'un redémarrage à l'autre. Vide : rien n'est conservé. | aucun |

## Métadonnées { #metadata }

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | La langue des titres et des résumés. La langue choisie dans l'assistant de configuration l'emporte sur elle. | Configuration du serveur → Langue et région → **Langue des métadonnées**, immédiat |
| `EVEY_TMDB_API_KEY` | clé intégrée | Votre propre clé TMDB, pour votre propre quota. | Configuration du serveur → Clés API → **Clé API TMDB**, redémarrage |
| `EVEY_OMDB_API_KEY` | vide | Une clé OMDb, seconde source pour les films. | Configuration du serveur → Clés API → **Clé API OMDB**, redémarrage |
| `EVEY_OPENSUBTITLES_API_KEY` | vide | Une clé OpenSubtitles, pour la recherche de sous-titres par l'API. | Configuration du serveur → Clés API → **Clé API OpenSubtitles**, redémarrage |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | vide | Un compte OpenSubtitles, utilisé pour se connecter à son API. | aucun |

## Intro, résumé et générique { #intro-recap-and-credits }

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Interroge les bases communautaires (TheIntroDB, puis IntroDB) avant la détection du serveur. | Configuration du serveur → Sources de détection des intros → **Activer les sources externes de skip intro**, redémarrage |
| `EVEY_INTRO_TIDB_API_KEY` | vide | Une clé TheIntroDB : 500 requêtes par jour au lieu de 100. | Configuration du serveur → Clés API → **Clé API TheIntroDB**, redémarrage |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Tous les lecteurs passent d'eux-mêmes les intros détectées. | Configuration du serveur → Bibliothèques → **Saut automatique des intros**, immédiat |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Propose aussi de passer l'aperçu de l'épisode suivant. | Configuration du serveur → Bibliothèques → **Passer les segments d'aperçu**, immédiat |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | selon la part CPU | Titres analysés en même temps. Définie à la main, elle remplace la **Puissance allouée** pour cette tâche. | aucun |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Threads par processus ffmpeg pendant la détection (1 à 16). | aucun |
