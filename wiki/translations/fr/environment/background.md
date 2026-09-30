---
source_sha: 25e92baf53807987435c8781d87f2af02a8a02782951e049a23e404082ad7980
reviewed: true
---
# Tâches de fond

Les passes que le serveur mène de lui-même, et la façon dont elles partagent
le processeur. Leur comportement :
[Tâches de fond et charge du serveur](../operations/background-work.md).

## Part du processeur

La part du processeur que peuvent utiliser les tâches de fond se règle dans
**Admin → Configuration du serveur → Puissance allouée**, immédiatement, sans
redémarrage. Elle n'a pas de variable d'environnement. Les deux variables
ci-dessous dimensionnent un pool à la main et court-circuitent cette part pour
lui :

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | selon la part | Fichiers analysés en parallèle pendant un scan. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | selon la part | Titres analysés en même temps pour les intros et génériques. |

## Passes

Chaque variable a une clé, acceptée aussi comme ligne dans **Admin →
Configuration du serveur → Entrées de configuration**, qui propose ces clés ;
la ligne l'emporte. Toutes sont lues au démarrage, et la carte **Travaux en
arrière-plan** du même écran règle les principales.

| Variable | Clé du panneau | Défaut | Effet |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | `transcoding.pause_bg_on_playback` | activé | Met les tâches de fond en pause pendant une lecture. `off` les laisse tourner. |
| `EVEY_KEYFRAME_BACKFILL` | `transcoding.keyframe_backfill` | activé | Indexe les images clés des fichiers scannés et classe leur fréquence d'images. La lecture directe et la copie en dépendent. `off` la désactive. |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | `transcoding.keyframe_backfill_batch` | défaut du serveur | Fichiers par passe (> 0). |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | `transcoding.keyframe_backfill_pause_ms` | défaut du serveur | Millisecondes entre deux fichiers (>= 0). |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | `transcoding.keyframe_backfill_idle_sec` | défaut du serveur | Secondes entre deux passes au repos (> 0). |
| `EVEY_LEVEL_BACKFILL` | `transcoding.level_backfill` | activé | Relève le niveau H.264/HEVC des fichiers scannés avant que cette information ne soit collectée. `off` la désactive. |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec` | défaut du serveur | Même rythme, pour cette passe. |
| `EVEY_SNAPSHOT_FASTPATH` | `transcoding.snapshot_fastpath` | activé | Extrait les miniatures avec le décodeur rapide intégré. `off` n'utilise que l'extracteur exact de ffmpeg, pour l'écarter quand une miniature montre la mauvaise image. |
| `EVEY_TITLE_PROBE` | `transcoding.title_probe` | activé | Sonde les titres en arrière-plan, en basse priorité, en pause pendant les lectures. `off` la désactive. |
