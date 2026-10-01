---
source_sha: 25e92baf53807987435c8781d87f2af02a8a02782951e049a23e404082ad7980
reviewed: false
---
# Tareas en segundo plano

Las pasadas que el servidor ejecuta por su cuenta y cómo se reparten el
procesador. Cómo se comportan:
[Tareas en segundo plano y carga del servidor](../operations/background-work.md).

## Parte de CPU

La parte del procesador que pueden usar las tareas en segundo plano se fija en
**Admin → Configuración del servidor → Potencia asignada**, en caliente, sin
reiniciar. No tiene variable de entorno. Las dos siguientes dimensionan un pool a
mano y se saltan esa parte para él:

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | según la parte | Archivos analizados en paralelo durante un escaneo. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | según la parte | Títulos analizados a la vez para intros y créditos. |

## Pasadas

Cada variable tiene una clave, aceptada también como fila en **Admin →
Configuración del servidor → Entradas de configuración**, que sugiere estas
claves; la fila prevalece. Todas se leen al arrancar, y **Trabajos en segundo
plano**, en la misma pantalla, fija las principales.

| Variable | Clave del panel | Por defecto | Efecto |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | `transcoding.pause_bg_on_playback` | activado | Pausa las tareas en segundo plano mientras se reproduce un flujo. `off` las mantiene en marcha. |
| `EVEY_KEYFRAME_BACKFILL` | `transcoding.keyframe_backfill` | activado | Indexa los fotogramas clave de los archivos escaneados y clasifica su frecuencia de imagen. La Reproducción directa y la copia dependen de ello. `off` lo desactiva. |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | `transcoding.keyframe_backfill_batch` | valor del servidor | Archivos por pasada (> 0). |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | `transcoding.keyframe_backfill_pause_ms` | valor del servidor | Milisegundos entre archivos (>= 0). |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | `transcoding.keyframe_backfill_idle_sec` | valor del servidor | Segundos entre pasadas en reposo (> 0). |
| `EVEY_LEVEL_BACKFILL` | `transcoding.level_backfill` | activado | Lee el nivel H.264/HEVC de los archivos escaneados antes de que se recogiera. `off` lo desactiva. |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec` | valor del servidor | El mismo ritmo, para esta pasada. |
| `EVEY_SNAPSHOT_FASTPATH` | `transcoding.snapshot_fastpath` | activado | Extrae las miniaturas con el descodificador rápido dentro del proceso. `off` usa solo el extractor exacto de ffmpeg, para descartarlo cuando una miniatura muestra el fotograma equivocado. |
| `EVEY_TITLE_PROBE` | `transcoding.title_probe` | activado | Sondea los títulos en segundo plano con baja prioridad, en pausa durante la reproducción. `off` lo desactiva. |
