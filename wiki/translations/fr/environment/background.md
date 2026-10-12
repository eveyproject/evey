---
source_sha: d0631abae137226c008a3954088887193957c49501d53f8c38282e9314512c14
reviewed: true
---
# Tâches de fond

Les variables qui règlent les passes que le serveur mène de lui-même et la
façon dont elles partagent le processeur. Ce que fait chaque passe et
comment la lecture en est protégée :
[Tâches de fond et charge du serveur](../operations/background-work.md).

## Part du processeur

La part du processeur que peuvent utiliser les tâches de fond se règle dans
**Administration → Configuration du serveur → Puissance allouée**, immédiatement, sans
redémarrage ; elle n'a pas de variable d'environnement. Les deux variables
ci-dessous dimensionnent un pool à la main et remplacent cette part
pour lui.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | selon la part | Fichiers analysés en parallèle pendant un scan. | aucun |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | selon la part | Titres analysés en même temps pour les intros et génériques. | aucun |

## Passes

Chaque variable a une clé, acceptée aussi comme ligne dans **Administration →
Configuration du serveur → Entrées de configuration**, qui propose ces
clés ; la ligne l'emporte sur l'environnement. La carte **Travaux en
arrière-plan** du même écran règle les principales. Toutes sont lues au
démarrage : un changement s'applique au redémarrage suivant, quelle que
soit la façon de le faire.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | activé | Met les tâches de fond en pause pendant une lecture. `off` les laisse tourner. | Entrées de configuration `transcoding.pause_bg_on_playback`, redémarrage |
| `EVEY_KEYFRAME_BACKFILL` | activé | Indexe les images clés des fichiers scannés et classe leur fréquence d'images. La lecture directe et la copie en dépendent. `off` la désactive. | Entrées de configuration `transcoding.keyframe_backfill`, redémarrage |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | défaut du serveur | Fichiers par passe (> 0). | Entrées de configuration `transcoding.keyframe_backfill_batch`, redémarrage |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | défaut du serveur | Millisecondes entre deux fichiers (>= 0). | Entrées de configuration `transcoding.keyframe_backfill_pause_ms`, redémarrage |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | défaut du serveur | Secondes entre deux passes au repos (> 0). | Entrées de configuration `transcoding.keyframe_backfill_idle_sec`, redémarrage |
| `EVEY_LEVEL_BACKFILL` | activé | Relève le niveau H.264/HEVC des fichiers scannés avant que cette information ne soit collectée. `off` la désactive. | Entrées de configuration `transcoding.level_backfill`, redémarrage |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | défaut du serveur | Même rythme, pour cette passe. | Entrées de configuration `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec`, redémarrage |
| `EVEY_SNAPSHOT_FASTPATH` | activé | Extrait les miniatures avec le décodeur rapide intégré. `off` n'utilise que l'extracteur exact de ffmpeg, pour l'écarter quand une miniature montre la mauvaise image. | Entrées de configuration `transcoding.snapshot_fastpath`, redémarrage |
| `EVEY_TITLE_PROBE` | activé | Sonde les titres en arrière-plan, en basse priorité, en pause pendant les lectures. `off` la désactive. | Entrées de configuration `transcoding.title_probe`, redémarrage |
